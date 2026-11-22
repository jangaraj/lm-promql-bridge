package lm

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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

func TestRateLimitRetryDelay(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		remaining string
		window    string
		wantRetry bool
		wantDelay time.Duration
	}{
		{
			name:      "window ends within two seconds",
			status:    http.StatusTooManyRequests,
			remaining: "0",
			window:    "1",
			wantRetry: true,
			wantDelay: time.Second,
		},
		{
			name:      "window exceeds two seconds",
			status:    http.StatusTooManyRequests,
			remaining: "0",
			window:    "3",
		},
		{
			name:   "missing remaining header",
			status: http.StatusTooManyRequests,
			window: "1",
		},
		{
			name:      "remaining quota is nonzero",
			status:    http.StatusTooManyRequests,
			remaining: "1",
			window:    "1",
		},
		{
			name:      "missing window header",
			status:    http.StatusTooManyRequests,
			remaining: "0",
		},
		{
			name:      "invalid window header",
			status:    http.StatusTooManyRequests,
			remaining: "0",
			window:    "invalid",
		},
		{
			name:      "not a rate-limit response",
			status:    http.StatusServiceUnavailable,
			remaining: "0",
			window:    "1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				StatusCode: tt.status,
				Header: http.Header{
					"X-Rate-Limit-Remaining": []string{tt.remaining},
					"X-Rate-Limit-Window":    []string{tt.window},
				},
			}

			delay, retry := rateLimitRetryDelay(resp)
			if retry != tt.wantRetry {
				t.Fatalf("retry = %t, want %t", retry, tt.wantRetry)
			}
			if delay != tt.wantDelay {
				t.Fatalf("delay = %s, want %s", delay, tt.wantDelay)
			}
		})
	}
}

type staticRoundTripper func(*http.Request) (*http.Response, error)

func (f staticRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestDoRequestWithContext_RejectsOversizedResponse(t *testing.T) {
	oversized := bytes.Repeat([]byte("a"), int(maxUpstreamResponseBytes)+1)
	client := &Client{
		cfg: &config.Config{
			Portal:      "example.com",
			BearerToken: "test-token",
		},
		httpClient: &http.Client{
			Transport: staticRoundTripper(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader(oversized)),
					Header:     make(http.Header),
					Request:    req,
				}, nil
			}),
		},
	}

	_, err := client.doRequestWithContext(context.Background(), "/metrics/query", &QueryRequest{Query: "up"})
	if err == nil {
		t.Fatal("expected error for oversized upstream response")
	}
	if !strings.Contains(err.Error(), "exceeded maximum size") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMetricKey_CollisionResistant(t *testing.T) {
	k1 := metricKey(map[string]string{"a": "b,c=d"})
	k2 := metricKey(map[string]string{"a": "b", "c": "d"})
	if k1 == k2 {
		t.Fatalf("metricKey collided: %q == %q", k1, k2)
	}
}

func TestQueryRange_InvalidRangeReturnsErrInvalidArgument(t *testing.T) {
	client := &Client{
		cfg: &config.Config{
			Portal:      "example.com",
			BearerToken: "test-token",
		},
		httpClient: &http.Client{},
	}

	_, err := client.QueryRange(context.Background(), &QueryRequest{
		Query: "up",
		Start: "200",
		End:   "100",
		Step:  "60",
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got %v", err)
	}
}

func TestQueryRange_MergesAndDeduplicatesChunks(t *testing.T) {
	var call atomic.Int32
	client := &Client{
		cfg: &config.Config{
			Portal:      "example.com",
			BearerToken: "test-token",
		},
		httpClient: &http.Client{
			Transport: staticRoundTripper(func(req *http.Request) (*http.Response, error) {
				n := call.Add(1)
				body := `{"data":[{"metric":{"__name__":"up","host":"a"},"values":[{"timestamp":100,"value":"1"},{"timestamp":200,"value":"2"}]}]}`
				if n == 2 {
					body = `{"data":[{"metric":{"__name__":"up","host":"a"},"values":[{"timestamp":200,"value":"2"},{"timestamp":300,"value":"3"}]}]}`
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(body)),
					Header:     make(http.Header),
					Request:    req,
				}, nil
			}),
		},
	}

	end := strconv.FormatInt(26*24*3600, 10)
	resp, err := client.QueryRange(context.Background(), &QueryRequest{
		Query: "up",
		Start: "0",
		End:   end,
		Step:  "60",
	})
	if err != nil {
		t.Fatalf("QueryRange() error = %v", err)
	}
	got := string(resp)
	if strings.Count(got, `"timestamp":200`) != 1 {
		t.Fatalf("expected deduplicated timestamp 200, got %s", got)
	}
	if !strings.Contains(got, `"timestamp":100`) || !strings.Contains(got, `"timestamp":300`) {
		t.Fatalf("expected timestamps 100 and 300 in merged response, got %s", got)
	}
}
