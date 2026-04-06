package fetcher

import (
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

func readAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}
