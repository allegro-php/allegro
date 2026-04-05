package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, "store")
	s := New(root)
	if err := s.EnsureDirectories(); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestEnsureDirectories(t *testing.T) {
	s := newTestStore(t)
	for _, sub := range []string{"files", "packages", "tmp"} {
		path := filepath.Join(s.Root, sub)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("directory %s not created", sub)
		}
	}
}

func TestEnsureMetadataCreatesNew(t *testing.T) {
	s := newTestStore(t)
	if err := s.EnsureMetadata(); err != nil {
		t.Fatal(err)
	}
	// Should be able to read it back
	if err := s.EnsureMetadata(); err != nil {
		t.Fatalf("second call: %v", err)
	}
}

func TestEnsureMetadataVersionTooHigh(t *testing.T) {
	s := newTestStore(t)
	// Write a future version
	meta := StoreMetadata{StoreVersion: 999, CreatedAt: time.Now()}
	path := s.MetadataPath()
	os.MkdirAll(filepath.Dir(path), 0755)
	writeJSONAtomic(path, meta)

	err := s.EnsureMetadata()
	if err == nil {
		t.Fatal("expected error for future version")
	}
}

func TestStoreAndReadFile(t *testing.T) {
	s := newTestStore(t)

	// Create a temp file
	tmpFile := filepath.Join(s.TmpDir(), "test.txt")
	os.WriteFile(tmpFile, []byte("hello"), 0644)

	hash := HashBytes([]byte("hello"))

	if err := s.StoreFile(tmpFile, hash, false); err != nil {
		t.Fatalf("StoreFile: %v", err)
	}

	if !s.FileExists(hash) {
		t.Error("file should exist in CAS")
	}

	// Check permissions
	info, _ := os.Stat(s.FilePath(hash))
	if info.Mode().Perm() != 0444 {
		t.Errorf("perm = %o, want 0444", info.Mode().Perm())
	}
}

func TestStoreFileExecutable(t *testing.T) {
	s := newTestStore(t)

	tmpFile := filepath.Join(s.TmpDir(), "script.sh")
	os.WriteFile(tmpFile, []byte("#!/bin/sh"), 0644)

	hash := HashBytes([]byte("#!/bin/sh"))

	if err := s.StoreFile(tmpFile, hash, true); err != nil {
		t.Fatalf("StoreFile: %v", err)
	}

	info, _ := os.Stat(s.FilePath(hash))
	if info.Mode().Perm() != 0555 {
		t.Errorf("perm = %o, want 0555", info.Mode().Perm())
	}
}

func TestStoreFileSkipExisting(t *testing.T) {
	s := newTestStore(t)

	hash := HashBytes([]byte("content"))

	// Pre-create the CAS entry
	casPath := s.FilePath(hash)
	os.MkdirAll(filepath.Dir(casPath), 0755)
	os.WriteFile(casPath, []byte("content"), 0444)

	// Try to store again — should skip without error
	tmpFile := filepath.Join(s.TmpDir(), "dup.txt")
	os.WriteFile(tmpFile, []byte("content"), 0644)

	if err := s.StoreFile(tmpFile, hash, false); err != nil {
		t.Fatalf("StoreFile skip existing: %v", err)
	}
}

func TestManifestReadWrite(t *testing.T) {
	s := newTestStore(t)

	m := &Manifest{
		Name:     "monolog/monolog",
		Version:  "3.9.0",
		DistHash: "sha256:abc",
		Files: []FileEntry{
			{Path: "src/Logger.php", Hash: "sha256:def", Size: 100, Executable: false},
		},
		StoredAt: time.Now().UTC(),
	}

	if err := s.WriteManifest(m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	if !s.ManifestExists("monolog/monolog", "3.9.0") {
		t.Error("manifest should exist")
	}

	m2, err := s.ReadManifest("monolog/monolog", "3.9.0")
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if m2.Name != "monolog/monolog" || len(m2.Files) != 1 {
		t.Errorf("manifest roundtrip failed: %+v", m2)
	}
}

func TestManifestNotExists(t *testing.T) {
	s := newTestStore(t)
	if s.ManifestExists("nonexistent/pkg", "1.0.0") {
		t.Error("should not exist")
	}
}
