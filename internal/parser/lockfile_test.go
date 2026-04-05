package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLockFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "composer.lock")
	content := `{
		"packages": [
			{"name":"monolog/monolog","version":"3.9.0","version_normalized":"3.9.0.0","type":"library",
			 "dist":{"type":"zip","url":"https://example.com/m.zip","reference":"abc","shasum":""}},
			{"name":"php","version":"8.3.0"}
		],
		"packages-dev": [
			{"name":"phpunit/phpunit","version":"10.0.0"}
		],
		"content-hash":"abc123"
	}`
	os.WriteFile(path, []byte(content), 0644)

	lock, err := ParseLockFile(path)
	if err != nil {
		t.Fatalf("ParseLockFile: %v", err)
	}
	if len(lock.Packages) != 2 {
		t.Errorf("packages = %d, want 2", len(lock.Packages))
	}
	if len(lock.PackagesDev) != 1 {
		t.Errorf("packages-dev = %d, want 1", len(lock.PackagesDev))
	}
}

func TestParseLockFileMissing(t *testing.T) {
	_, err := ParseLockFile("/nonexistent/composer.lock")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err)
	}
}

func TestParseLockFileInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "composer.lock")
	os.WriteFile(path, []byte(`{invalid`), 0644)

	_, err := ParseLockFile(path)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "line") {
		t.Errorf("error should mention line number: %v", err)
	}
}

func TestIsPlatformPackage(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"php", true},
		{"php-64bit", true},
		{"ext-json", true},
		{"ext-mbstring", true},
		{"lib-libxml", true},
		{"monolog/monolog", false},
		{"laravel/framework", false},
	}
	for _, tt := range tests {
		if got := IsPlatformPackage(tt.name); got != tt.want {
			t.Errorf("IsPlatformPackage(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestFilterInstallable(t *testing.T) {
	packages := []Package{
		{Name: "monolog/monolog"},
		{Name: "php"},
		{Name: "ext-json"},
		{Name: "laravel/framework"},
	}
	result := FilterInstallable(packages)
	if len(result) != 2 {
		t.Errorf("filtered = %d, want 2", len(result))
	}
}

func TestMergePackages(t *testing.T) {
	lock := &ComposerLock{
		Packages:    []Package{{Name: "a/b"}, {Name: "php"}},
		PackagesDev: []Package{{Name: "c/d"}, {Name: "ext-json"}},
	}
	merged := MergePackages(lock)
	if len(merged) != 2 {
		t.Errorf("merged = %d, want 2 (a/b + c/d)", len(merged))
	}
}

func TestDevPackageNames(t *testing.T) {
	lock := &ComposerLock{
		PackagesDev: []Package{{Name: "phpunit/phpunit"}, {Name: "ext-xdebug"}},
	}
	names := DevPackageNames(lock)
	if len(names) != 1 || names[0] != "phpunit/phpunit" {
		t.Errorf("dev names = %v, want [phpunit/phpunit]", names)
	}
}

func TestIsDevPackage(t *testing.T) {
	lock := &ComposerLock{
		PackagesDev: []Package{{Name: "phpunit/phpunit"}},
	}
	if !IsDevPackage("phpunit/phpunit", lock) {
		t.Error("phpunit should be dev")
	}
	if IsDevPackage("monolog/monolog", lock) {
		t.Error("monolog should not be dev")
	}
}
