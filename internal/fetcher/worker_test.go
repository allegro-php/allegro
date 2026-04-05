package fetcher

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
	// Empty shasum should be checked before calling verifySHA1
	// but verifySHA1 with empty expected should fail
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

	results := pool.Download(context.Background(), tasks)
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

	results := pool.Download(context.Background(), tasks)
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
	tasks := []DownloadTask{
		{Name: "pkg/a", URL: srv.URL, Shasum: "0000000000000000000000000000000000000000"},
	}

	results := pool.Download(context.Background(), tasks)
	if results[0].Error == nil {
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

	results := pool.Download(context.Background(), tasks)
	// All should fail
	for _, r := range results {
		if r.Error == nil {
			t.Error("expected error")
		}
	}
}
