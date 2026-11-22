package prom

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lm-promql-bridge/internal/config"
	"lm-promql-bridge/internal/lm"
)

func TestParseInstantQueryRequest(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		target      string
		contentType string
		body        string
		wantQuery   string
		wantTime    string
	}{
		{
			name:        "JSON POST",
			method:      http.MethodPost,
			target:      "/api/v1/query",
			contentType: "application/json; charset=utf-8",
			body:        `{"query":"up","time":"123"}`,
			wantQuery:   "up",
			wantTime:    "123",
		},
		{
			name:        "JSON POST numeric time",
			method:      http.MethodPost,
			target:      "/api/v1/query",
			contentType: "application/json",
			body:        `{"query":"up","time":123.5}`,
			wantQuery:   "up",
			wantTime:    "123.5",
		},
		{
			name:        "form-encoded POST",
			method:      http.MethodPost,
			target:      "/api/v1/query",
			contentType: "application/x-www-form-urlencoded",
			body:        "query=up&time=123",
			wantQuery:   "up",
			wantTime:    "123",
		},
		{
			name:      "GET query parameters",
			method:    http.MethodGet,
			target:    "/api/v1/query?query=up&time=123",
			wantQuery: "up",
			wantTime:  "123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			gotQuery, gotTime := parseInstantQueryRequest(req)
			if gotQuery != tt.wantQuery || gotTime != tt.wantTime {
				t.Fatalf("parseInstantQueryRequest() = (%q, %q), want (%q, %q)",
					gotQuery, gotTime, tt.wantQuery, tt.wantTime)
			}
		})
	}
}

func TestParseTimestampParam(t *testing.T) {
	tests := []struct {
		input  string
		want   int64
		wantOK bool
	}{
		{"1700000000", 1700000000, true},
		{"1700000000.75", 1700000000, true},
		{"2026-01-02T03:04:05Z", 1767323045, true},
		{"", 0, false},
		{"not-a-time", 0, false},
	}

	for _, tt := range tests {
		got, ok := parseTimestampParam(tt.input)
		if ok != tt.wantOK || got != tt.want {
			t.Fatalf("parseTimestampParam(%q) = (%d, %t), want (%d, %t)", tt.input, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestLimitRequestBody_RejectsOversizedBody(t *testing.T) {
	h := LimitRequestBody(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	oversized := strings.Repeat("x", int(maxRequestBodyBytes)+1)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/query", strings.NewReader(oversized))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestHandleQueryRange_JSONNumericAndInvalidRange(t *testing.T) {
	cfg := &config.Config{
		Portal:      "example.com",
		BearerToken: "token",
	}
	h := NewHandler(lm.NewClient(cfg), false)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/query_range",
		strings.NewReader(`{"query":"up","start":200,"end":100,"step":60}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleQueryRange(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestTranslateSeriesResponse_ReturnsLabelSetObjects(t *testing.T) {
	out, err := translateSeriesResponse([]byte(`{"data":["cpu_usage","mem_usage"]}`))
	if err != nil {
		t.Fatalf("translateSeriesResponse() error = %v", err)
	}

	var resp struct {
		Status string              `json:"status"`
		Data   []map[string]string `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Status != "success" || len(resp.Data) != 2 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.Data[0]["__name__"] != "cpu_usage" || resp.Data[1]["__name__"] != "mem_usage" {
		t.Fatalf("unexpected series data: %+v", resp.Data)
	}
}

func TestTranslateLMResponse(t *testing.T) {
	t.Run("vector", func(t *testing.T) {
		raw := []byte(`{"data":[{"metric":{"__name__":"up"},"value":{"timestamp":1700000000,"value":"1"}}],"meta":{"message":"partial"}}`)
		out, err := TranslateLMResponse(raw, "vector", false, "/api/v1/query")
		if err != nil {
			t.Fatalf("TranslateLMResponse() error = %v", err)
		}
		var resp PromResponse
		if err := json.Unmarshal(out, &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if resp.Status != "success" || resp.Data.ResultType != "vector" {
			t.Fatalf("unexpected response: %+v", resp)
		}
		if len(resp.Warnings) != 1 || resp.Warnings[0] != "partial" {
			t.Fatalf("unexpected warnings: %+v", resp.Warnings)
		}
	})

	t.Run("matrix", func(t *testing.T) {
		raw := []byte(`{"data":[{"metric":{"__name__":"up"},"values":[{"timestamp":100,"value":"1"},{"timestamp":200,"value":"2"}]}]}`)
		out, err := TranslateLMResponse(raw, "matrix", false, "/api/v1/query_range")
		if err != nil {
			t.Fatalf("TranslateLMResponse() error = %v", err)
		}
		var resp PromResponse
		if err := json.Unmarshal(out, &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if resp.Data.ResultType != "matrix" {
			t.Fatalf("resultType = %q, want matrix", resp.Data.ResultType)
		}
	})

	t.Run("null data with message", func(t *testing.T) {
		raw := []byte(`{"data":null,"meta":{"message":"no data"}}`)
		out, err := TranslateLMResponse(raw, "vector", false, "/api/v1/query")
		if err != nil {
			t.Fatalf("TranslateLMResponse() error = %v", err)
		}
		var resp PromResponse
		if err := json.Unmarshal(out, &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if len(resp.Warnings) != 1 || resp.Warnings[0] != "no data" {
			t.Fatalf("unexpected warnings: %+v", resp.Warnings)
		}
	})
}

func TestTranslateMetadataResponse(t *testing.T) {
	raw := []byte(`{"data":["host","job"]}`)
	out, err := TranslateMetadataResponse(raw, false, "/api/v1/labels")
	if err != nil {
		t.Fatalf("TranslateMetadataResponse() error = %v", err)
	}
	var resp PromMetaResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Status != "success" {
		t.Fatalf("status = %q, want success", resp.Status)
	}
}
