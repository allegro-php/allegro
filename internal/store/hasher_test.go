package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHashBytes(t *testing.T) {
	// SHA-256 of empty string
	got := HashBytes([]byte(""))
	want := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got != want {
		t.Errorf("HashBytes empty = %q, want %q", got, want)
	}

	got2 := HashBytes([]byte("hello"))
	want2 := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got2 != want2 {
		t.Errorf("HashBytes hello = %q, want %q", got2, want2)
	}
}

func TestHashFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := HashFile(path)
	if err != nil {
		t.Fatalf("HashFile: %v", err)
	}
	want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got != want {
		t.Errorf("HashFile = %q, want %q", got, want)
	}
}

func TestHashFileNotFound(t *testing.T) {
	_, err := HashFile("/nonexistent/file")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestShardPrefix(t *testing.T) {
	if got := ShardPrefix("abcdef1234"); got != "ab" {
		t.Errorf("ShardPrefix = %q, want ab", got)
	}
	if got := ShardPrefix("a"); got != "a" {
		t.Errorf("ShardPrefix short = %q, want a", got)
	}
	if got := ShardPrefix(""); got != "" {
		t.Errorf("ShardPrefix empty = %q, want empty", got)
	}
}
