package config

import (
	"os"
	"path/filepath"
)

// DefaultConfigPath returns ~/.allegro/config.json.
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".allegro", "config.json")
	}
	return filepath.Join(home, ".allegro", "config.json")
}
