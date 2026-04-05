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
	err := WriteVendorState(vendorDir, "0.1.0", Reflink, "sha256:abc", pkgs)
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
