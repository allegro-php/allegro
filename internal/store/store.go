package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const currentStoreVersion = 1

// Store manages the content-addressable store.
type Store struct {
	Root string // e.g. ~/.allegro/store
}

// New creates a Store at the given root path.
func New(root string) *Store {
	return &Store{Root: root}
}

// EnsureDirectories creates the store directory tree if missing.
func (s *Store) EnsureDirectories() error {
	dirs := []string{
		filepath.Join(s.Root, "files"),
		filepath.Join(s.Root, "packages"),
		filepath.Join(s.Root, "tmp"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("create store dir %s: %w", d, err)
		}
	}
	return nil
}

// MetadataPath returns the path to allegro.json.
func (s *Store) MetadataPath() string {
	return filepath.Join(filepath.Dir(s.Root), "allegro.json")
}

// EnsureMetadata creates allegro.json if missing, validates store version.
func (s *Store) EnsureMetadata() error {
	path := s.MetadataPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		meta := StoreMetadata{
			StoreVersion: currentStoreVersion,
			CreatedAt:    time.Now().UTC(),
		}
		return writeJSONAtomic(path, meta)
	}
	if err != nil {
		return fmt.Errorf("read store metadata: %w", err)
	}

	var meta StoreMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return fmt.Errorf("parse store metadata: %w", err)
	}
	if meta.StoreVersion > currentStoreVersion {
		return fmt.Errorf("store version %d is newer than this binary supports (max %d); upgrade allegro", meta.StoreVersion, currentStoreVersion)
	}
	return nil
}

// FilePath returns the CAS path for a content hash.
func (s *Store) FilePath(hash string) string {
	return filepath.Join(s.Root, "files", ShardPrefix(hash), hash)
}

// FileExists checks if a CAS file exists.
func (s *Store) FileExists(hash string) bool {
	_, err := os.Stat(s.FilePath(hash))
	return err == nil
}

// StoreFile moves a file to its CAS location with correct permissions.
func (s *Store) StoreFile(srcPath, hash string, executable bool) error {
	dst := s.FilePath(hash)

	// Skip if already exists
	if s.FileExists(hash) {
		return nil
	}

	// Ensure shard directory
	shardDir := filepath.Dir(dst)
	if err := os.MkdirAll(shardDir, 0755); err != nil {
		return fmt.Errorf("create shard dir: %w", err)
	}

	// Atomic rename
	if err := os.Rename(srcPath, dst); err != nil {
		return fmt.Errorf("store file rename: %w", err)
	}

	// Set CAS permissions
	perm := os.FileMode(0444)
	if executable {
		perm = 0555
	}
	if err := os.Chmod(dst, perm); err != nil {
		return fmt.Errorf("chmod CAS file: %w", err)
	}

	return nil
}

// ManifestPath returns the path for a package manifest.
func (s *Store) ManifestPath(name, version string) string {
	return filepath.Join(s.Root, "packages", name, version+".json")
}

// ManifestExists checks if a manifest exists for a package version.
func (s *Store) ManifestExists(name, version string) bool {
	_, err := os.Stat(s.ManifestPath(name, version))
	return err == nil
}

// ReadManifest reads a package manifest from disk.
func (s *Store) ReadManifest(name, version string) (*Manifest, error) {
	data, err := os.ReadFile(s.ManifestPath(name, version))
	if err != nil {
		return nil, fmt.Errorf("read manifest %s@%s: %w", name, version, err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest %s@%s: %w", name, version, err)
	}
	return &m, nil
}

// WriteManifest writes a package manifest atomically.
func (s *Store) WriteManifest(m *Manifest) error {
	path := s.ManifestPath(m.Name, m.Version)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create manifest dir: %w", err)
	}
	return writeJSONAtomic(path, m)
}

// TmpDir returns the store tmp directory path.
func (s *Store) TmpDir() string {
	return filepath.Join(s.Root, "tmp")
}

func writeJSONAtomic(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, filepath.Base(path)+"*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}
	tmpFile.Close()
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("atomic rename: %w", err)
	}
	return nil
}
