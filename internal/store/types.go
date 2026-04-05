package store

import "time"

// FileEntry represents a single file in a package manifest.
type FileEntry struct {
	Path       string `json:"path"`
	Hash       string `json:"hash"`
	Size       int64  `json:"size"`
	Executable bool   `json:"executable"`
}

// Manifest represents a package version's manifest in the CAS.
type Manifest struct {
	Name     string      `json:"name"`
	Version  string      `json:"version"`
	DistHash string      `json:"dist_hash"`
	Files    []FileEntry `json:"files"`
	StoredAt time.Time   `json:"stored_at"`
}

// StoreMetadata represents ~/.allegro/allegro.json.
type StoreMetadata struct {
	StoreVersion int       `json:"store_version"`
	CreatedAt    time.Time `json:"created_at"`
}
