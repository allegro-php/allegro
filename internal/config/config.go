package config

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
