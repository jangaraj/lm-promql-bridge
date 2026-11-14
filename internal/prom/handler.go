package prom

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	json "github.com/json-iterator/go"

	"lm-promql-bridge/internal/lm"
)

type Handler struct {
	lmClient    *lm.Client
	debug       bool
	concurrency chan struct{}
}

func NewHandler(lmClient *lm.Client, debug bool) *Handler {
	return &Handler{
		lmClient:    lmClient,
		debug:       debug,
		concurrency: make(chan struct{}, 50), // Limit concurrent inbound requests
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/query", h.HandleQuery)
	mux.HandleFunc("/api/v1/query_range", h.HandleQueryRange)
	mux.HandleFunc("/api/v1/labels", h.HandleLabels)
	mux.HandleFunc("/api/v1/label/", h.HandleLabelValuesOrSeries)
	mux.HandleFunc("/api/v1/series", h.HandleSeries)
	mux.HandleFunc("/api/v1/metadata", h.HandleMetadata)
}

func writeError(w http.ResponseWriter, errorType, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	resp := PromResponse{
		Status:    "error",
		ErrorType: errorType,
		Error:     message,
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func parseRequestParams(r *http.Request) (int64, int64, int, []string, string) {
	var start, end int64
	limit := 200
	var matches []string
	explicitMetric := ""

	q := r.URL.Query()
	if s := q.Get("start"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			start = v
		}
	}
	if e := q.Get("end"); e != "" {
		if v, err := strconv.ParseInt(e, 10, 64); err == nil {
			end = v
		}
	}
	if l := q.Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	if m := q["match[]"]; len(m) > 0 {
		matches = append(matches, m...)
	}
	if m := q["match"]; len(m) > 0 {
		matches = append(matches, m...)
	}
	if met := q.Get("metric"); met != "" {
		explicitMetric = met
	}

	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		ct := r.Header.Get("Content-Type")
		if strings.Contains(ct, "application/json") {
			bodyBytes, err := io.ReadAll(r.Body)
			if err == nil && len(bodyBytes) > 0 {
				r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

				var jsonMap map[string]interface{}
				if json.Unmarshal(bodyBytes, &jsonMap) == nil {
					if val, ok := jsonMap["start"]; ok {
						switch v := val.(type) {
						case float64:
							start = int64(v)
						case string:
							if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
								start = parsed
							}
						}
					}
					if val, ok := jsonMap["end"]; ok {
						switch v := val.(type) {
						case float64:
							end = int64(v)
						case string:
							if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
								end = parsed
							}
						}
					}
					if val, ok := jsonMap["limit"]; ok {
						switch v := val.(type) {
						case float64:
							if int(v) > 0 {
								limit = int(v)
							}
						case string:
							if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
								limit = parsed
							}
						}
					}
					if val, ok := jsonMap["metric"]; ok {
						if s, ok := val.(string); ok && s != "" {
							explicitMetric = s
						}
					}
					for _, key := range []string{"match[]", "match"} {
						if val, ok := jsonMap[key]; ok {
							switch v := val.(type) {
							case string:
								matches = append(matches, v)
							case []interface{}:
								for _, item := range v {
									if s, ok := item.(string); ok {
										matches = append(matches, s)
									}
								}
							}
						}
					}
				}
			}
		} else {
			_ = r.ParseForm()
			if s := r.Form.Get("start"); s != "" {
				if parsed, err := strconv.ParseInt(s, 10, 64); err == nil {
					start = parsed
				}
			}
			if e := r.Form.Get("end"); e != "" {
				if parsed, err := strconv.ParseInt(e, 10, 64); err == nil {
					end = parsed
				}
			}
			if l := r.Form.Get("limit"); l != "" {
				if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
					limit = parsed
				}
			}
			if m := r.Form["match[]"]; len(m) > 0 {
				matches = append(matches, m...)
			}
			if m := r.Form["match"]; len(m) > 0 {
				matches = append(matches, m...)
			}
			if met := r.Form.Get("metric"); met != "" {
				explicitMetric = met
			}
		}
	}

	return start, end, limit, matches, explicitMetric
}

func extractContainsFromMatches(matches []string) string {
	for _, m := range matches {
		if strings.Contains(m, "__name__") {
			idx := strings.Index(m, "__name__")
			sub := m[idx:]
			parts := strings.SplitN(sub, "=", 2)
			if len(parts) == 2 {
				val := strings.TrimSpace(parts[1])
				val = strings.TrimPrefix(val, "~")
				val = strings.Trim(val, "\"'{}()")
				val = strings.TrimPrefix(val, ".*")
				val = strings.TrimSuffix(val, ".*")
				val = strings.TrimPrefix(val, "^")
				val = strings.TrimSuffix(val, "$")
				return strings.TrimSpace(val)
			}
		}
	}
	return ""
}

func extractMetricFromMatches(matches []string) string {
	for _, m := range matches {
		if strings.Contains(m, "__name__") {
			idx := strings.Index(m, "__name__")
			sub := m[idx:]
			parts := strings.SplitN(sub, "=", 2)
			if len(parts) == 2 {
				opAndVal := strings.TrimSpace(parts[0])
				if !strings.Contains(opAndVal, "~") {
					val := strings.TrimSpace(parts[1])
					val = strings.Trim(val, "\"'{}()")
					if val != "" {
						return val
					}
				}
			}
		}
	}
	return ""
}

func (h *Handler) acquireConcurrency(r *http.Request) bool {
	select {
	case h.concurrency <- struct{}{}:
		return true
	case <-r.Context().Done():
		return false
	}
}

func (h *Handler) HandleQuery(w http.ResponseWriter, r *http.Request) {
	if !h.acquireConcurrency(r) {
		return
	}
	defer func() { <-h.concurrency }()

	var query, timeStr string

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err == nil {
			query = r.FormValue("query")
			timeStr = r.FormValue("time")
		} else {
			var bodyMap map[string]string
			if err := json.NewDecoder(r.Body).Decode(&bodyMap); err == nil {
				query = bodyMap["query"]
				timeStr = bodyMap["time"]
			}
		}
	} else {
		query = r.URL.Query().Get("query")
		timeStr = r.URL.Query().Get("time")
	}

	if query == "" {
		writeError(w, "bad_data", "the query parameter is missing", http.StatusBadRequest)
		return
	}

	lmReq := &lm.QueryRequest{
		Query: query,
		Time:  timeStr,
	}

	rawResp, err := h.lmClient.Query(r.Context(), lmReq)
	if err != nil {
		slog.Error("failed to execute instant query against LM", "err", err)
		writeError(w, "execution", err.Error(), http.StatusInternalServerError)
		return
	}

	translated, err := TranslateLMResponse(rawResp, "vector", h.debug, r.URL.Path)
	if err != nil {
		writeError(w, "internal", err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(translated)
}

func (h *Handler) HandleQueryRange(w http.ResponseWriter, r *http.Request) {
	if !h.acquireConcurrency(r) {
		return
	}
	defer func() { <-h.concurrency }()

	var query, start, end, step string

	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		query = r.FormValue("query")
		start = r.FormValue("start")
		end = r.FormValue("end")
		step = r.FormValue("step")
		if query == "" {
			var bodyMap map[string]string
			if err := json.NewDecoder(r.Body).Decode(&bodyMap); err == nil {
				query = bodyMap["query"]
				start = bodyMap["start"]
				end = bodyMap["end"]
				step = bodyMap["step"]
			}
		}
	} else {
		query = r.URL.Query().Get("query")
		start = r.URL.Query().Get("start")
		end = r.URL.Query().Get("end")
		step = r.URL.Query().Get("step")
	}

	if query == "" || start == "" || end == "" || step == "" {
		writeError(w, "bad_data", "missing required parameters (query, start, end, step)", http.StatusBadRequest)
		return
	}

	lmReq := &lm.QueryRequest{
		Query: query,
		Start: start,
		End:   end,
		Step:  step,
	}

	rawResp, err := h.lmClient.QueryRange(r.Context(), lmReq)
	if err != nil {
		slog.Error("failed to execute range query against LM", "err", err)
		writeError(w, "execution", err.Error(), http.StatusInternalServerError)
		return
	}

	translated, err := TranslateLMResponse(rawResp, "matrix", h.debug, r.URL.Path)
	if err != nil {
		writeError(w, "internal", err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(translated)
}

func (h *Handler) HandleLabels(w http.ResponseWriter, r *http.Request) {
	if !h.acquireConcurrency(r) {
		return
	}
	defer func() { <-h.concurrency }()

	_, _, _, matches, explicitMetric := parseRequestParams(r)
	metric := explicitMetric
	if metric == "" {
		metric = extractMetricFromMatches(matches)
	}

	rawResp, err := h.lmClient.Labels(r.Context(), &lm.LabelsRequest{
		Metric: metric,
	})
	if err != nil {
		slog.Error("failed to fetch labels from LM", "err", err)
		writeError(w, "execution", err.Error(), http.StatusInternalServerError)
		return
	}

	translated, err := TranslateMetadataResponse(rawResp, h.debug, r.URL.Path)
	if err != nil {
		writeError(w, "internal", err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(translated)
}

func (h *Handler) HandleLabelValuesOrSeries(w http.ResponseWriter, r *http.Request) {
	if !h.acquireConcurrency(r) {
		return
	}
	defer func() { <-h.concurrency }()

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/label/")
	parts := strings.Split(path, "/")

	if len(parts) >= 2 && parts[1] == "values" {
		labelName := parts[0]
		start, end, limit, matches, explicitMetric := parseRequestParams(r)

		if labelName == "__name__" {
			contains := explicitMetric
			if contains == "" {
				contains = extractContainsFromMatches(matches)
			}

			rawResp, err := h.lmClient.MetricNames(r.Context(), &lm.MetricNamesRequest{
				Start:    start,
				End:      end,
				Limit:    limit,
				Contains: contains,
			})
			if err != nil {
				slog.Error("failed to fetch metric names from LM for __name__", "err", err)
				writeError(w, "execution", err.Error(), http.StatusInternalServerError)
				return
			}

			translated, err := TranslateMetadataResponse(rawResp, h.debug, r.URL.Path)
			if err != nil {
				writeError(w, "internal", err.Error(), http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(translated)
			return
		}

		metric := explicitMetric
		if metric == "" {
			metric = extractMetricFromMatches(matches)
		}

		rawResp, err := h.lmClient.LabelValues(r.Context(), &lm.LabelValuesRequest{
			Key:    labelName,
			Metric: metric,
			Start:  start,
			End:    end,
			Limit:  limit,
		})
		if err != nil {
			slog.Error("failed to fetch label values from LM", "label", labelName, "err", err)
			writeError(w, "execution", err.Error(), http.StatusInternalServerError)
			return
		}

		translated, err := TranslateMetadataResponse(rawResp, h.debug, r.URL.Path)
		if err != nil {
			writeError(w, "internal", err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(translated)
		return
	}

	writeError(w, "bad_data", "not found", http.StatusNotFound)
}

func (h *Handler) HandleSeries(w http.ResponseWriter, r *http.Request) {
	if !h.acquireConcurrency(r) {
		return
	}
	defer func() { <-h.concurrency }()

	start, end, limit, matches, explicitMetric := parseRequestParams(r)
	contains := explicitMetric
	if contains == "" {
		contains = extractContainsFromMatches(matches)
	}

	rawResp, err := h.lmClient.MetricNames(r.Context(), &lm.MetricNamesRequest{
		Start:    start,
		End:      end,
		Limit:    limit,
		Contains: contains,
	})
	if err != nil {
		slog.Error("failed to fetch metric names from LM", "err", err)
		writeError(w, "execution", err.Error(), http.StatusInternalServerError)
		return
	}

	translated, err := TranslateMetadataResponse(rawResp, h.debug, r.URL.Path)
	if err != nil {
		writeError(w, "internal", err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(translated)
}

type MetricMetadata struct {
	Type string `json:"type"`
	Help string `json:"help"`
	Unit string `json:"unit"`
}

func (h *Handler) HandleMetadata(w http.ResponseWriter, r *http.Request) {
	if !h.acquireConcurrency(r) {
		return
	}
	defer func() { <-h.concurrency }()

	start, end, limit, matches, explicitMetric := parseRequestParams(r)
	metricFilter := explicitMetric
	if metricFilter == "" {
		metricFilter = extractContainsFromMatches(matches)
	}

	rawResp, err := h.lmClient.MetricNames(r.Context(), &lm.MetricNamesRequest{
		Start:    start,
		End:      end,
		Limit:    limit,
		Contains: metricFilter,
	})
	if err != nil {
		slog.Error("failed to fetch metric names for metadata from LM", "err", err)
		writeError(w, "execution", err.Error(), http.StatusInternalServerError)
		return
	}

	var lmResp struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(rawResp, &lmResp); err != nil {
		lmResp.Data = []string{}
	}

	metadataMap := make(map[string][]MetricMetadata)
	for _, m := range lmResp.Data {
		metadataMap[m] = []MetricMetadata{
			{
				Type: "gauge",
				Help: "Metric exported from LogicMonitor OpenTelemetry",
				Unit: "",
			},
		}
	}

	resp := struct {
		Status string                      `json:"status"`
		Data   map[string][]MetricMetadata `json:"data"`
	}{
		Status: "success",
		Data:   metadataMap,
	}

	out, err := json.Marshal(resp)
	if err != nil {
		writeError(w, "internal", err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out)
}

// // BasicAuthMiddleware enforces HTTP Basic Authentication if credentials are provided.
// func BasicAuthMiddleware(username, password string, next http.Handler) http.Handler {
// 	if username == "" && password == "" {
// 		return next
// 	}

// 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		user, pass, ok := r.BasicAuth()
// 		if !ok || user != username || pass != password {
// 			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
// 			writeError(w, "unauthorized", "invalid username or password", http.StatusUnauthorized)
// 			return
// 		}
// 		next.ServeHTTP(w, r)
// 	})
// }
