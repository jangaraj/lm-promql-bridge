package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBasicAuth_RejectsMissingCredentials(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/query", nil)
	rec := httptest.NewRecorder()

	handler := basicAuth("user", "pass", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestBasicAuth_RejectsWrongCredentials(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/query", nil)
	req.SetBasicAuth("user", "wrong")
	rec := httptest.NewRecorder()

	handler := basicAuth("user", "pass", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestBasicAuth_AllowsCorrectCredentials(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/query", nil)
	req.SetBasicAuth("user", "pass")
	rec := httptest.NewRecorder()

	var reached bool
	handler := basicAuth("user", "pass", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !reached {
		t.Fatal("expected downstream handler to run")
	}
}
