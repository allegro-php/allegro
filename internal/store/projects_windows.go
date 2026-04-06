//go:build windows

package store

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// DefaultRegistryPath returns ~/.allegro/projects.json.
func DefaultRegistryPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".allegro", "projects.json")
	}
	return filepath.Join(home, ".allegro", "projects.json")
}

// ReadRegistry reads projects.json. Returns empty registry if missing.
func ReadRegistry(path string) (*ProjectRegistry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ProjectRegistry{}, nil
		}
		return nil, fmt.Errorf("read registry: %w", err)
	}
	var reg ProjectRegistry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("corrupt projects.json: %w", err)
	}
	}
	return &reg, nil
}

// RegisterProject on Windows — no flock (not available).
func RegisterProject(path string, entry ProjectEntry) error {
	log.Printf("warning: file locking not available on Windows; concurrent project registration unprotected")
	reg, err := ReadRegistry(path)
	if err != nil {
		return err
	}
	entry.LastInstall = time.Now().UTC()
	found := false
	for i, p := range reg.Projects {
		if p.Path == entry.Path {
			reg.Projects[i] = entry
			found = true
			break
		}
	}
	if !found {
		reg.Projects = append(reg.Projects, entry)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, data, 0644)
}
