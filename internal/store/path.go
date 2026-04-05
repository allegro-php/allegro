package store

import (
	"os"
	"path/filepath"
)

const defaultStoreDir = ".allegro/store"

// ResolvePath returns the store directory path based on precedence:
// flagValue > envValue > default (~/.allegro/store).
func ResolvePath(flagValue, envValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if envValue != "" {
		return envValue
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", defaultStoreDir)
	}
	return filepath.Join(home, defaultStoreDir)
}
