package store

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func createTestZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte(content))
	}
	w.Close()
	return buf.Bytes()
}

func createTestTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	buf := new(bytes.Buffer)
	gw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gw)
	for name, content := range files {
		tw.WriteHeader(&tar.Header{
			Name: name,
			Size: int64(len(content)),
			Mode: 0644,
		})
		tw.Write([]byte(content))
	}
	tw.Close()
	gw.Close()
	return buf.Bytes()
}

func TestExtractZip(t *testing.T) {
	data := createTestZip(t, map[string]string{
		"src/Logger.php": "<?php class Logger {}",
		"README.md":      "# Hello",
	})

	dir := t.TempDir()
	if err := ExtractZip(data, dir); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "src/Logger.php"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "<?php class Logger {}" {
		t.Errorf("content = %q", content)
	}
}

func TestExtractGzip(t *testing.T) {
	data := createTestTarGz(t, map[string]string{
		"src/App.php": "<?php class App {}",
	})

	dir := t.TempDir()
	if err := ExtractGzip(data, dir); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "src/App.php"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "<?php class App {}" {
		t.Errorf("content = %q", content)
	}
}

func TestStripTopLevelDir(t *testing.T) {
	dir := t.TempDir()
	// Create: topdir/src/file.php
	os.MkdirAll(filepath.Join(dir, "monolog-abc123/src"), 0755)
	os.WriteFile(filepath.Join(dir, "monolog-abc123/src/Logger.php"), []byte("php"), 0644)

	if err := StripTopLevelDir(dir); err != nil {
		t.Fatal(err)
	}

	// Should now be: src/Logger.php (not monolog-abc123/src/Logger.php)
	if _, err := os.Stat(filepath.Join(dir, "src/Logger.php")); err != nil {
		t.Error("file should be at src/Logger.php after stripping")
	}
	if _, err := os.Stat(filepath.Join(dir, "monolog-abc123")); err == nil {
		t.Error("top-level dir should be removed")
	}
}

func TestStripTopLevelDirMultipleEntries(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("b"), 0644)

	// Should not strip when multiple top-level entries
	if err := StripTopLevelDir(dir); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "file1.txt")); err != nil {
		t.Error("file1.txt should still exist")
	}
}

func TestStripTopLevelDirEmpty(t *testing.T) {
	dir := t.TempDir()
	err := StripTopLevelDir(dir)
	if err == nil {
		t.Error("expected error for empty archive")
	}
}

func TestExtractByType(t *testing.T) {
	zipData := createTestZip(t, map[string]string{"a.txt": "hello"})

	dir := t.TempDir()
	if err := ExtractByType(zipData, "zip", dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); err != nil {
		t.Error("zip extraction failed")
	}
}

func TestExtractByTypeUnsupported(t *testing.T) {
	err := ExtractByType(nil, "rar", t.TempDir())
	if err == nil {
		t.Error("expected error for unsupported type")
	}
}
