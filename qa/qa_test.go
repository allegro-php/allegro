package qa_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/allegro-php/allegro/internal/autoloader"
	"github.com/allegro-php/allegro/internal/cli"
	"github.com/allegro-php/allegro/internal/fetcher"
	"github.com/allegro-php/allegro/internal/linker"
	"github.com/allegro-php/allegro/internal/parser"
	"github.com/allegro-php/allegro/internal/platform"
	"github.com/allegro-php/allegro/internal/store"
)

// ====== PARSER ======

func TestQA_PlatformPackagesExcluded(t *testing.T) {
	dir := t.TempDir()
	lockData := `{"packages":[
		{"name":"monolog/monolog","version":"3.9.0","dist":{"type":"zip","url":"http://x","reference":"a","shasum":""}},
		{"name":"php","version":"8.3.0"},
		{"name":"ext-json","version":"*"},
		{"name":"lib-libxml","version":"*"},
		{"name":"laravel/framework","version":"11.0.0","dist":{"type":"zip","url":"http://x","reference":"b","shasum":""}}
	],"packages-dev":[],"content-hash":"test"}`
	os.WriteFile(filepath.Join(dir, "composer.lock"), []byte(lockData), 0644)
	lock, err := parser.ParseLockFile(filepath.Join(dir, "composer.lock"))
	if err != nil { t.Fatal(err) }
	all := parser.MergePackages(lock)
	if len(all) != 2 { t.Errorf("expected 2, got %d", len(all)) }
	for _, p := range all {
		if parser.IsPlatformPackage(p.Name) { t.Errorf("platform pkg %s leaked", p.Name) }
	}
}

func TestQA_InvalidJSONLineCol(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "composer.lock"), []byte("{\n  \"bad\n}"), 0644)
	_, err := parser.ParseLockFile(filepath.Join(dir, "composer.lock"))
	if err == nil { t.Fatal("expected error") }
	if !strings.Contains(err.Error(), "line") { t.Errorf("missing line in error: %v", err) }
}

func TestQA_MissingLockFile(t *testing.T) {
	_, err := parser.ParseLockFile("/nonexistent/composer.lock")
	if err == nil { t.Fatal("expected error") }
	if !strings.Contains(err.Error(), "not found") { t.Errorf("error: %v", err) }
}

func TestQA_DevPackageNames(t *testing.T) {
	lock := &parser.ComposerLock{
		PackagesDev: []parser.Package{{Name: "phpunit/phpunit"}, {Name: "ext-xdebug"}, {Name: "mockery/mockery"}},
	}
	names := parser.DevPackageNames(lock)
	if len(names) != 2 { t.Errorf("expected 2, got %d: %v", len(names), names) }
}

func TestQA_IsDevPackage(t *testing.T) {
	lock := &parser.ComposerLock{PackagesDev: []parser.Package{{Name: "phpunit/phpunit"}}}
	if !parser.IsDevPackage("phpunit/phpunit", lock) { t.Error("phpunit should be dev") }
	if parser.IsDevPackage("monolog/monolog", lock) { t.Error("monolog not dev") }
}

// ====== STORE ======

func TestQA_StoreDeduplication(t *testing.T) {
	dir := t.TempDir()
	s := store.New(filepath.Join(dir, "store"))
	s.EnsureDirectories()
	content := []byte("dup-content")
	hash := store.HashBytes(content)
	f1 := filepath.Join(s.TmpDir(), "f1")
	os.WriteFile(f1, content, 0644)
	s.StoreFile(f1, hash, false)
	f2 := filepath.Join(s.TmpDir(), "f2")
	os.WriteFile(f2, content, 0644)
	s.StoreFile(f2, hash, false) // should skip
	if !s.FileExists(hash) { t.Error("should exist") }
}

func TestQA_StorePermissions(t *testing.T) {
	dir := t.TempDir()
	s := store.New(filepath.Join(dir, "store"))
	s.EnsureDirectories()
	// Non-exec
	h1 := store.HashBytes([]byte("reg"))
	f1 := filepath.Join(s.TmpDir(), "r")
	os.WriteFile(f1, []byte("reg"), 0644)
	s.StoreFile(f1, h1, false)
	i1, _ := os.Stat(s.FilePath(h1))
	if i1.Mode().Perm() != 0444 { t.Errorf("non-exec: %o", i1.Mode().Perm()) }
	// Exec
	h2 := store.HashBytes([]byte("exec"))
	f2 := filepath.Join(s.TmpDir(), "e")
	os.WriteFile(f2, []byte("exec"), 0644)
	s.StoreFile(f2, h2, true)
	i2, _ := os.Stat(s.FilePath(h2))
	if i2.Mode().Perm() != 0555 { t.Errorf("exec: %o", i2.Mode().Perm()) }
}

func TestQA_StoreVersionFuture(t *testing.T) {
	dir := t.TempDir()
	s := store.New(filepath.Join(dir, "store"))
	s.EnsureDirectories()
	mp := s.MetadataPath()
	os.MkdirAll(filepath.Dir(mp), 0755)
	os.WriteFile(mp, []byte(`{"store_version":999}`), 0644)
	err := s.EnsureMetadata()
	if err == nil { t.Fatal("expected error") }
	if !strings.Contains(err.Error(), "upgrade") { t.Errorf("err: %v", err) }
}

func TestQA_ManifestRoundtrip(t *testing.T) {
	dir := t.TempDir()
	s := store.New(filepath.Join(dir, "store"))
	s.EnsureDirectories()
	m := &store.Manifest{Name: "a/b", Version: "1.0.0",
		Files: []store.FileEntry{{Path: "x.php", Hash: "sha256:abc", Size: 10, Executable: false}},
		StoredAt: time.Now().UTC()}
	s.WriteManifest(m)
	m2, err := s.ReadManifest("a/b", "1.0.0")
	if err != nil { t.Fatal(err) }
	if m2.Name != "a/b" || len(m2.Files) != 1 { t.Error("roundtrip failed") }
}

// ====== EXTRACTION ======

func mkzip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	for n, c := range files { f, _ := w.Create(n); f.Write([]byte(c)) }
	w.Close()
	return buf.Bytes()
}

func TestQA_ExtractPathTraversal(t *testing.T) {
	data := mkzip(t, map[string]string{"../../../tmp/evil": "x", "ok.txt": "y"})
	dir := t.TempDir()
	store.ExtractZip(data, dir)
	if _, err := os.Stat(filepath.Join(dir, "..", "..", "..", "tmp", "evil")); err == nil { t.Error("traversal not blocked") }
	if _, err := os.Stat(filepath.Join(dir, "ok.txt")); err != nil { t.Error("normal file missing") }
}

func TestQA_ExtractTopLevelStrip(t *testing.T) {
	data := mkzip(t, map[string]string{"pkg-123/src/A.php": "<?php", "pkg-123/README": "hi"})
	dir := t.TempDir()
	store.ExtractZip(data, dir)
	store.StripTopLevelDir(dir)
	if _, err := os.Stat(filepath.Join(dir, "src", "A.php")); err != nil { t.Error("strip failed") }
}

func TestQA_ExtractEmptyArchive(t *testing.T) {
	err := store.StripTopLevelDir(t.TempDir())
	if err == nil { t.Fatal("expected error") }
	if !strings.Contains(err.Error(), "empty archive") { t.Errorf("err: %v", err) }
}

func TestQA_ExtractUnsupportedType(t *testing.T) {
	err := store.ExtractByType(nil, "rar", t.TempDir(), "pkg/x")
	if err == nil { t.Fatal("expected error") }
	if !strings.Contains(err.Error(), "unsupported dist type: rar for package pkg/x") { t.Errorf("err: %v", err) }
}

// ====== LINKER ======

func TestQA_LinkerForcedStrategy(t *testing.T) {
	for _, f := range []string{"reflink", "hardlink", "copy"} {
		s, _ := linker.DetectStrategy("", "", f)
		if s.String() != f { t.Errorf("forced %s got %s", f, s) }
	}
}

func TestQA_LinkerCopy(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "s"); dst := filepath.Join(dir, "d")
	os.WriteFile(src, []byte("data"), 0644)
	(&linker.CopyLinker{}).LinkFile(src, dst)
	d, _ := os.ReadFile(dst)
	if string(d) != "data" { t.Error("copy failed") }
}

func TestQA_LinkerHardlink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "s"); dst := filepath.Join(dir, "d")
	os.WriteFile(src, []byte("data"), 0644)
	(&linker.HardLinker{}).LinkFile(src, dst)
	si, _ := os.Stat(src); di, _ := os.Stat(dst)
	if !os.SameFile(si, di) { t.Error("not same inode") }
}

func TestQA_StaleCleanup(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "vendor.allegro.old"), 0755)
	os.MkdirAll(filepath.Join(dir, "vendor.allegro.tmp"), 0755)
	linker.CleanStaleVendorDirs(dir)
	if _, err := os.Stat(filepath.Join(dir, "vendor.allegro.old")); err == nil { t.Error("old not cleaned") }
	if _, err := os.Stat(filepath.Join(dir, "vendor.allegro.tmp")); err == nil { t.Error("tmp not cleaned") }
}

func TestQA_FlockPersists(t *testing.T) {
	dir := t.TempDir()
	lock, _ := linker.AcquireLock(dir)
	lock.Release()
	if _, err := os.Stat(filepath.Join(dir, ".allegro.lock")); err != nil { t.Error("lock file deleted") }
}

// ====== FETCHER ======

func TestQA_FetcherRetries5xx(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++; if calls <= 2 { w.WriteHeader(503); return }; w.Write([]byte("ok"))
	}))
	defer srv.Close()
	body, err := fetcher.NewClient().DownloadWithRetry(context.Background(), srv.URL)
	if err != nil { t.Fatal(err) }
	if string(body) != "ok" { t.Error("wrong body") }
}

func TestQA_FetcherNo4xxRetry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) }))
	defer srv.Close()
	_, err := fetcher.NewClient().DownloadWithRetry(context.Background(), srv.URL)
	if err == nil { t.Fatal("should fail") }
	if !strings.Contains(err.Error(), "not retryable") { t.Errorf("err: %v", err) }
}

func TestQA_FetcherSHA1Pass(t *testing.T) {
	data := []byte("content")
	h := sha1.Sum(data); shasum := hex.EncodeToString(h[:])
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data) }))
	defer srv.Close()
	res := fetcher.NewPool(1).Download(context.Background(), []fetcher.DownloadTask{{Name: "a", URL: srv.URL, Shasum: shasum}})
	if res[0].Error != nil { t.Error(res[0].Error) }
}

func TestQA_FetcherSHA1Fail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("x")) }))
	defer srv.Close()
	res := fetcher.NewPool(1).Download(context.Background(), []fetcher.DownloadTask{{Name: "a", URL: srv.URL, Shasum: "0000000000000000000000000000000000000000"}})
	if res[0].Error == nil { t.Fatal("should fail") }
}

func TestQA_RetryAfterCap(t *testing.T) {
	if d := fetcher.RetryAfterDuration("120"); d != 60*time.Second { t.Errorf("cap: %v", d) }
	if d := fetcher.RetryAfterDuration(""); d != 0 { t.Error("empty nonzero") }
}

func TestQA_IsRetryable(t *testing.T) {
	for _, c := range []int{500, 502, 503, 429} { if !fetcher.IsRetryable(c) { t.Errorf("%d should retry", c) } }
	for _, c := range []int{200, 301, 400, 403, 404} { if fetcher.IsRetryable(c) { t.Errorf("%d should not retry", c) } }
}

// ====== BIN PROXY ======

func TestQA_BinProxyDetection(t *testing.T) {
	cases := []struct{ n, c string; w autoloader.BinTargetType }{
		{"php", "<?php\necho 1;", autoloader.BinPHPNoShebang},
		{"ws", "\n  <?php\n", autoloader.BinPHPNoShebang},
		{"shebang", "#!/usr/bin/env php\n<?php\n", autoloader.BinPHPWithShebang},
		{"shebang82", "#!/usr/bin/env php8.2\n<?php\n", autoloader.BinPHPWithShebang},
		{"bom", "\xEF\xBB\xBF<?php\n", autoloader.BinPHPNoShebang},
		{"sh", "#!/bin/bash\necho", autoloader.BinNonPHP},
		{"py", "#!/usr/bin/env python\n", autoloader.BinNonPHP},
	}
	for _, tc := range cases {
		t.Run(tc.n, func(t *testing.T) {
			d := t.TempDir(); p := filepath.Join(d, "bin")
			os.WriteFile(p, []byte(tc.c), 0755)
			got, _ := autoloader.DetectBinTarget(p)
			if got != tc.w { t.Errorf("got %d want %d", got, tc.w) }
		})
	}
}

func TestQA_PHPProxyHasRequiredParts(t *testing.T) {
	out := autoloader.GeneratePHPProxyNoShebang("vendor/pkg", "bin/tool")
	for _, s := range []string{"#!/usr/bin/env php", "namespace Composer", "_composer_bin_dir", "_composer_autoload_path", "vendor/pkg/bin/tool"} {
		if !strings.Contains(out, s) { t.Errorf("missing %q", s) }
	}
}

func TestQA_ShebangProxyHasBinProxyWrapper(t *testing.T) {
	out := autoloader.GeneratePHPProxyWithShebang("v/p", "bin/x")
	for _, s := range []string{"BinProxyWrapper", "PHP_VERSION_ID < 80000", "phpvfscomposer://", "substr($path, 17)"} {
		if !strings.Contains(out, s) { t.Errorf("missing %q", s) }
	}
}

func TestQA_ShellProxyContent(t *testing.T) {
	out := autoloader.GenerateShellProxy("v/p", "bin/x")
	for _, s := range []string{"#!/bin/sh", "COMPOSER_RUNTIME_BIN_DIR", `"$@"`} {
		if !strings.Contains(out, s) { t.Errorf("missing %q", s) }
	}
}

// ====== INSTALLED.JSON ======

func TestQA_InstalledJSONFields(t *testing.T) {
	lock := &parser.ComposerLock{
		Packages: []parser.Package{{Name: "a/b", Version: "1.0.0", VersionNormalized: "1.0.0.0", Type: "library",
			Autoload: &parser.Autoload{PSR4: map[string]interface{}{"A\\": "src/"}}, Dist: &parser.Dist{Reference: "abc"}}},
		PackagesDev: []parser.Package{{Name: "c/d", Version: "2.0.0"}},
	}
	data, _ := autoloader.GenerateInstalledJSON(lock)
	var m map[string]interface{}
	json.Unmarshal(data, &m)
	if m["dev"] != true { t.Error("dev") }
	pkgs := m["packages"].([]interface{})
	if len(pkgs) != 2 { t.Fatalf("pkgs: %d", len(pkgs)) }
	p := pkgs[0].(map[string]interface{})
	if p["install-path"] != "../a/b" { t.Errorf("path: %v", p["install-path"]) }
	if p["type"] != "library" { t.Errorf("type: %v", p["type"]) }
}

func TestQA_InstalledPHPDefaults(t *testing.T) {
	lock := &parser.ComposerLock{Packages: []parser.Package{{Name: "a/b", Version: "1.0"}}}
	php := autoloader.GenerateInstalledPHP(lock, map[string]interface{}{})
	if !strings.Contains(php, "'__root__'") { t.Error("root name") }
	if !strings.Contains(php, "'dev-main'") { t.Error("dev-main") }
	if !strings.Contains(php, "'project'") { t.Error("project type") }
}

func TestQA_InstalledPHPDevRequirement(t *testing.T) {
	lock := &parser.ComposerLock{
		Packages: []parser.Package{{Name: "a/b", Version: "1.0", Dist: &parser.Dist{Reference: "x"}}},
		PackagesDev: []parser.Package{{Name: "c/d", Version: "2.0", Dist: &parser.Dist{Reference: "y"}}},
	}
	php := autoloader.GenerateInstalledPHP(lock, map[string]interface{}{})
	if !strings.Contains(php, "'dev_requirement' => false") { t.Error("a/b should not be dev") }
	if !strings.Contains(php, "'dev_requirement' => true") { t.Error("c/d should be dev") }
}

// ====== VENDOR STATE ======

func TestQA_VendorStateRoundtrip(t *testing.T) {
	dir := t.TempDir(); vd := filepath.Join(dir, "vendor"); os.MkdirAll(vd, 0755)
	linker.WriteVendorState(vd, "0.1.0", linker.Reflink, "sha256:abc", map[string]string{"a/b": "1.0"})
	s, err := linker.ReadVendorState(vd)
	if err != nil { t.Fatal(err) }
	if s.AllegroVersion != "0.1.0" || s.LinkStrategy != "reflink" || s.LockHash != "sha256:abc" { t.Error("mismatch") }
	if s.Packages["a/b"] != "1.0" { t.Error("packages") }
}

func TestQA_VendorStateCorrupt(t *testing.T) {
	d := t.TempDir(); os.WriteFile(filepath.Join(d, ".allegro-state.json"), []byte("{bad"), 0644)
	_, err := linker.ReadVendorState(d)
	if err == nil { t.Error("should fail") }
}

func TestQA_VendorStateMissingHash(t *testing.T) {
	d := t.TempDir(); os.WriteFile(filepath.Join(d, ".allegro-state.json"), []byte(`{"allegro_version":"0.1"}`), 0644)
	_, err := linker.ReadVendorState(d)
	if err == nil || !strings.Contains(err.Error(), "lock_hash") { t.Errorf("err: %v", err) }
}

// ====== MISC ======

func TestQA_ExitCodes(t *testing.T) {
	if cli.ExitSuccess != 0 || cli.ExitGeneralError != 1 || cli.ExitProjectFile != 2 ||
		cli.ExitNetworkError != 3 || cli.ExitFilesystemError != 4 || cli.ExitComposerError != 5 { t.Error("codes wrong") }
}

func TestQA_FormatBinarySize(t *testing.T) {
	for _, tc := range []struct{b int64;w string}{{0,"0 B"},{1023,"1023 B"},{1024,"1 KiB"},{1048576,"1 MiB"},{1073741824,"1 GiB"}} {
		if g := cli.FormatBinarySize(tc.b); g != tc.w { t.Errorf("%d: %q != %q", tc.b, g, tc.w) }
	}
}

func TestQA_FormatComma(t *testing.T) {
	for _, tc := range []struct{n int;w string}{{0,"0"},{999,"999"},{1000,"1,000"},{1000000,"1,000,000"},{-5000,"-5,000"}} {
		if g := cli.FormatCommaThousands(tc.n); g != tc.w { t.Errorf("%d: %q != %q", tc.n, g, tc.w) }
	}
}

func TestQA_Platform(t *testing.T) {
	if !platform.IsWindows() && !platform.IsDarwin() && !platform.IsLinux() { t.Error("no platform") }
}

func TestQA_LockHashDeterministic(t *testing.T) {
	d := t.TempDir(); p := filepath.Join(d, "composer.lock")
	os.WriteFile(p, []byte(`{}`), 0644)
	h1, _ := parser.ComputeLockHash(p); h2, _ := parser.ComputeLockHash(p)
	if h1 != h2 { t.Error("not deterministic") }
	if !strings.HasPrefix(h1, "sha256:") { t.Error("prefix") }
}

func TestQA_HasherKnown(t *testing.T) {
	if store.HashBytes([]byte("")) != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" { t.Error("empty") }
	if store.HashBytes([]byte("hello")) != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" { t.Error("hello") }
}

func TestQA_ShardPrefix(t *testing.T) {
	if store.ShardPrefix("abcdef") != "ab" { t.Error("ab") }
	if store.ShardPrefix("") != "" { t.Error("empty") }
}

func TestQA_StorePath(t *testing.T) {
	if store.ResolvePath("/f", "/e") != "/f" { t.Error("flag") }
	if store.ResolvePath("", "/e") != "/e" { t.Error("env") }
	if !strings.Contains(store.ResolvePath("", ""), ".allegro/store") { t.Error("default") }
}

func TestQA_ComposerDetectEnv(t *testing.T) {
	d := t.TempDir(); b := filepath.Join(d, "c"); os.WriteFile(b, []byte("x"), 0755)
	t.Setenv("ALLEGRO_COMPOSER_PATH", b)
	g, err := autoloader.FindComposer(d)
	if err != nil { t.Fatal(err) }
	if g != b { t.Error("env not used") }
}

func TestQA_ComposerNotFound(t *testing.T) {
	d := t.TempDir(); t.Setenv("ALLEGRO_COMPOSER_PATH", ""); t.Setenv("PATH", d)
	_, err := autoloader.FindComposer(d)
	if err == nil { t.Fatal("should fail") }
}

func TestQA_BinBasename(t *testing.T) {
	if autoloader.BinBasename("bin/phpunit") != "phpunit" { t.Error("phpunit") }
	if autoloader.BinBasename("phpunit") != "phpunit" { t.Error("bare") }
	if autoloader.BinBasename("a/b/c") != "c" { t.Error("deep") }
}
