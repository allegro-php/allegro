package fetcher

import (
	"context"
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
			Timeout: 5 * time.Minute,
			Transport: &http.Transport{
				ResponseHeaderTimeout: 30 * time.Second,
			},
		},
	}
}

// Download fetches a URL and returns its body bytes.
func (c *Client) Download(ctx context.Context, url string) ([]byte, int, error) {
	resp, err := c.DownloadFull(ctx, url)
	if err != nil {
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
		}
		return nil, statusCode, err
	}
	return resp.Body, resp.StatusCode, nil
}

func readAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}
