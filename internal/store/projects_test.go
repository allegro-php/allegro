package store

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProjectEntryFields(t *testing.T) {
	e := ProjectEntry{
		Path:        "/Users/dev/laravel-app",
		LastInstall: time.Date(2026, 4, 6, 8, 0, 0, 0, time.UTC),
		LockHash:    "sha256:abc",
		Packages:    map[string]string{"monolog/monolog": "3.9.0"},
	}
	if e.Path != "/Users/dev/laravel-app" { t.Error("Path") }
	if e.LockHash != "sha256:abc" { t.Error("LockHash") }
	if e.Packages["monolog/monolog"] != "3.9.0" { t.Error("Packages") }
}

func TestProjectRegistryJSONRoundtrip(t *testing.T) {
	r := ProjectRegistry{
		Projects: []ProjectEntry{
			{Path: "/app1", LockHash: "sha256:a", Packages: map[string]string{"a/b": "1.0"}},
			{Path: "/app2", LockHash: "sha256:b", Packages: map[string]string{"c/d": "2.0"}},
		},
	}
	data, err := json.Marshal(r)
	if err != nil { t.Fatal(err) }
	var r2 ProjectRegistry
	if err := json.Unmarshal(data, &r2); err != nil { t.Fatal(err) }
	if len(r2.Projects) != 2 { t.Errorf("projects count = %d", len(r2.Projects)) }
	if r2.Projects[0].Path != "/app1" { t.Error("first project path") }
}

func TestDefaultRegistryPath(t *testing.T) {
	p := DefaultRegistryPath()
	if !strings.Contains(p, ".allegro") || !strings.HasSuffix(p, "projects.json") {
		t.Errorf("registry path = %q", p)
	}
}

func TestRegisterProject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects.json")

	entry := ProjectEntry{
		Path:     "/app/test",
		LockHash: "sha256:abc",
		Packages: map[string]string{"a/b": "1.0"},
	}
	if err := RegisterProject(path, entry); err != nil {
		t.Fatal(err)
	}
	reg, err := ReadRegistry(path)
	if err != nil { t.Fatal(err) }
	if len(reg.Projects) != 1 || reg.Projects[0].Path != "/app/test" {
		t.Errorf("projects = %+v", reg.Projects)
	}

	// Register again — should update, not duplicate
	entry.LockHash = "sha256:def"
	if err := RegisterProject(path, entry); err != nil { t.Fatal(err) }
	reg2, _ := ReadRegistry(path)
	if len(reg2.Projects) != 1 {
		t.Errorf("duplicate entry: %d projects", len(reg2.Projects))
	}
	if reg2.Projects[0].LockHash != "sha256:def" {
		t.Error("should update existing entry")
	}
}

func TestReadRegistryMissing(t *testing.T) {
	reg, err := ReadRegistry("/nonexistent/projects.json")
	if err != nil { t.Fatal(err) }
	if len(reg.Projects) != 0 {
		t.Error("missing file should return empty registry")
	}
}
