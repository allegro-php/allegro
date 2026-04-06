//go:build !windows

package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// ProjectEntry represents a single project in the registry.
type ProjectEntry struct {
	Path        string            `json:"path"`
	LastInstall time.Time         `json:"last_install"`
	LockHash    string            `json:"lock_hash"`
	Packages    map[string]string `json:"packages"`
}

// ProjectRegistry represents ~/.allegro/projects.json.
type ProjectRegistry struct {
	Projects []ProjectEntry `json:"projects"`
}

// DefaultRegistryPath returns ~/.allegro/projects.json.
func DefaultRegistryPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".allegro", "projects.json")
	}
	return filepath.Join(home, ".allegro", "projects.json")
}

// ReadRegistry reads ~/.allegro/projects.json. Returns empty registry if missing.
func ReadRegistry(path string) (*ProjectRegistry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ProjectRegistry{}, nil
		}
		return &ProjectRegistry{}, nil
	}
	var reg ProjectRegistry
	if err := json.Unmarshal(data, &reg); err != nil {
		return &ProjectRegistry{}, nil
	}
	return &reg, nil
}

// RegisterProject adds or updates a project entry in the registry.
func RegisterProject(path string, entry ProjectEntry) error {
	// Acquire flock on projects.lock (§9.1)
	lockPath := filepath.Join(filepath.Dir(path), "projects.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("create projects lock: %w", err)
	}
	defer lockFile.Close()
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("acquire projects lock: %w", err)
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)

	reg, err := ReadRegistry(path)
	if err != nil {
		return err
	}

	entry.LastInstall = time.Now().UTC()

	// Update existing or append
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

	// Write atomically with fsync
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, data, 0644)
}
