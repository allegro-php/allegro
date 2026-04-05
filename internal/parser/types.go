package parser

// Dist represents the distribution information for a package.
type Dist struct {
	Type      string `json:"type"`
	URL       string `json:"url"`
	Reference string `json:"reference"`
	Shasum    string `json:"shasum"`
}

// Autoload represents a package's autoload configuration.
type Autoload struct {
	PSR4     map[string]interface{} `json:"psr-4,omitempty"`
	PSR0     map[string]interface{} `json:"psr-0,omitempty"`
	Classmap []string               `json:"classmap,omitempty"`
	Files    []string               `json:"files,omitempty"`
}

// Package represents a single package entry in composer.lock.
type Package struct {
	Name              string                 `json:"name"`
	Version           string                 `json:"version"`
	VersionNormalized string                 `json:"version_normalized"`
	Type              string                 `json:"type"`
	Dist              *Dist                  `json:"dist"`
	Autoload          *Autoload              `json:"autoload,omitempty"`
	Extra             map[string]interface{} `json:"extra,omitempty"`
	Description       string                 `json:"description,omitempty"`
	Bin               []string               `json:"bin,omitempty"`
	NotificationURL   string                 `json:"notification-url,omitempty"`
	Require           map[string]string      `json:"require,omitempty"`
	Replace           map[string]string      `json:"replace,omitempty"`
	Provide           map[string]string      `json:"provide,omitempty"`
	Source            map[string]interface{} `json:"source,omitempty"`
}

// ComposerLock represents the root structure of a composer.lock file.
type ComposerLock struct {
	Packages    []Package `json:"packages"`
	PackagesDev []Package `json:"packages-dev"`
	ContentHash string    `json:"content-hash"`
}
