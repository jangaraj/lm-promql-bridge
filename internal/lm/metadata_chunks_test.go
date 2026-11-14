package lm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"lm-promql-bridge/internal/config"
)

type metadataChunkRoundTripper struct {
	// return payload keyed by requested chunk start/end
	payloads map[string]string
}

func (rt *metadataChunkRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = req.Body.Close()
	}()

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	start, _ := payload["start"].(float64)
	end, _ := payload["end"].(float64)

	key := strings.Join([]string{
		"start=" + toString(int64(start)),
		"end=" + toString(int64(end)),
	}, ";")

	respBody, ok := rt.payloads[key]
	if !ok {
		respBody = `{"data":[]}`
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(respBody)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func toString(v int64) string {
	return strconv.FormatInt(v, 10)
}

func TestQueryTimeChunkedStrings_MergesSortedUniqueValues(t *testing.T) {
	t.Run("sorted unique values with limit applied", func(t *testing.T) {
		start := int64(1_000_000_000)
		end := start + metadataChunkDuration*2

		rt := &metadataChunkRoundTripper{
			payloads: map[string]string{
				"start=1000000000;end=1002160000": `{"data":["zeta","alpha"]}`,
				"start=1002160000;end=1004320000": `{"data":["alpha","beta","gamma"]}`,
			},
		}

		client := &Client{
			cfg: &config.Config{
				Portal:      "example.com",
				BearerToken: "token",
			},
			httpClient: &http.Client{Transport: rt},
		}

		result, chunked, err := client.queryTimeChunkedStrings(
			context.Background(),
			"/metrics/metric-names",
			start,
			end,
			10,
			func(start, end int64) interface{} {
				return &MetricNamesRequest{
					Start: start,
					End:   end,
					Limit: 100,
				}
			},
		)
		if err != nil {
			t.Fatalf("queryTimeChunkedStrings() unexpected error: %v", err)
		}
		if !chunked {
			t.Fatal("queryTimeChunkedStrings() expected chunked=true for long range")
		}

		var resp lmAPIResponse
		if err := json.Unmarshal(result, &resp); err != nil {
			t.Fatalf("Unmarshal response: %v", err)
		}

		var got []string
		if err := json.Unmarshal(resp.Data, &got); err != nil {
			t.Fatalf("Unmarshal data: %v", err)
		}

		want := []string{"alpha", "beta", "gamma", "zeta"}
		if len(got) != len(want) {
			t.Fatalf("len(got) = %d, want %d; got=%v", len(got), len(want), got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("got[%d] = %q, want %q; got=%v", i, got[i], want[i], got)
			}
		}
	})
}

func TestQueryTimeChunkedStrings_IgnoresEmptyChunks(t *testing.T) {
	start := int64(1_000_000_000)
	end := start + metadataChunkDuration*2

	rt := &metadataChunkRoundTripper{
		payloads: map[string]string{
			"start=1000000000;end=1002160000": `{"data":null}`,
			"start=1002160000;end=1004320000": `{"data":["beta","alpha"]}`,
		},
	}

	client := &Client{
		cfg: &config.Config{
			Portal:      "example.com",
			BearerToken: "token",
		},
		httpClient: &http.Client{Transport: rt},
	}

	result, chunked, err := client.queryTimeChunkedStrings(
		context.Background(),
		"/metrics/metric-names",
		start,
		end,
		10,
		func(start, end int64) interface{} {
			return &MetricNamesRequest{
				Start: start,
				End:   end,
				Limit: 100,
			}
		},
	)
	if err != nil {
		t.Fatalf("queryTimeChunkedStrings() unexpected error: %v", err)
	}
	if !chunked {
		t.Fatal("expected chunked=true")
	}

	var resp lmAPIResponse
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("Unmarshal response: %v", err)
	}

	var got []string
	if err := json.Unmarshal(resp.Data, &got); err != nil {
		t.Fatalf("Unmarshal data: %v", err)
	}

	want := []string{"alpha", "beta"}
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d; got=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q; got=%v", i, got[i], want[i], got)
		}
	}
}

func TestQueryTimeChunkedStrings_RespectsLimit(t *testing.T) {
	start := int64(1_000_000_000)
	end := start + metadataChunkDuration*2

	rt := &metadataChunkRoundTripper{
		payloads: map[string]string{
			"start=1000000000;end=1002160000": `{"data":["zeta","alpha"]}`,
			"start=1002160000;end=1004320000": `{"data":["alpha","beta","gamma"]}`,
		},
	}

	client := &Client{
		cfg: &config.Config{
			Portal:      "example.com",
			BearerToken: "token",
		},
		httpClient: &http.Client{Transport: rt},
	}

	result, chunked, err := client.queryTimeChunkedStrings(
		context.Background(),
		"/metrics/metric-names",
		start,
		end,
		2,
		func(start, end int64) interface{} {
			return &MetricNamesRequest{
				Start: start,
				End:   end,
				Limit: 100,
			}
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !chunked {
		t.Fatal("expected chunked=true")
	}

	var resp lmAPIResponse
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("Unmarshal response: %v", err)
	}

	var got []string
	if err := json.Unmarshal(resp.Data, &got); err != nil {
		t.Fatalf("Unmarshal data: %v", err)
	}

	want := []string{"alpha", "beta"}
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d; got=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q; got=%v", i, got[i], want[i], got)
		}
	}
}
