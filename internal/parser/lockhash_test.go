package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComputeLockHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "composer.lock")
	content := `{"packages":[]}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	hash, err := ComputeLockHash(path)
	if err != nil {
		t.Fatalf("ComputeLockHash: %v", err)
	}
	if !strings.HasPrefix(hash, "sha256:") {
		t.Errorf("hash = %q, want sha256: prefix", hash)
	}
	if len(hash) != 7+64 { // "sha256:" + 64 hex chars
		t.Errorf("hash length = %d, want %d", len(hash), 7+64)
	}
}

func TestComputeLockHashMissing(t *testing.T) {
	_, err := ComputeLockHash("/nonexistent/composer.lock")
	if err == nil {
		t.Error("expected error for missing file")
	}
}
