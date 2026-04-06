package store

import (
	"encoding/json"
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
