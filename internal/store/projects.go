package store

import (
	"os"
	"path/filepath"
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
