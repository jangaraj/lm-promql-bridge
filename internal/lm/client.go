package lm

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"lm-promql-bridge/internal/config"

	"golang.org/x/sync/errgroup"
)

const (
	maxQueryRangeChunks = 100
	queryRangeWorkers   = 10
)

type Client struct {
	cfg        *config.Config
	httpClient *http.Client
}

type QueryRequest struct {
	Query string `json:"query"`
	Time  string `json:"time,omitempty"`
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
	Step  string `json:"step,omitempty"`
}

type LabelsRequest struct {
	Metric string `json:"metric,omitempty"`
}

type LabelValuesRequest struct {
	Key    string `json:"key"`
	Metric string `json:"metric,omitempty"`
	Start  int64  `json:"start,omitempty"`
	End    int64  `json:"end,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type MetricNamesRequest struct {
	Start    int64  `json:"start,omitempty"`
	End      int64  `json:"end,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	Contains string `json:"contains,omitempty"`
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

func NewClient(cfg *config.Config) *Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   cfg.RequestTimeout,
		},
	}
}

func (c *Client) doRequestWithContext(ctx context.Context, path string, payload interface{}) ([]byte, error) {
	url := fmt.Sprintf("https://%s/santaba/rest%s", c.cfg.Portal, path)

	var bodyBytes []byte
	var err error
	if payload != nil {
		bodyBytes, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request payload: %w", err)
		}
	}

	if c.cfg.Debug {
		slog.Info("LM API Request",
			"method", "POST",
			"url", url,
			"payload", string(bodyBytes),
		)
	}

	var resp *http.Response
	maxRetries := 3
	backoff := 500 * time.Millisecond

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Recreate the request on every attempt because the HTTP client consumes
		// the request body when sending it.
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to create http request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Version", "3")

		epoch := strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10)

		if c.cfg.BearerToken != "" {
			req.Header.Set("Authorization", "Bearer "+c.cfg.BearerToken)
		} else {
			verb := "POST"
			requestParams := string(bodyBytes)
			signatureVars := verb + epoch + requestParams + path

			h := hmac.New(sha256.New, []byte(c.cfg.AccessKey))
			h.Write([]byte(signatureVars))
			signature := base64.StdEncoding.EncodeToString(h.Sum(nil))

			authHeader := fmt.Sprintf("LMv1 %s:%s:%s", c.cfg.AccessID, signature, epoch)
			req.Header.Set("Authorization", authHeader)
		}

		resp, err = c.httpClient.Do(req)
		if err == nil && resp.StatusCode < 500 && resp.StatusCode != 429 {
			break
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		if attempt == maxRetries {
			if err != nil {
				return nil, fmt.Errorf("request failed after %d attempts: %w", maxRetries, err)
			}
			return nil, fmt.Errorf("logicmonitor API returned status %d after %d attempts", resp.StatusCode, maxRetries)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
	}
	defer func() {
		if resp != nil {
			_ = resp.Body.Close()
		}
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if c.cfg.Debug {
		slog.Info("LM API Response",
			"url", url,
			"status_code", resp.StatusCode,
			"body", string(respBody),
		)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("logicmonitor API error [status %d]: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func (c *Client) Query(ctx context.Context, req *QueryRequest) ([]byte, error) {
	return c.doRequestWithContext(ctx, "/metrics/query", req)
}

func (c *Client) QueryRange(ctx context.Context, req *QueryRequest) ([]byte, error) {
	if req.Start == "" || req.End == "" {
		return c.doRequestWithContext(ctx, "/metrics/query-range", req)
	}

	startVal, err1 := strconv.ParseFloat(req.Start, 64)
	endVal, err2 := strconv.ParseFloat(req.End, 64)
	if err1 != nil || err2 != nil {
		return c.doRequestWithContext(ctx, "/metrics/query-range", req)
	}

	if math.IsNaN(startVal) || math.IsInf(startVal, 0) ||
		math.IsNaN(endVal) || math.IsInf(endVal, 0) {
		return nil, fmt.Errorf("query range start and end must be finite values")
	}
	if endVal < startVal {
		return nil, fmt.Errorf("query range end must not be before start")
	}

	maxChunkDuration := float64(25 * 24 * 3600)
	duration := endVal - startVal
	maxRangeDuration := maxChunkDuration * maxQueryRangeChunks
	if duration > maxRangeDuration {
		return nil, fmt.Errorf("query range exceeds maximum of %d chunks", maxQueryRangeChunks)
	}

	chunkCount := int(math.Ceil(duration / maxChunkDuration))
	if chunkCount == 0 {
		// A zero-length range still requires one upstream query.
		chunkCount = 1
	}

	if c.cfg.Debug && duration > maxChunkDuration {
		slog.Info("Parallelizing chunked QueryRange execution", "totalDurationDays", duration/86400)
	}

	type queryChunk struct {
		start float64
		end   float64
	}

	chunks := make([]queryChunk, 0, chunkCount)
	currentStart := startVal
	for i := 0; i < chunkCount; i++ {
		currentEnd := currentStart + maxChunkDuration
		if currentEnd > endVal {
			currentEnd = endVal
		}
		chunks = append(chunks, queryChunk{start: currentStart, end: currentEnd})
		currentStart = currentEnd
	}

	results := make([][]lmMetricResult, len(chunks))
	jobs := make(chan int)
	g, gCtx := errgroup.WithContext(ctx)

	workerCount := min(queryRangeWorkers, len(chunks))
	for range workerCount {
		g.Go(func() error {
			for i := range jobs {
				chunk := chunks[i]
				subReq := &QueryRequest{
					Query: req.Query,
					Time:  req.Time,
					Start: strconv.FormatFloat(chunk.start, 'f', -1, 64),
					End:   strconv.FormatFloat(chunk.end, 'f', -1, 64),
					Step:  req.Step,
				}

				respBytes, err := c.doRequestWithContext(gCtx, "/metrics/query-range", subReq)
				if err != nil {
					return err
				}

				var lmResp lmAPIResponse
				if err := json.Unmarshal(respBytes, &lmResp); err == nil && len(lmResp.Data) > 0 {
					var chunkMetrics []lmMetricResult
					if err := json.Unmarshal(lmResp.Data, &chunkMetrics); err == nil {
						results[i] = chunkMetrics
					}
				}
			}
			return nil
		})
	}

dispatch:
	for i := range chunks {
		select {
		case jobs <- i:
		case <-gCtx.Done():
			break dispatch
		}
	}
	close(jobs)

	if err := g.Wait(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var allMetrics []lmMetricResult
	for _, chunkMetrics := range results {
		allMetrics = append(allMetrics, chunkMetrics...)
	}

	seriesMap := make(map[string]*lmMetricResult)
	for _, m := range allMetrics {
		key := metricKey(m.Metric)
		existing, ok := seriesMap[key]
		if !ok {
			mCopy := m
			mCopy.Values = make([]lmValue, len(m.Values))
			copy(mCopy.Values, m.Values)
			seriesMap[key] = &mCopy
		} else {
			existing.Values = append(existing.Values, m.Values...)
		}
	}

	var mergedMetrics []lmMetricResult
	for _, m := range seriesMap {
		m.Values = sortAndDeduplicateValues(m.Values)
		mergedMetrics = append(mergedMetrics, *m)
	}

	mergedDataBytes, err := json.Marshal(mergedMetrics)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal merged query-range data: %w", err)
	}

	finalResp := lmAPIResponse{
		Data: mergedDataBytes,
	}

	return json.Marshal(finalResp)
}

func metricKey(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(labels[k])
		sb.WriteString(",")
	}
	return sb.String()
}

func sortAndDeduplicateValues(values []lmValue) []lmValue {
	if len(values) <= 1 {
		return values
	}
	sort.Slice(values, func(i, j int) bool {
		return values[i].Timestamp < values[j].Timestamp
	})
	n := 1
	for i := 1; i < len(values); i++ {
		if values[i].Timestamp != values[i-1].Timestamp {
			values[n] = values[i]
			n++
		}
	}
	return values[:n]
}

func (c *Client) Labels(ctx context.Context, req *LabelsRequest) ([]byte, error) {
	return c.doRequestWithContext(ctx, "/metrics/labels", req)
}

func (c *Client) LabelValues(ctx context.Context, req *LabelValuesRequest) ([]byte, error) {
	if req.Limit <= 0 {
		req.Limit = 200
	}
	return c.doRequestWithContext(ctx, "/metrics/label-values", req)
}

func (c *Client) MetricNames(ctx context.Context, req *MetricNamesRequest) ([]byte, error) {
	if req.Limit <= 0 {
		req.Limit = 200
	}
	return c.doRequestWithContext(ctx, "/metrics/metric-names", req)
}
