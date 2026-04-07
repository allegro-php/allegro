package fetcher

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
)

// drainCh collects all results from a download channel into a slice.
func drainCh(ch <-chan DownloadResult) []DownloadResult {
	var results []DownloadResult
	for r := range ch {
		results = append(results, r)
	}
	return results
}

func TestVerifySHA1(t *testing.T) {
	data := []byte("hello world")
	h := sha1.Sum(data)
	expected := hex.EncodeToString(h[:])

	if err := verifySHA1(data, expected); err != nil {
		t.Errorf("valid SHA-1 failed: %v", err)
	}

	if err := verifySHA1(data, "badhash"); err == nil {
		t.Error("expected error for bad hash")
	}
}

func TestVerifySHA1Empty(t *testing.T) {
	if err := verifySHA1([]byte("data"), ""); err == nil {
		t.Error("empty expected should fail")
	}
}

func TestPoolDownload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("package-data"))
	}))
	defer srv.Close()

	pool := NewPool(2)
	tasks := []DownloadTask{
		{Name: "a/b", URL: srv.URL + "/a", Shasum: ""},
		{Name: "c/d", URL: srv.URL + "/c", Shasum: ""},
	}

	results := drainCh(pool.Download(context.Background(), tasks, 2))
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}

	for _, r := range results {
		if r.Error != nil {
			t.Errorf("download %s: %v", r.Task.Name, r.Error)
		}
		if string(r.Data) != "package-data" {
			t.Errorf("data = %q", r.Data)
		}
	}
}

func TestPoolDownloadWithSHA1(t *testing.T) {
	data := []byte("archive-content")
	h := sha1.Sum(data)
	shasum := hex.EncodeToString(h[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer srv.Close()

	pool := NewPool(1)
	tasks := []DownloadTask{
		{Name: "pkg/a", URL: srv.URL, Shasum: shasum},
	}

	results := drainCh(pool.Download(context.Background(), tasks, 1))
	if results[0].Error != nil {
		t.Errorf("download with valid shasum: %v", results[0].Error)
	}
}
func TestPoolDownloadSHA1Mismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("bad data"))
	}))
	defer srv.Close()

	pool := NewPool(1)
	_, err := pool.DownloadOne(context.Background(), DownloadTask{
		Name: "pkg/a", URL: srv.URL, Shasum: "0000000000000000000000000000000000000000",
	})
	if err == nil {
		t.Error("expected SHA-1 mismatch error")
	}
}

func TestPoolCircuitBreaker(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(500)
	}))
	defer srv.Close()

	pool := NewPool(1)
	tasks := make([]DownloadTask, 5)
	for i := range tasks {
		tasks[i] = DownloadTask{Name: "pkg/" + string(rune('a'+i)), URL: srv.URL}
	}

	results := drainCh(pool.Download(context.Background(), tasks, 5))
	for _, r := range results {
		if r.Error == nil {
			t.Error("expected error")
		}
	}
}

func TestPoolDownloadEmpty(t *testing.T) {
	pool := NewPool(2)
	results := drainCh(pool.Download(context.Background(), nil, 1))
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestPoolDownloadOne(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("single-pkg"))
	}))
	defer srv.Close()

	pool := NewPool(1)
	r, err := pool.DownloadOne(context.Background(), DownloadTask{
		Name: "test/pkg", URL: srv.URL,
	})
	if err != nil {
		t.Fatalf("DownloadOne: %v", err)
	}
	if string(r.Data) != "single-pkg" {
		t.Errorf("data = %q, want single-pkg", r.Data)
	}
}

func TestPoolDownloadOneError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()

	pool := NewPool(1)
	_, err := pool.DownloadOne(context.Background(), DownloadTask{
		Name: "bad/pkg", URL: srv.URL,
	})
	if err == nil {
		t.Error("expected error from DownloadOne")
	}
}

func TestPoolDownloadOneCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("data"))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	pool := NewPool(1)
	_, err := pool.DownloadOne(ctx, DownloadTask{
		Name: "cancel/pkg", URL: srv.URL,
	})
	if err == nil {
		t.Error("expected error from cancelled DownloadOne")
	}
}
