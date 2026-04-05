package fetcher

import (
	"context"
	"fmt"
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

// DownloadWithRetry downloads a URL with retry logic per spec.
func (c *Client) DownloadWithRetry(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoffSchedule[attempt-1]):
			}
		}

		body, status, err := c.Download(ctx, url)
		if err != nil {
			lastErr = err
			continue
		}

		if status >= 200 && status < 300 {
			return body, nil
		}

		if status == 429 {
			// 429 consumes a retry
			lastErr = fmt.Errorf("HTTP 429 for %s", url)
			continue
		}

		if !IsRetryable(status) {
			return nil, fmt.Errorf("HTTP %d for %s (not retryable)", status, url)
		}

		lastErr = fmt.Errorf("HTTP %d for %s", status, url)
	}
	return nil, fmt.Errorf("download failed after %d retries: %w", maxRetries, lastErr)
}
