package linker

import "time"

// Strategy represents the link method used.
type Strategy int

const (
	Reflink  Strategy = iota
	Hardlink
	Copy
)

func (s Strategy) String() string {
	switch s {
	case Reflink:
		return "reflink"
	case Hardlink:
		return "hardlink"
	case Copy:
		return "copy"
	default:
		return "unknown"
	}
}

// Linker is the interface for linking files from CAS to vendor.
type Linker interface {
	LinkFile(src, dst string) error
}

// VendorState represents vendor/.allegro-state.json.
type VendorState struct {
	AllegroVersion  string            `json:"allegro_version"`
	SchemaVersion   int               `json:"schema_version,omitempty"`
	LinkStrategy    string            `json:"link_strategy"`
	LockHash        string            `json:"lock_hash"`
	InstalledAt     time.Time         `json:"installed_at"`
	Dev             bool              `json:"dev"`
	DevPackages     []string          `json:"dev_packages"`
	ScriptsExecuted bool              `json:"scripts_executed"`
	Packages        map[string]string `json:"packages"`
}
