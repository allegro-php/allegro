package linker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAndReadVendorState(t *testing.T) {
	dir := t.TempDir()
	vendorDir := filepath.Join(dir, "vendor")
	os.MkdirAll(vendorDir, 0755)

	pkgs := map[string]string{"monolog/monolog": "3.9.0"}
	err := WriteVendorState(vendorDir, WriteVendorStateOpts{
		Version: "0.1.0", Strategy: Reflink, LockHash: "sha256:abc", Packages: pkgs,
		Dev: true, DevPackages: []string{}, ScriptsExecuted: false,
	})
	if err != nil {
		t.Fatalf("WriteVendorState: %v", err)
	}

	state, err := ReadVendorState(vendorDir)
	if err != nil {
		t.Fatalf("ReadVendorState: %v", err)
	}
	if state.AllegroVersion != "0.1.0" {
		t.Errorf("version = %q, want 0.1.0", state.AllegroVersion)
	}
	if state.LinkStrategy != "reflink" {
		t.Errorf("strategy = %q, want reflink", state.LinkStrategy)
	}
	if state.LockHash != "sha256:abc" {
		t.Errorf("lock_hash = %q", state.LockHash)
	}
	if state.Packages["monolog/monolog"] != "3.9.0" {
		t.Error("packages mismatch")
	}
}

func TestReadVendorStateMissing(t *testing.T) {
	_, err := ReadVendorState(t.TempDir())
	if err == nil {
		t.Error("expected error for missing state")
	}
}

func TestReadVendorStateCorrupt(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".allegro-state.json"), []byte("{invalid"), 0644)
	_, err := ReadVendorState(dir)
	if err == nil {
		t.Error("expected error for corrupt state")
	}
}

func TestReadVendorStateMissingLockHash(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".allegro-state.json"), []byte(`{"allegro_version":"0.1.0"}`), 0644)
	_, err := ReadVendorState(dir)
	if err == nil {
		t.Error("expected error for missing lock_hash")
	}
}

func TestEffectiveDevPhase1(t *testing.T) {
	s := &VendorState{SchemaVersion: 0} // Phase 1 - no schema version
	if !s.EffectiveDev() {
		t.Error("Phase 1 state should default to dev=true")
	}
}

func TestEffectiveDevPhase2(t *testing.T) {
	s := &VendorState{SchemaVersion: 2, Dev: false}
	if s.EffectiveDev() {
		t.Error("Phase 2 with dev=false should return false")
	}
}

func TestHasDevPackages(t *testing.T) {
	s1 := &VendorState{SchemaVersion: 0}
	if s1.HasDevPackages() {
		t.Error("Phase 1 should not have dev_packages")
	}
	s2 := &VendorState{SchemaVersion: 2, DevPackages: []string{"phpunit/phpunit"}}
	if !s2.HasDevPackages() {
		t.Error("Phase 2 with dev_packages should return true")
	}
}

func TestNeedsFullRebuildForDevSwitch(t *testing.T) {
	// Phase 1 state switching dev mode → needs full rebuild
	s1 := &VendorState{SchemaVersion: 0}
	if !s1.NeedsFullRebuildForDevSwitch(false) {
		t.Error("Phase 1 switching to no-dev needs rebuild")
	}
	// Phase 2 state with dev_packages → no rebuild needed
	s2 := &VendorState{SchemaVersion: 2, Dev: true, DevPackages: []string{"phpunit/phpunit"}}
	if s2.NeedsFullRebuildForDevSwitch(false) {
		t.Error("Phase 2 with dev_packages should not need rebuild")
	}
	// Phase 2 same dev mode → no rebuild
	s3 := &VendorState{SchemaVersion: 2, Dev: true, DevPackages: []string{"x"}}
	if s3.NeedsFullRebuildForDevSwitch(true) {
		t.Error("same dev mode should not need rebuild")
	}
}
