package linker

import (
	"encoding/json"
	"testing"
	"time"
)

func TestStrategyString(t *testing.T) {
	tests := []struct {
		s    Strategy
		want string
	}{
		{Reflink, "reflink"},
		{Hardlink, "hardlink"},
		{Copy, "copy"},
		{Strategy(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.s.String(); got != tt.want {
			t.Errorf("Strategy(%d).String() = %q, want %q", tt.s, got, tt.want)
		}
	}
}

func TestVendorStateJSON(t *testing.T) {
	vs := VendorState{
		AllegroVersion:  "0.2.0",
		SchemaVersion:   2,
		LinkStrategy:    "reflink",
		LockHash:        "sha256:abc",
		InstalledAt:     time.Date(2026, 4, 5, 20, 0, 0, 0, time.UTC),
		Dev:             true,
		DevPackages:     []string{"phpunit/phpunit", "mockery/mockery"},
		ScriptsExecuted: true,
		Packages:        map[string]string{"monolog/monolog": "3.9.0"},
	}
	data, err := json.Marshal(vs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var vs2 VendorState
	if err := json.Unmarshal(data, &vs2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if vs2.SchemaVersion != 2 {
		t.Errorf("schema_version = %d, want 2", vs2.SchemaVersion)
	}
	if vs2.Dev != true {
		t.Error("dev should be true")
	}
	if len(vs2.DevPackages) != 2 || vs2.DevPackages[0] != "phpunit/phpunit" {
		t.Errorf("dev_packages = %v", vs2.DevPackages)
	}
	if vs2.ScriptsExecuted != true {
		t.Error("scripts_executed should be true")
	}
	if vs2.Packages["monolog/monolog"] != "3.9.0" {
		t.Error("packages roundtrip failed")
	}
}

func TestVendorStateBackwardCompat(t *testing.T) {
	// Phase 1 state file — missing new fields
	raw := `{"allegro_version":"0.1.0","link_strategy":"copy","lock_hash":"sha256:x","installed_at":"2026-04-05T20:00:00Z","packages":{"a/b":"1.0"}}`
	var vs VendorState
	if err := json.Unmarshal([]byte(raw), &vs); err != nil {
		t.Fatalf("unmarshal Phase 1 state: %v", err)
	}
	// Missing fields should have zero values
	if vs.SchemaVersion != 0 {
		t.Errorf("schema_version should be 0 (absent), got %d", vs.SchemaVersion)
	}
	if vs.Dev != false {
		t.Error("dev should default to false (zero value)")
	}
	if vs.DevPackages != nil && len(vs.DevPackages) != 0 {
		t.Errorf("dev_packages should be nil/empty, got %v", vs.DevPackages)
	}
	if vs.ScriptsExecuted != false {
		t.Error("scripts_executed should default to false")
	}
}
