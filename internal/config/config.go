package config

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

// Config represents ~/.allegro/config.json.
type Config struct {
	StorePath     string `json:"store_path,omitempty"`
	Workers       int    `json:"workers,omitempty"`
	LinkStrategy  string `json:"link_strategy,omitempty"`
	NoProgress    bool   `json:"no_progress,omitempty"`
	NoColor       bool   `json:"no_color,omitempty"`
	ComposerPath  string `json:"composer_path,omitempty"`
	NoDev         bool   `json:"no_dev,omitempty"`
	NoScripts     bool   `json:"no_scripts,omitempty"`
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
	return os.WriteFile(path, data, 0644)
}
