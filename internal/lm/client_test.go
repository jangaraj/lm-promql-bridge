package lm

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"lm-promql-bridge/internal/config"
)

type queryRangeRoundTripper struct {
	mu           sync.Mutex
	active       int
	maxActive    int
	requestCount int
}

func (rt *queryRangeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	rt.active++
	rt.requestCount++
	if rt.active > rt.maxActive {
		rt.maxActive = rt.active
	}
	rt.mu.Unlock()

	// Keep requests in flight long enough for the test to observe concurrency.
	time.Sleep(10 * time.Millisecond)

	rt.mu.Lock()
	rt.active--
	rt.mu.Unlock()

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"data":[]}`)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestQueryRangeUsesBoundedWorkerPool(t *testing.T) {
	roundTripper := &queryRangeRoundTripper{}
	client := &Client{
		cfg: &config.Config{
			Portal:      "example.com",
			BearerToken: "test-token",
		},
		httpClient: &http.Client{Transport: roundTripper},
	}

	start := int64(0)
	end := int64(25*24*3600) * int64(queryRangeWorkers+5)
	_, err := client.QueryRange(context.Background(), &QueryRequest{
		Query: "up",
		Start: strconv.FormatInt(start, 10),
		End:   strconv.FormatInt(end, 10),
	})
	if err != nil {
		t.Fatalf("QueryRange() error = %v", err)
	}

	roundTripper.mu.Lock()
	defer roundTripper.mu.Unlock()

	wantRequests := queryRangeWorkers + 5
	if roundTripper.requestCount != wantRequests {
		t.Fatalf("request count = %d, want %d", roundTripper.requestCount, wantRequests)
	}
	if roundTripper.maxActive > queryRangeWorkers {
		t.Fatalf("maximum concurrent requests = %d, want at most %d", roundTripper.maxActive, queryRangeWorkers)
	}
	if roundTripper.maxActive < 2 {
		t.Fatalf("maximum concurrent requests = %d, expected parallel requests", roundTripper.maxActive)
	}
}

func TestQueryRangeRejectsExcessiveChunkCount(t *testing.T) {
	roundTripper := &queryRangeRoundTripper{}
	client := &Client{
		cfg: &config.Config{
			Portal:      "example.com",
			BearerToken: "test-token",
		},
		httpClient: &http.Client{Transport: roundTripper},
	}

	maxChunkDuration := int64(25 * 24 * 3600)
	req := &QueryRequest{
		Query: "up",
		Start: "0",
		End:   strconv.FormatInt(maxChunkDuration*int64(maxQueryRangeChunks)+1, 10),
	}
	if _, err := client.QueryRange(context.Background(), req); err == nil {
		t.Fatal("QueryRange() expected an error for an excessive range")
	}

	roundTripper.mu.Lock()
	defer roundTripper.mu.Unlock()
	if roundTripper.requestCount != 0 {
		t.Fatalf("request count = %d, want 0 for rejected range", roundTripper.requestCount)
	}
}
