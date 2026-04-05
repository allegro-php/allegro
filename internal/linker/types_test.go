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
		AllegroVersion: "0.1.0",
		LinkStrategy:   "reflink",
		LockHash:       "sha256:abc",
		InstalledAt:    time.Date(2026, 4, 5, 20, 0, 0, 0, time.UTC),
		Packages:       map[string]string{"monolog/monolog": "3.9.0"},
	}
	data, err := json.Marshal(vs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var vs2 VendorState
	if err := json.Unmarshal(data, &vs2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if vs2.LinkStrategy != "reflink" {
		t.Errorf("link_strategy = %q, want reflink", vs2.LinkStrategy)
	}
	if vs2.Packages["monolog/monolog"] != "3.9.0" {
		t.Error("packages roundtrip failed")
	}
}
