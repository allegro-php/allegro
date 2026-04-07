package fetcher

import (
	"io"
	"net"
	"net/http"
	"time"
)

// Client wraps HTTP downloading with timeouts.
type Client struct {
	httpClient *http.Client
}

// NewClient creates a fetcher client with a tuned transport.
// This is the single constructor for all HTTP clients — transport
// tuning in this function applies globally (Pool workers, re-downloads, etc.).
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
			Transport: &http.Transport{
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   32,
				IdleConnTimeout:       90 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
				TLSHandshakeTimeout:  10 * time.Second,
				ForceAttemptHTTP2:     true,
				DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
			},
		},
	}
}


const maxResponseSize = 512 << 20 // 512 MiB max response body

func readAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, maxResponseSize))
}
