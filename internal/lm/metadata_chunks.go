package lm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"golang.org/x/sync/errgroup"
)

const metadataChunkDuration = int64(25 * 24 * 3600)

// queryTimeChunkedStrings chunks a bounded time range and merges string-array
// results returned in LogicMonitor's {"data":[...]} response envelope.
// It reports chunked=false when the range is not eligible for chunking.
func (c *Client) queryTimeChunkedStrings(
	ctx context.Context,
	path string,
	start, end int64,
	limit int,
	makeRequest func(start, end int64) interface{},
) ([]byte, bool, error) {
	// Require both bounds: a missing bound may represent an open-ended query.
	if start == 0 || end == 0 || end <= start {
		return nil, false, nil
	}

	duration := uint64(end) - uint64(start)
	if duration <= uint64(metadataChunkDuration) {
		return nil, false, nil
	}

	maxDuration := uint64(metadataChunkDuration) * uint64(maxQueryRangeChunks)
	if duration > maxDuration {
		return nil, true, fmt.Errorf(
			"metadata time range exceeds maximum of %d chunks",
			maxQueryRangeChunks,
		)
	}

	type timeChunk struct {
		start int64
		end   int64
	}

	var chunks []timeChunk
	for chunkStart := start; chunkStart < end; {
		remaining := uint64(end) - uint64(chunkStart)
		chunkEnd := end
		if remaining > uint64(metadataChunkDuration) {
			chunkEnd = chunkStart + metadataChunkDuration
		}

		chunks = append(chunks, timeChunk{start: chunkStart, end: chunkEnd})
		chunkStart = chunkEnd
	}

	results := make([][]string, len(chunks))
	jobs := make(chan int)
	group, groupCtx := errgroup.WithContext(ctx)

	workerCount := min(queryRangeWorkers, len(chunks))
	for range workerCount {
		group.Go(func() error {
			for i := range jobs {
				chunk := chunks[i]
				body, err := c.doRequestWithContext(
					groupCtx,
					path,
					makeRequest(chunk.start, chunk.end),
				)
				if err != nil {
					return fmt.Errorf("metadata chunk %d: %w", i+1, err)
				}

				var response lmAPIResponse
				if err := json.Unmarshal(body, &response); err != nil {
					return fmt.Errorf("decode metadata response for chunk %d: %w", i+1, err)
				}

				data := bytes.TrimSpace(response.Data)
				if len(data) == 0 || bytes.Equal(data, []byte("null")) {
					// An empty chunk is valid; merge results from the other chunks.
					continue
				}
				if data[0] != '[' {
					return fmt.Errorf("metadata response for chunk %d has invalid data: expected an array", i+1)
				}

				var values []string
				if err := json.Unmarshal(data, &values); err != nil {
					return fmt.Errorf("decode metadata data for chunk %d: %w", i+1, err)
				}
				results[i] = values
			}
			return nil
		})
	}

dispatch:
	for i := range chunks {
		select {
		case jobs <- i:
		case <-groupCtx.Done():
			break dispatch
		}
	}
	close(jobs)

	if err := group.Wait(); err != nil {
		return nil, true, err
	}
	if err := ctx.Err(); err != nil {
		return nil, true, err
	}

	merged := make([]string, 0)
	seen := make(map[string]struct{})
	for _, values := range results {
		for _, value := range values {
			if _, exists := seen[value]; exists {
				continue
			}
			seen[value] = struct{}{}
			merged = append(merged, value)
		}
	}

	sort.Strings(merged)
	if limit > 0 && len(merged) > limit {
		merged = merged[:limit]
	}

	data, err := json.Marshal(merged)
	if err != nil {
		return nil, true, fmt.Errorf("marshal merged metadata results: %w", err)
	}

	result, err := json.Marshal(lmAPIResponse{Data: data})
	if err != nil {
		return nil, true, fmt.Errorf("marshal merged metadata response: %w", err)
	}

	return result, true, nil
}
