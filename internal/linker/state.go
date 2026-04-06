package linker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// WriteVendorState writes vendor/.allegro-state.json.
func WriteVendorState(vendorDir string, version string, strategy Strategy, lockHash string, packages map[string]string) error {
	state := VendorState{
		AllegroVersion: version,
		LinkStrategy:   strategy.String(),
		LockHash:       lockHash,
		InstalledAt:    time.Now().UTC(),
		Packages:       packages,
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	path := filepath.Join(vendorDir, ".allegro-state.json")
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write state tmp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename state: %w", err)
	}
	return nil
}

// ReadVendorState reads vendor/.allegro-state.json.
func ReadVendorState(vendorDir string) (*VendorState, error) {
	path := filepath.Join(vendorDir, ".allegro-state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var state VendorState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	if state.LockHash == "" {
		return nil, fmt.Errorf("state file missing lock_hash field")
	}
	return &state, nil
	return &state, nil
}

// EffectiveDev returns the dev mode from state, treating absent as true (Phase 1 default).
func (s *VendorState) EffectiveDev() bool {
	if s.SchemaVersion == 0 {
		return true // Phase 1 state: always installed dev
	}
	return s.Dev
}

// HasDevPackages returns true if dev_packages info is available for incremental diff.
func (s *VendorState) HasDevPackages() bool {
	return s.SchemaVersion >= 2 && s.DevPackages != nil
}

// NeedsFullRebuildForDevSwitch returns true if switching dev mode requires
// a full rebuild because dev_packages info is missing (Phase 1 state).
func (s *VendorState) NeedsFullRebuildForDevSwitch(currentDev bool) bool {
	if !s.HasDevPackages() && s.EffectiveDev() != currentDev {
		return true
	}
	return false
}
