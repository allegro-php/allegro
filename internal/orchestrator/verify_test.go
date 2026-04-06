package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/allegro-php/allegro/internal/linker"
	"github.com/allegro-php/allegro/internal/store"
)

func setupVerifyTest(t *testing.T) (string, *store.Store, *linker.VendorState) {
	t.Helper()
	dir := t.TempDir()
	s := store.New(filepath.Join(dir, "store"))
	s.EnsureDirectories()

	// Create a CAS file
	content := []byte("<?php class Logger {}")
	hash := store.HashBytes(content)
	tmpFile := filepath.Join(s.TmpDir(), "test")
	os.WriteFile(tmpFile, content, 0644)
	s.StoreFile(tmpFile, hash, false)

	// Write manifest
	m := &store.Manifest{
		Name: "monolog/monolog", Version: "3.9.0",
		Files:    []store.FileEntry{{Path: "src/Logger.php", Hash: "sha256:" + hash, Size: int64(len(content)), Executable: false}},
		StoredAt: time.Now().UTC(),
	}
	s.WriteManifest(m)

	// Create vendor with correct file
	vendorDir := filepath.Join(dir, "vendor")
	os.MkdirAll(filepath.Join(vendorDir, "monolog/monolog/src"), 0755)
	os.WriteFile(filepath.Join(vendorDir, "monolog/monolog/src/Logger.php"), content, 0644)

	state := &linker.VendorState{
		LinkStrategy: "copy",
		Packages:     map[string]string{"monolog/monolog": "3.9.0"},
	}

	return vendorDir, s, state
}

func TestVerifyAllOK(t *testing.T) {
	vendorDir, s, state := setupVerifyTest(t)
	result, err := VerifyVendor(vendorDir, s, state, 1)
	if err != nil { t.Fatal(err) }
	if result.OKPackages != 1 { t.Errorf("ok = %d, want 1", result.OKPackages) }
	if result.FailPackages != 0 { t.Errorf("fail = %d, want 0", result.FailPackages) }
	if len(result.Issues) != 0 { t.Errorf("issues = %v", result.Issues) }
}

func TestVerifyMissingFile(t *testing.T) {
	vendorDir, s, state := setupVerifyTest(t)
	os.Remove(filepath.Join(vendorDir, "monolog/monolog/src/Logger.php"))

	result, _ := VerifyVendor(vendorDir, s, state, 1)
	if result.FailPackages != 1 { t.Errorf("fail = %d, want 1", result.FailPackages) }
	found := false
	for _, issue := range result.Issues {
		if issue.Type == "missing" { found = true }
	}
	if !found { t.Error("should detect missing file") }
}

func TestVerifyModifiedFile(t *testing.T) {
	vendorDir, s, state := setupVerifyTest(t)
	// Modify the vendor file
	os.WriteFile(filepath.Join(vendorDir, "monolog/monolog/src/Logger.php"), []byte("MODIFIED"), 0644)

	result, _ := VerifyVendor(vendorDir, s, state, 1)
	if result.FailPackages != 1 { t.Errorf("fail = %d", result.FailPackages) }
	found := false
	for _, issue := range result.Issues {
		if issue.Type == "modified" { found = true }
	}
	if !found { t.Error("should detect modified file") }
}

func TestVerifyPermissionMismatch(t *testing.T) {
	vendorDir, s, state := setupVerifyTest(t)
	// Set wrong permission
	os.Chmod(filepath.Join(vendorDir, "monolog/monolog/src/Logger.php"), 0755) // should be 0644

	result, _ := VerifyVendor(vendorDir, s, state, 1)
	found := false
	for _, issue := range result.Issues {
		if issue.Type == "permission" { found = true }
	}
	if !found { t.Error("should detect permission mismatch") }
}

func TestVerifyMissingManifest(t *testing.T) {
	dir := t.TempDir()
	s := store.New(filepath.Join(dir, "store"))
	s.EnsureDirectories()
	vendorDir := filepath.Join(dir, "vendor")
	os.MkdirAll(vendorDir, 0755)

	state := &linker.VendorState{
		LinkStrategy: "copy",
		Packages:     map[string]string{"nonexistent/pkg": "1.0.0"},
	}

	result, _ := VerifyVendor(vendorDir, s, state, 1)
	if result.FailPackages != 1 { t.Errorf("fail = %d", result.FailPackages) }
}
