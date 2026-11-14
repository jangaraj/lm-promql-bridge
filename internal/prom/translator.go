package prom

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
)

type PromResponse struct {
	Status    string   `json:"status"`
	Data      PromData `json:"data"`
	ErrorType string   `json:"errorType,omitempty"`
	Error     string   `json:"error,omitempty"`
}

type PromData struct {
	ResultType string      `json:"resultType"`
	Result     interface{} `json:"result"`
}

type PromMetaResponse struct {
	Status    string      `json:"status"`
	Data      interface{} `json:"data"`
	ErrorType string      `json:"errorType,omitempty"`
	Error     string      `json:"error,omitempty"`
}

type lmMetricResult struct {
	Metric map[string]string `json:"metric,omitempty"`
	Values []lmValue         `json:"values,omitempty"`
	Value  *lmValue          `json:"value,omitempty"`
}

type lmValue struct {
	Timestamp float64 `json:"timestamp"`
	Value     string  `json:"value"`
}

func extractLMData(rawBody []byte) (json.RawMessage, string, error) {
	if !json.Valid(rawBody) {
		return nil, "", fmt.Errorf("LogicMonitor response is not valid JSON")
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &object); err != nil {
		return nil, "", fmt.Errorf("decode LogicMonitor response: %w", err)
	}

	// LogicMonitor normally wraps responses in an object with a "data" field.
	// Also accept a direct JSON value for upstream endpoints that return data
	// without the standard envelope.
	data, hasData := object["data"]
	if !hasData {
		return json.RawMessage(bytes.TrimSpace(rawBody)), "", nil
	}
	if len(data) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil, "", fmt.Errorf("LogicMonitor response is missing non-null data")
	}

	resultType := ""
	if rawMeta, hasMeta := object["meta"]; hasMeta {
		var meta struct {
			ResultType string `json:"resultType"`
		}
		if err := json.Unmarshal(rawMeta, &meta); err != nil {
			return nil, "", fmt.Errorf("decode LogicMonitor response metadata: %w", err)
		}
		resultType = meta.ResultType
	}

	return data, resultType, nil
}

func decodeMetricResults(data json.RawMessage) ([]lmMetricResult, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, fmt.Errorf("expected LogicMonitor metric data to be an array")
	}

	var metrics []lmMetricResult
	if err := json.Unmarshal(trimmed, &metrics); err != nil {
		return nil, fmt.Errorf("decode LogicMonitor metric data: %w", err)
	}
	if metrics == nil {
		return nil, fmt.Errorf("LogicMonitor metric data is null; expected an array")
	}

	return metrics, nil
}

func TranslateLMResponse(rawLMBody []byte, defaultResultType string, debug bool, endpoint string) ([]byte, error) {
	data, upstreamResultType, err := extractLMData(rawLMBody)
	if err != nil {
		return nil, err
	}

	resultType := defaultResultType
	if upstreamResultType != "" {
		resultType = upstreamResultType
	}

	metrics, err := decodeMetricResults(data)
	if err != nil {
		return nil, err
	}

	var promResult interface{}

	switch resultType {
	case "scalar":
		if len(metrics) == 0 {
			return nil, fmt.Errorf("LogicMonitor scalar response contains no metric values")
		}

		var value *lmValue
		if len(metrics[0].Values) > 0 {
			value = &metrics[0].Values[0]
		} else {
			value = metrics[0].Value
		}
		if value == nil {
			return nil, fmt.Errorf("LogicMonitor scalar response contains no value")
		}
		promResult = []interface{}{value.Timestamp, value.Value}

	case "vector":
		vectorResults := make([]map[string]interface{}, 0, len(metrics))
		for i, metric := range metrics {
			var value *lmValue
			if len(metric.Values) > 0 {
				value = &metric.Values[0]
			} else {
				value = metric.Value
			}
			if value == nil {
				return nil, fmt.Errorf("LogicMonitor vector result %d contains no value", i)
			}

			vectorResults = append(vectorResults, map[string]interface{}{
				"metric": metric.Metric,
				"value":  []interface{}{value.Timestamp, value.Value},
			})
		}
		promResult = vectorResults

	case "matrix":
		matrixResults := make([]map[string]interface{}, 0, len(metrics))
		for i, metric := range metrics {
			if metric.Values == nil {
				return nil, fmt.Errorf("LogicMonitor matrix result %d is missing values", i)
			}

			formattedValues := make([][]interface{}, 0, len(metric.Values))
			for _, value := range metric.Values {
				formattedValues = append(formattedValues, []interface{}{value.Timestamp, value.Value})
			}
			matrixResults = append(matrixResults, map[string]interface{}{
				"metric": metric.Metric,
				"values": formattedValues,
			})
		}
		promResult = matrixResults

	default:
		return nil, fmt.Errorf("unsupported Prometheus result type %q", resultType)
	}

	promResp := PromResponse{
		Status: "success",
		Data: PromData{
			ResultType: resultType,
			Result:     promResult,
		},
	}

	out, err := json.Marshal(promResp)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal translated response: %w", err)
	}

	if debug {
		slog.Info("PromQL Final Response Sent",
			"endpoint", endpoint,
			"resultType", resultType,
			"payload", string(out),
		)
	}

	return out, nil
}

func TranslateMetadataResponse(rawBody []byte, debug bool, endpoint string) ([]byte, error) {
	data, _, err := extractLMData(rawBody)
	if err != nil {
		return nil, err
	}

	var rawData interface{}
	if err := json.Unmarshal(data, &rawData); err != nil {
		return nil, fmt.Errorf("decode LogicMonitor metadata data: %w", err)
	}
	if rawData == nil {
		return nil, fmt.Errorf("LogicMonitor metadata data is null")
	}

	resp := PromMetaResponse{
		Status: "success",
		Data:   rawData,
	}

	out, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata response: %w", err)
	}

	if debug {
		slog.Info("PromQL Final Metadata Response Sent",
			"endpoint", endpoint,
			"payload", string(out),
		)
	}

	return out, nil
}
