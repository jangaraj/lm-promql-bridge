package prom

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
