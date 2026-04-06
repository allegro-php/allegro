package fetcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDownloadFullSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	}))
	defer srv.Close()

	c := NewClient()
	resp, err := c.DownloadFull(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("DownloadFull: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if string(resp.Body) != "hello" {
		t.Errorf("body = %q, want hello", resp.Body)
	}
}

func TestDownloadFull404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()

	c := NewClient()
	resp, err := c.DownloadFull(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("DownloadFull: %v", err)
	}
	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestIsRetryable(t *testing.T) {
	if !IsRetryable(500) {
		t.Error("500 should be retryable")
	}
	if !IsRetryable(503) {
		t.Error("503 should be retryable")
	}
	if !IsRetryable(429) {
		t.Error("429 should be retryable")
	}
	if IsRetryable(404) {
		t.Error("404 should not be retryable")
	}
	if IsRetryable(200) {
		t.Error("200 should not be retryable")
	}
}

func TestRetryAfterDuration(t *testing.T) {
	if d := RetryAfterDuration(""); d != 0 {
		t.Errorf("empty = %v, want 0", d)
	}
	if d := RetryAfterDuration("30"); d.Seconds() != 30 {
		t.Errorf("30 = %v, want 30s", d)
	}
	if d := RetryAfterDuration("120"); d.Seconds() != 60 {
		t.Errorf("120 should be capped at 60s, got %v", d)
	}
	if d := RetryAfterDuration("invalid"); d != 0 {
		t.Errorf("invalid = %v, want 0", d)
	}
}

func TestDownloadWithRetrySuccess(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 2 {
			w.WriteHeader(500)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient()
	body, err := c.DownloadWithRetry(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("DownloadWithRetry: %v", err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want ok", body)
	}
}

func TestDownloadWithRetry4xxNoRetry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
	}))
	defer srv.Close()

	c := NewClient()
	_, err := c.DownloadWithRetry(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error for 403")
	}
}
