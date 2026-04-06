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


const maxResponseSize = 512 << 20 // 512 MiB max response body

func readAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, maxResponseSize))
}
