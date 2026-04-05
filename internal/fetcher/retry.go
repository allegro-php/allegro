package fetcher

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

const maxRetries = 3

var backoffSchedule = []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}

// IsRetryable returns true for HTTP status codes that should be retried.
func IsRetryable(statusCode int) bool {
	if statusCode >= 500 {
		return true
	}
	if statusCode == 429 {
		return true
	}
	return false
}

// RetryAfterDuration parses the Retry-After header value.
// Returns 0 if absent or unparseable. Caps at 60s.
func RetryAfterDuration(headerValue string) time.Duration {
	if headerValue == "" {
		return 0
	}
	seconds, err := strconv.Atoi(headerValue)
	if err != nil {
		return 0
	}
	d := time.Duration(seconds) * time.Second
	if d > 60*time.Second {
		d = 60 * time.Second
	}
	return d
}

// DownloadResult holds response details for retry decisions.
type DownloadResponse struct {
	Body       []byte
	StatusCode int
	Headers    http.Header
}

// DownloadFull fetches a URL and returns body, status, headers.
func (c *Client) DownloadFull(ctx context.Context, url string) (*DownloadResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := readAll(resp.Body)
	if err != nil {
		return &DownloadResponse{StatusCode: resp.StatusCode, Headers: resp.Header},
			fmt.Errorf("read body %s: %w", url, err)
	}

	return &DownloadResponse{Body: body, StatusCode: resp.StatusCode, Headers: resp.Header}, nil
}

// DownloadWithRetry downloads a URL with retry logic per spec.
// Max 3 retries; 429 respects Retry-After; 4xx (except 429) not retried.
func (c *Client) DownloadWithRetry(ctx context.Context, url string) ([]byte, error) {
	var lastErr error

	for attempt := 0; attempt < maxRetries+1; attempt++ {
		// Backoff before retries (not before first attempt)
		if attempt > 0 {
			backoffIdx := attempt - 1
			if backoffIdx >= len(backoffSchedule) {
				backoffIdx = len(backoffSchedule) - 1
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoffSchedule[backoffIdx]):
			}
		}

		resp, err := c.DownloadFull(ctx, url)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp.Body, nil
		}

		if resp.StatusCode == 429 {
			// Respect Retry-After header (counts as one retry)
			retryAfter := RetryAfterDuration(resp.Headers.Get("Retry-After"))
			if retryAfter > 0 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(retryAfter):
				}
			}
			lastErr = fmt.Errorf("HTTP 429 for %s", url)
			continue
		}

		if !IsRetryable(resp.StatusCode) {
			return nil, fmt.Errorf("HTTP %d for %s (not retryable)", resp.StatusCode, url)
		}

		lastErr = fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}
	return nil, fmt.Errorf("download failed after %d retries: %w", maxRetries, lastErr)
}
