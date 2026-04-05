package autoloader

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allegro-php/allegro/internal/parser"
)

func testLock() *parser.ComposerLock {
	return &parser.ComposerLock{
		Packages: []parser.Package{
			{
				Name:              "monolog/monolog",
				Version:           "3.9.0",
				VersionNormalized: "3.9.0.0",
				Type:              "library",
				Dist:              &parser.Dist{Type: "zip", Reference: "abc123"},
				Autoload:          &parser.Autoload{PSR4: map[string]interface{}{"Monolog\\": "src/"}},
			},
		},
		PackagesDev: []parser.Package{
			{
				Name:    "phpunit/phpunit",
				Version: "10.0.0",
				Dist:    &parser.Dist{Reference: "def456"},
			},
		},
	}
}

func TestGenerateInstalledJSON(t *testing.T) {
	lock := testLock()
	data, err := GenerateInstalledJSON(lock)
	if err != nil {
		t.Fatal(err)
	}

	var result InstalledJSON
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}

	if !result.Dev {
		t.Error("dev should be true")
	}
	if len(result.Packages) != 2 {
		t.Errorf("packages = %d, want 2", len(result.Packages))
	}
	if result.Packages[0].InstallPath != "../monolog/monolog" {
		t.Errorf("install-path = %q", result.Packages[0].InstallPath)
	}
	if result.Packages[0].Type != "library" {
		t.Errorf("type = %q, want library", result.Packages[0].Type)
	}
	if len(result.DevPackageNames) != 1 || result.DevPackageNames[0] != "phpunit/phpunit" {
		t.Errorf("dev-package-names = %v", result.DevPackageNames)
	}
}

func TestGenerateInstalledJSONDefaultType(t *testing.T) {
	lock := &parser.ComposerLock{
		Packages: []parser.Package{{Name: "a/b", Version: "1.0.0"}},
	}
	data, _ := GenerateInstalledJSON(lock)
	var result InstalledJSON
	json.Unmarshal(data, &result)
	if result.Packages[0].Type != "library" {
		t.Errorf("default type = %q, want library", result.Packages[0].Type)
	}
}

func TestGenerateInstalledPHP(t *testing.T) {
	lock := testLock()
	composerJSON := map[string]interface{}{"name": "my/project", "type": "project"}

	php := GenerateInstalledPHP(lock, composerJSON)

	if !strings.Contains(php, "'name' => 'my/project'") {
		t.Error("missing root name")
	}
	if !strings.Contains(php, "'monolog/monolog'") {
		t.Error("missing monolog package")
	}
	if !strings.Contains(php, "'reference' => 'abc123'") {
		t.Error("missing reference")
	}
	if !strings.Contains(php, "'dev_requirement' => false") {
		t.Error("monolog should not be dev")
	}
	if !strings.Contains(php, "'dev_requirement' => true") {
		t.Error("phpunit should be dev")
	}
	if !strings.Contains(php, "__DIR__ . '/../monolog/monolog'") {
		t.Error("missing install_path")
	}
}

func TestGenerateInstalledPHPDefaults(t *testing.T) {
	lock := testLock()
	composerJSON := map[string]interface{}{} // empty

	php := GenerateInstalledPHP(lock, composerJSON)

	if !strings.Contains(php, "'name' => '__root__'") {
		t.Error("should fallback to __root__")
	}
	if !strings.Contains(php, "'version' => 'dev-main'") {
		t.Error("should fallback to dev-main")
	}
}

func TestWriteInstalledFiles(t *testing.T) {
	lock := testLock()
	dir := t.TempDir()
	vendorDir := filepath.Join(dir, "vendor")

	err := WriteInstalledFiles(vendorDir, lock, map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(vendorDir, "composer/installed.json")); err != nil {
		t.Error("installed.json not created")
	}
	if _, err := os.Stat(filepath.Join(vendorDir, "composer/installed.php")); err != nil {
		t.Error("installed.php not created")
	}
}
