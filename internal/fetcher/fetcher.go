package fetcher

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client wraps HTTP downloading with timeouts.
type Client struct {
	httpClient *http.Client
}

// NewClient creates a fetcher client with spec timeouts.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // response body timeout
			Transport: &http.Transport{
				ResponseHeaderTimeout: 30 * time.Second, // connection timeout
			},
		},
	}
}

// Download fetches a URL and returns its body bytes.
func (c *Client) Download(ctx context.Context, url string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read body %s: %w", url, err)
	}

	return body, resp.StatusCode, nil
}
