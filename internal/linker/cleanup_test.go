package linker

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanStaleVendorDirs(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "vendor.allegro.old"), 0755)
	os.MkdirAll(filepath.Join(dir, "vendor.allegro.tmp"), 0755)

	CleanStaleVendorDirs(dir)

	if _, err := os.Stat(filepath.Join(dir, "vendor.allegro.old")); err == nil {
		t.Error("vendor.allegro.old should be removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor.allegro.tmp")); err == nil {
		t.Error("vendor.allegro.tmp should be removed")
	}
}

func TestCleanStaleStoreTmpCurrentPID(t *testing.T) {
	dir := t.TempDir()
	tmpDir := filepath.Join(dir, fmt.Sprintf("tmp-%d-abc123", os.Getpid()))
	os.MkdirAll(tmpDir, 0755)

	CleanStaleStoreTmp(dir)

	if _, err := os.Stat(tmpDir); err == nil {
		t.Error("own PID tmp dir should be removed")
	}
}

func TestCleanStaleStoreTmpOldDir(t *testing.T) {
	dir := t.TempDir()
	tmpDir := filepath.Join(dir, "tmp-99999-old")
	os.MkdirAll(tmpDir, 0755)
	// Set mtime to 2 hours ago
	past := time.Now().Add(-2 * time.Hour)
	os.Chtimes(tmpDir, past, past)

	CleanStaleStoreTmp(dir)

	if _, err := os.Stat(tmpDir); err == nil {
		t.Error("old tmp dir should be removed")
	}
}

func TestCleanStaleStoreTmpPreservesRecent(t *testing.T) {
	dir := t.TempDir()
	tmpDir := filepath.Join(dir, "tmp-99999-recent")
	os.MkdirAll(tmpDir, 0755)
	// Recent — should be preserved

	CleanStaleStoreTmp(dir)

	if _, err := os.Stat(tmpDir); err != nil {
		t.Error("recent other-process tmp dir should be preserved")
	}
}

func TestCreateTempDir(t *testing.T) {
	dir := t.TempDir()
	path, err := CreateTempDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("temp dir not created: %v", err)
	}
}

func TestParsePIDFromTmpName(t *testing.T) {
	pid, ok := ParsePIDFromTmpName("tmp-12345-abc")
	if !ok || pid != 12345 {
		t.Errorf("got pid=%d ok=%v, want 12345 true", pid, ok)
	}

	_, ok = ParsePIDFromTmpName("notmp")
	if ok {
		t.Error("should fail for non-tmp prefix")
	}
}
