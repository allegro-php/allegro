package store

import (
	"encoding/json"
	"testing"
	"time"
)

func TestManifestJSONRoundTrip(t *testing.T) {
	m := Manifest{
		Name:     "monolog/monolog",
		Version:  "3.9.0",
		DistHash: "sha256:abc123",
		Files: []FileEntry{
			{Path: "src/Logger.php", Hash: "sha256:def456", Size: 12345, Executable: false},
			{Path: "bin/console", Hash: "sha256:ghi789", Size: 890, Executable: true},
		},
		StoredAt: time.Date(2026, 4, 5, 20, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m2 Manifest
	if err := json.Unmarshal(data, &m2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m2.Name != m.Name || len(m2.Files) != 2 {
		t.Errorf("roundtrip failed: got %+v", m2)
	}
	if m2.Files[1].Executable != true {
		t.Error("executable flag lost in roundtrip")
	}
}

func TestStoreMetadataJSON(t *testing.T) {
	sm := StoreMetadata{
		StoreVersion: 1,
		CreatedAt:    time.Date(2026, 4, 5, 20, 0, 0, 0, time.UTC),
	}
	data, err := json.Marshal(sm)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var sm2 StoreMetadata
	if err := json.Unmarshal(data, &sm2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if sm2.StoreVersion != 1 {
		t.Errorf("store_version = %d, want 1", sm2.StoreVersion)
	}
}
