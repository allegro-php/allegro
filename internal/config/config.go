package config

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/allegro-php/allegro/internal/store"
)

// Config represents ~/.allegro/config.json.
// Config represents ~/.allegro/config.json.
type Config struct {
	StorePath     string `json:"store_path,omitempty"`
	Workers       int    `json:"workers,omitempty"`
	LinkStrategy  string `json:"link_strategy,omitempty"`
	NoProgress    bool   `json:"no_progress"`
	NoColor       bool   `json:"no_color"`
	ComposerPath  string `json:"composer_path,omitempty"`
	NoDev         bool   `json:"no_dev"`
	NoScripts     bool   `json:"no_scripts"`
	PruneStaleDay int    `json:"prune_stale_days,omitempty"`
}

// ReadConfig reads the config file. Returns zero-value Config if missing or malformed.
func ReadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil // absent = all defaults
		}
		return Config{}, nil // other errors: warn and use defaults
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		log.Printf("warning: malformed config file %s, using defaults", path)
		return Config{}, nil // malformed = warn + defaults per spec §7.6
	}
	return c, nil
}

// WriteConfig writes config to the given path, creating parent dirs.
func WriteConfig(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return store.WriteFileAtomic(path, data, 0644)
}

// ResolveWithConfig implements the 4-tier precedence: flag > env > config > default.
// For int values: 0 means "not set" for flag and config tiers.
func ResolveWithConfig(flagVal int, envVal string, configVal int, defaultVal int) int {
	if flagVal != 0 {
		return flagVal
	}
	if envVal != "" {
		if v, err := strconv.Atoi(envVal); err == nil && v != 0 {
			return v
		}
	}
	if configVal != 0 {
		return configVal
	}
	return defaultVal
}
