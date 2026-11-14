package prom

import (
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

type lmAPIResponse struct {
	Data json.RawMessage        `json:"data"`
	Meta map[string]interface{} `json:"meta,omitempty"`
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

func TranslateLMResponse(rawLMBody []byte, defaultResultType string, debug bool, endpoint string) ([]byte, error) {
	var lmResp lmAPIResponse
	resultType := defaultResultType

	if err := json.Unmarshal(rawLMBody, &lmResp); err == nil {
		if val, ok := lmResp.Meta["resultType"].(string); ok && val != "" {
			resultType = val
		}
	} else {
		lmResp.Data = rawLMBody
	}

	var parsedData interface{}
	_ = json.Unmarshal(lmResp.Data, &parsedData)

	promResult := parsedData

	switch resultType {
	case "scalar":
		var metrics []lmMetricResult
		if err := json.Unmarshal(lmResp.Data, &metrics); err == nil && len(metrics) > 0 {
			if len(metrics[0].Values) > 0 {
				v := metrics[0].Values[0]
				promResult = []interface{}{v.Timestamp, v.Value}
			}
		}
	case "vector":
		var metrics []lmMetricResult
		if err := json.Unmarshal(lmResp.Data, &metrics); err == nil {
			var vectorResults []map[string]interface{}
			for _, m := range metrics {
				if len(m.Values) > 0 {
					v := m.Values[0]
					vectorResults = append(vectorResults, map[string]interface{}{
						"metric": m.Metric,
						"value":  []interface{}{v.Timestamp, v.Value},
					})
				} else if m.Value != nil {
					vectorResults = append(vectorResults, map[string]interface{}{
						"metric": m.Metric,
						"value":  []interface{}{m.Value.Timestamp, m.Value.Value},
					})
				}
			}
			promResult = vectorResults
		}
	case "matrix":
		var metrics []lmMetricResult
		if err := json.Unmarshal(lmResp.Data, &metrics); err == nil {
			var matrixResults []map[string]interface{}
			for _, m := range metrics {
				var formattedValues [][]interface{}
				for _, v := range m.Values {
					formattedValues = append(formattedValues, []interface{}{v.Timestamp, v.Value})
				}
				matrixResults = append(matrixResults, map[string]interface{}{
					"metric": m.Metric,
					"values": formattedValues,
				})
			}
			promResult = matrixResults
		}
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
	var lmResp lmAPIResponse
	var rawData interface{}

	// Extract the "data" portion from LogicMonitor's response envelope
	if err := json.Unmarshal(rawBody, &lmResp); err == nil && len(lmResp.Data) > 0 {
		_ = json.Unmarshal(lmResp.Data, &rawData)
	} else {
		_ = json.Unmarshal(rawBody, &rawData)
		rawData = rawBody
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
