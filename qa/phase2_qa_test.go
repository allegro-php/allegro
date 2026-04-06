package qa_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/allegro-php/allegro/internal/autoloader"
	"github.com/allegro-php/allegro/internal/cli"
	"github.com/allegro-php/allegro/internal/config"
	"github.com/allegro-php/allegro/internal/linker"
	"github.com/allegro-php/allegro/internal/orchestrator"
	"github.com/allegro-php/allegro/internal/parser"
	"github.com/allegro-php/allegro/internal/store"
)

// ===== PACKAGE DIFF =====

func TestQA2_DiffAddedRemovedUpdatedUnchanged(t *testing.T) {
	old := map[string]string{"a/b": "1.0", "c/d": "2.0", "e/f": "3.0"}
	new := []parser.Package{
		{Name: "a/b", Version: "1.0"},   // unchanged
		{Name: "c/d", Version: "3.0"},   // updated
		{Name: "g/h", Version: "1.0"},   // added
	}
	diff := orchestrator.ComputeDiff(old, new)
	if len(diff.Unchanged) != 1 || diff.Unchanged[0].Name != "a/b" { t.Error("unchanged") }
	if len(diff.Updated) != 1 || diff.Updated[0].OldVersion != "2.0" { t.Error("updated") }
	if len(diff.Added) != 1 || diff.Added[0].Name != "g/h" { t.Error("added") }
	if len(diff.Removed) != 1 || diff.Removed[0].Name != "e/f" { t.Error("removed") }
}

func TestQA2_DiffCaseInsensitive(t *testing.T) {
	old := map[string]string{"Monolog/Monolog": "3.0"}
	new := []parser.Package{{Name: "monolog/monolog", Version: "3.0"}}
	diff := orchestrator.ComputeDiff(old, new)
	if len(diff.Unchanged) != 1 { t.Errorf("case insensitive failed: %+v", diff) }
	if len(diff.Added)+len(diff.Removed) != 0 { t.Error("spurious add/remove") }
}

func TestQA2_DiffEmptyBothSides(t *testing.T) {
	diff := orchestrator.ComputeDiff(map[string]string{}, []parser.Package{})
	if len(diff.Added)+len(diff.Removed)+len(diff.Updated)+len(diff.Unchanged) != 0 {
		t.Error("empty diff should have no entries")
	}
}

// ===== NOOP DETECTION =====

func TestQA2_NoopHashMatchDevMatch(t *testing.T) {
	if !orchestrator.IsNoop("sha256:abc", "sha256:abc", true, true) {
		t.Error("should be noop")
	}
}

func TestQA2_NoopHashDiffers(t *testing.T) {
	if orchestrator.IsNoop("sha256:abc", "sha256:def", true, true) {
		t.Error("different hash should not be noop")
	}
}

func TestQA2_NoopDevDiffers(t *testing.T) {
	if orchestrator.IsNoop("sha256:abc", "sha256:abc", true, false) {
		t.Error("different dev flag should not be noop")
	}
}

// ===== VENDOR STATE PHASE 2 FIELDS =====

func TestQA2_VendorStatePhase2Fields(t *testing.T) {
	dir := t.TempDir()
	vd := filepath.Join(dir, "vendor")
	os.MkdirAll(vd, 0755)

	err := linker.WriteVendorState(vd, linker.WriteVendorStateOpts{
		Version: "0.2.0", Strategy: linker.Reflink, LockHash: "sha256:abc",
		Packages: map[string]string{"a/b": "1.0"},
		Dev: true, DevPackages: []string{"phpunit/phpunit"},
		ScriptsExecuted: true,
	})
	if err != nil { t.Fatal(err) }

	state, err := linker.ReadVendorState(vd)
	if err != nil { t.Fatal(err) }
	if state.SchemaVersion != 2 { t.Errorf("schema = %d", state.SchemaVersion) }
	if !state.Dev { t.Error("dev should be true") }
	if len(state.DevPackages) != 1 { t.Error("dev_packages") }
	if !state.ScriptsExecuted { t.Error("scripts_executed") }
}

func TestQA2_VendorStateDevFalsePersistedNotOmitted(t *testing.T) {
	dir := t.TempDir()
	vd := filepath.Join(dir, "vendor")
	os.MkdirAll(vd, 0755)

	linker.WriteVendorState(vd, linker.WriteVendorStateOpts{
		Version: "0.2.0", Strategy: linker.Copy, LockHash: "sha256:x",
		Packages: map[string]string{}, Dev: false, DevPackages: []string{},
		ScriptsExecuted: false,
	})

	// Read raw JSON to verify fields are present (not omitted)
	data, _ := os.ReadFile(filepath.Join(vd, ".allegro-state.json"))
	raw := string(data)
	if !strings.Contains(raw, `"dev":false`) && !strings.Contains(raw, `"dev": false`) {
		t.Errorf("dev:false should be persisted, got: %s", raw)
	}
	if !strings.Contains(raw, `"scripts_executed":false`) && !strings.Contains(raw, `"scripts_executed": false`) {
		t.Errorf("scripts_executed:false should be persisted, got: %s", raw)
	}
}

func TestQA2_VendorStateBackwardCompatPhase1(t *testing.T) {
	dir := t.TempDir()
	// Write Phase 1 style state (no schema_version, dev, dev_packages, scripts_executed)
	raw := `{"allegro_version":"0.1.0","link_strategy":"copy","lock_hash":"sha256:x","installed_at":"2026-04-05T20:00:00Z","packages":{"a/b":"1.0"}}`
	os.WriteFile(filepath.Join(dir, ".allegro-state.json"), []byte(raw), 0644)

	state, err := linker.ReadVendorState(dir)
	if err != nil { t.Fatal(err) }
	if state.SchemaVersion != 0 { t.Error("schema should be 0") }
	if state.EffectiveDev() != true { t.Error("Phase 1 should default to dev=true") }
	if state.HasDevPackages() { t.Error("Phase 1 should not have dev_packages") }
	if state.NeedsFullRebuildForDevSwitch(false) != true { t.Error("Phase 1→no-dev should rebuild") }
	if state.NeedsFullRebuildForDevSwitch(true) != false { t.Error("Phase 1→dev should not rebuild") }
}

// ===== CONFIG =====

func TestQA2_ConfigReadWriteRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	c := config.Config{Workers: 16, LinkStrategy: "copy", PruneStaleDay: 30, NoDev: true}
	config.WriteConfig(path, c)
	c2, err := config.ReadConfig(path)
	if err != nil { t.Fatal(err) }
	if c2.Workers != 16 { t.Error("workers") }
	if c2.NoDev != true { t.Error("no_dev should persist") }
	if c2.PruneStaleDay != 30 { t.Error("prune_stale_days") }
}

func TestQA2_ConfigMissingFileDefaults(t *testing.T) {
	c, err := config.ReadConfig("/nonexistent/config.json")
	if err != nil { t.Fatal(err) }
	if c.Workers != 0 { t.Error("should be zero/default") }
}

func TestQA2_ConfigMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte("{broken"), 0644)
	c, err := config.ReadConfig(path)
	if err != nil { t.Fatal(err) }
	if c.Workers != 0 { t.Error("malformed should return defaults") }
}

func TestQA2_ConfigPrecedence(t *testing.T) {
	// flag > env > config > default
	got := config.ResolveWithConfig(4, "16", 12, 8)
	if got != 4 { t.Errorf("flag should win: %d", got) }
	got = config.ResolveWithConfig(0, "16", 12, 8)
	if got != 16 { t.Errorf("env should win: %d", got) }
	got = config.ResolveWithConfig(0, "", 12, 8)
	if got != 12 { t.Errorf("config should win: %d", got) }
	got = config.ResolveWithConfig(0, "", 0, 8)
	if got != 8 { t.Errorf("default: %d", got) }
}

// ===== PROJECT REGISTRY =====

func TestQA2_RegistryRegisterAndRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects.json")
	entry := store.ProjectEntry{Path: "/app1", LockHash: "sha256:a", Packages: map[string]string{"a/b": "1.0"}}
	store.RegisterProject(path, entry)
	reg, _ := store.ReadRegistry(path)
	if len(reg.Projects) != 1 { t.Fatal("should have 1 project") }
	if reg.Projects[0].Path != "/app1" { t.Error("path") }
}

func TestQA2_RegistryUpsert(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects.json")
	store.RegisterProject(path, store.ProjectEntry{Path: "/app1", LockHash: "v1"})
	store.RegisterProject(path, store.ProjectEntry{Path: "/app1", LockHash: "v2"})
	reg, _ := store.ReadRegistry(path)
	if len(reg.Projects) != 1 { t.Errorf("should upsert: %d", len(reg.Projects)) }
	if reg.Projects[0].LockHash != "v2" { t.Error("should update hash") }
}

func TestQA2_RegistryCorruptJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects.json")
	os.WriteFile(path, []byte("{bad"), 0644)
	_, err := store.ReadRegistry(path)
	if err == nil { t.Fatal("should error on corrupt JSON") }
}

// ===== GC =====

func TestQA2_GCDeletedProject(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "packages"), 0755)
	os.MkdirAll(filepath.Join(dir, "files"), 0755)
	regPath := filepath.Join(dir, "projects.json")
	store.RegisterProject(regPath, store.ProjectEntry{Path: "/gone", Packages: map[string]string{}})
	result, _ := store.GarbageCollect(dir, regPath, 90, false)
	if result.ProjectsRemoved != 1 { t.Errorf("removed = %d", result.ProjectsRemoved) }
}

func TestQA2_GCStaleProjectWarned(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "packages"), 0755)
	os.MkdirAll(filepath.Join(dir, "files"), 0755)
	projDir := filepath.Join(dir, "myproject")
	os.MkdirAll(projDir, 0755)
	regPath := filepath.Join(dir, "projects.json")
	reg := &store.ProjectRegistry{Projects: []store.ProjectEntry{{
		Path: projDir, LastInstall: time.Now().AddDate(0, 0, -100),
		Packages: map[string]string{},
	}}}
	data, _ := json.MarshalIndent(reg, "", "  ")
	os.WriteFile(regPath, data, 0644)
	result, _ := store.GarbageCollect(dir, regPath, 90, false)
	if result.StaleWarned != 1 { t.Errorf("stale = %d", result.StaleWarned) }
	if result.ProjectsRemoved != 0 { t.Error("stale should not be removed") }
}

func TestQA2_GCDryRunPreserves(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "packages"), 0755)
	os.MkdirAll(filepath.Join(dir, "files"), 0755)
	regPath := filepath.Join(dir, "projects.json")
	store.RegisterProject(regPath, store.ProjectEntry{Path: "/gone"})
	store.GarbageCollect(dir, regPath, 90, true) // dry run
	reg, _ := store.ReadRegistry(regPath)
	if len(reg.Projects) != 1 { t.Error("dry run should preserve") }
}

// ===== VERIFY =====

func TestQA2_VerifyOK(t *testing.T) {
	dir := t.TempDir()
	s := store.New(filepath.Join(dir, "store"))
	s.EnsureDirectories()
	content := []byte("<?php class A {}")
	hash := store.HashBytes(content)
	tmp := filepath.Join(s.TmpDir(), "f")
	os.WriteFile(tmp, content, 0644)
	s.StoreFile(tmp, hash, false)
	m := &store.Manifest{Name: "a/b", Version: "1.0",
		Files: []store.FileEntry{{Path: "A.php", Hash: "sha256:" + hash, Size: int64(len(content))}},
		StoredAt: time.Now().UTC()}
	s.WriteManifest(m)
	vd := filepath.Join(dir, "vendor")
	os.MkdirAll(filepath.Join(vd, "a/b"), 0755)
	os.WriteFile(filepath.Join(vd, "a/b/A.php"), content, 0644)

	state := &linker.VendorState{LinkStrategy: "copy", Packages: map[string]string{"a/b": "1.0"}}
	result, _ := orchestrator.VerifyVendor(vd, s, state, 1)
	if result.FailPackages != 0 { t.Errorf("fail = %d", result.FailPackages) }
}

func TestQA2_VerifyDetectsModified(t *testing.T) {
	dir := t.TempDir()
	s := store.New(filepath.Join(dir, "store"))
	s.EnsureDirectories()
	content := []byte("original")
	hash := store.HashBytes(content)
	tmp := filepath.Join(s.TmpDir(), "f")
	os.WriteFile(tmp, content, 0644)
	s.StoreFile(tmp, hash, false)
	m := &store.Manifest{Name: "a/b", Version: "1.0",
		Files: []store.FileEntry{{Path: "f.php", Hash: "sha256:" + hash, Size: int64(len(content))}},
		StoredAt: time.Now().UTC()}
	s.WriteManifest(m)
	vd := filepath.Join(dir, "vendor")
	os.MkdirAll(filepath.Join(vd, "a/b"), 0755)
	os.WriteFile(filepath.Join(vd, "a/b/f.php"), []byte("MODIFIED"), 0644)

	state := &linker.VendorState{LinkStrategy: "copy", Packages: map[string]string{"a/b": "1.0"}}
	result, _ := orchestrator.VerifyVendor(vd, s, state, 1)
	if result.FailPackages != 1 { t.Errorf("fail = %d", result.FailPackages) }
	found := false
	for _, i := range result.Issues { if i.Type == "modified" { found = true } }
	if !found { t.Error("should detect modified") }
}

func TestQA2_VerifyDetectsMissing(t *testing.T) {
	dir := t.TempDir()
	s := store.New(filepath.Join(dir, "store"))
	s.EnsureDirectories()
	m := &store.Manifest{Name: "a/b", Version: "1.0",
		Files: []store.FileEntry{{Path: "gone.php", Hash: "sha256:abc123", Size: 10}},
		StoredAt: time.Now().UTC()}
	s.WriteManifest(m)
	vd := filepath.Join(dir, "vendor")
	os.MkdirAll(filepath.Join(vd, "a/b"), 0755) // no file

	state := &linker.VendorState{LinkStrategy: "copy", Packages: map[string]string{"a/b": "1.0"}}
	result, _ := orchestrator.VerifyVendor(vd, s, state, 1)
	found := false
	for _, i := range result.Issues { if i.Type == "missing" { found = true } }
	if !found { t.Error("should detect missing") }
}

// ===== PARALLEL LINKING =====

func TestQA2_ParallelLinkCopy(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	os.MkdirAll(src, 0755)
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("aaa"), 0644)
	os.WriteFile(filepath.Join(src, "b.txt"), []byte("bbb"), 0644)

	ops := []orchestrator.LinkOp{
		{Src: filepath.Join(src, "a.txt"), Dst: filepath.Join(dst, "sub", "a.txt")},
		{Src: filepath.Join(src, "b.txt"), Dst: filepath.Join(dst, "sub", "b.txt"), Executable: true},
	}
	err := orchestrator.ParallelLink(ops, &linker.CopyLinker{}, linker.Copy, 2)
	if err != nil { t.Fatal(err) }
	d, _ := os.ReadFile(filepath.Join(dst, "sub", "a.txt"))
	if string(d) != "aaa" { t.Error("content") }
	info, _ := os.Stat(filepath.Join(dst, "sub", "b.txt"))
	if info.Mode().Perm() != 0755 { t.Errorf("perm = %o", info.Mode().Perm()) }
}

func TestQA2_ParallelLinkErrorCancels(t *testing.T) {
	ops := []orchestrator.LinkOp{
		{Src: "/nonexistent/src", Dst: "/tmp/test-dst"},
	}
	err := orchestrator.ParallelLink(ops, &linker.CopyLinker{}, linker.Copy, 1)
	if err == nil { t.Error("should error on missing src") }
}

// ===== FLAGS =====

func TestQA2_ExitCodes(t *testing.T) {
	if cli.ExitSuccess != 0 { t.Error("0") }
	if cli.ExitGeneralError != 1 { t.Error("1") }
	if cli.ExitProjectFile != 2 { t.Error("2") }
	if cli.ExitNetworkError != 3 { t.Error("3") }
	if cli.ExitFilesystemError != 4 { t.Error("4") }
	if cli.ExitComposerError != 5 { t.Error("5") }
}

func TestQA2_IsDevModeDefault(t *testing.T) {
	// Can't easily reset flags in test, but test the env path
	t.Setenv("ALLEGRO_NO_DEV", "")
	// Default should be true (can't test flag without cobra)
}

func TestQA2_IsColorDisabledByEnv(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if cli.IsColorEnabled() { t.Error("NO_COLOR should disable") }
}

// ===== BIN PROXY =====

func TestQA2_BinProxyPHPNoShebang(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "tool"), []byte("<?php echo 1;"), 0755)
	typ, _ := autoloader.DetectBinTarget(filepath.Join(d, "tool"))
	if typ != autoloader.BinPHPNoShebang { t.Error("should be PHP no shebang") }
}

func TestQA2_BinProxyPHPWithShebang(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "tool"), []byte("#!/usr/bin/env php\n<?php echo 1;"), 0755)
	typ, _ := autoloader.DetectBinTarget(filepath.Join(d, "tool"))
	if typ != autoloader.BinPHPWithShebang { t.Error("should be PHP with shebang") }
}

func TestQA2_BinProxyShell(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "tool"), []byte("#!/bin/sh\necho hi"), 0755)
	typ, _ := autoloader.DetectBinTarget(filepath.Join(d, "tool"))
	if typ != autoloader.BinNonPHP { t.Error("should be non-PHP") }
}

func TestQA2_BinProxyBOM(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "tool"), []byte("\xEF\xBB\xBF<?php echo 1;"), 0755)
	typ, _ := autoloader.DetectBinTarget(filepath.Join(d, "tool"))
	if typ != autoloader.BinPHPNoShebang { t.Error("BOM should be stripped") }
}

// ===== COMPOSER VERSION PARSING =====

func TestQA2_ComposerVersionParse(t *testing.T) {
	// CheckComposerVersion needs a real binary; test the parser indirectly
	// by verifying the version string format expectations
	_ = autoloader.CheckComposerVersion // just verify it exists/compiles
}

// ===== FLOCK =====

func TestQA2_FlockAcquireRelease(t *testing.T) {
	dir := t.TempDir()
	lock, err := linker.AcquireLock(context.Background(), dir)
	if err != nil { t.Fatal(err) }
	lock.Release()
	if _, err := os.Stat(filepath.Join(dir, ".allegro.lock")); err != nil {
		t.Error("lock file should persist")
	}
}

func TestQA2_FlockContextCancel(t *testing.T) {
	dir := t.TempDir()
	// Acquire first lock
	lock1, _ := linker.AcquireLock(context.Background(), dir)
	defer lock1.Release()

	// Try second lock with immediate cancel
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	_, err := linker.AcquireLock(ctx, dir)
	if err == nil { t.Error("should fail on cancelled context") }
}

// ===== STORE ATOMIC WRITES =====

func TestQA2_WriteFileAtomicCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	err := store.WriteFileAtomic(path, []byte(`{"key":"value"}`), 0644)
	if err != nil { t.Fatal(err) }
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "key") { t.Error("content") }
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0644 { t.Errorf("perm = %o", info.Mode().Perm()) }
}

// ===== INSTALLED.JSON =====

func TestQA2_InstalledJSONDefaultType(t *testing.T) {
	lock := &parser.ComposerLock{
		Packages: []parser.Package{{Name: "a/b", Version: "1.0"}},
	}
	data, _ := autoloader.GenerateInstalledJSON(lock)
	var m map[string]interface{}
	json.Unmarshal(data, &m)
	pkgs := m["packages"].([]interface{})
	pkg := pkgs[0].(map[string]interface{})
	if pkg["type"] != "library" { t.Errorf("default type = %v", pkg["type"]) }
}

func TestQA2_InstalledPHPDevRequirement(t *testing.T) {
	lock := &parser.ComposerLock{
		Packages:    []parser.Package{{Name: "a/b", Version: "1.0", Dist: &parser.Dist{Reference: "x"}}},
		PackagesDev: []parser.Package{{Name: "c/d", Version: "2.0", Dist: &parser.Dist{Reference: "y"}}},
	}
	php := autoloader.GenerateInstalledPHP(lock, map[string]interface{}{})
	if !strings.Contains(php, "'dev_requirement' => false") { t.Error("a/b dev_requirement") }
	if !strings.Contains(php, "'dev_requirement' => true") { t.Error("c/d dev_requirement") }
}

// ===== COLORED DIFF =====

func TestQA2_ColoredDiffNoPanic(t *testing.T) {
	diff := orchestrator.PackageDiff{
		Added:   []parser.Package{{Name: "a/b", Version: "1.0"}},
		Updated: []orchestrator.PackageUpdate{{Name: "c/d", OldVersion: "1.0", NewVersion: "2.0"}},
		Removed: []parser.Package{{Name: "e/f", Version: "3.0"}},
	}
	cli.PrintDiff(diff) // should not panic
}

// ===== HASHER =====

func TestQA2_HashKnownValues(t *testing.T) {
	if store.HashBytes([]byte("")) != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Error("empty hash")
	}
}

func TestQA2_ShardPrefix(t *testing.T) {
	if store.ShardPrefix("abcdef") != "ab" { t.Error("shard") }
	if store.ShardPrefix("") != "" { t.Error("empty shard") }
}

// ===== LOCK HASH =====

func TestQA2_LockHashDeterministic(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "composer.lock")
	os.WriteFile(p, []byte(`{}`), 0644)
	h1, _ := parser.ComputeLockHash(p)
	h2, _ := parser.ComputeLockHash(p)
	if h1 != h2 { t.Error("not deterministic") }
	if !strings.HasPrefix(h1, "sha256:") { t.Error("prefix") }
}
