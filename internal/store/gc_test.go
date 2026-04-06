package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGCRemoveDeletedProjects(t *testing.T) {
	dir := t.TempDir()
	regPath := filepath.Join(dir, "projects.json")

	entry := ProjectEntry{
		Path:     "/nonexistent/project",
		LockHash: "sha256:abc",
		Packages: map[string]string{"a/b": "1.0"},
	}
	RegisterProject(regPath, entry)

	result, err := GarbageCollect(dir, regPath, 90, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.ProjectsRemoved != 1 {
		t.Errorf("removed = %d, want 1", result.ProjectsRemoved)
	}
}

func TestGCWarnStaleProjects(t *testing.T) {
	dir := t.TempDir()
	regPath := filepath.Join(dir, "projects.json")

	projectDir := filepath.Join(dir, "myproject")
	os.MkdirAll(projectDir, 0755)

	reg := &ProjectRegistry{
		Projects: []ProjectEntry{{
			Path:        projectDir,
			LastInstall: time.Now().AddDate(0, 0, -100),
			LockHash:    "sha256:abc",
			Packages:    map[string]string{"a/b": "1.0"},
		}},
	}
	data, _ := json.MarshalIndent(reg, "", "  ")
	os.WriteFile(regPath, data, 0644)

	result, err := GarbageCollect(dir, regPath, 90, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.StaleWarned != 1 {
		t.Errorf("stale warned = %d, want 1", result.StaleWarned)
	}
	if result.ProjectsRemoved != 0 {
		t.Errorf("removed = %d, want 0", result.ProjectsRemoved)
	}
}

func TestGCDryRun(t *testing.T) {
	dir := t.TempDir()
	regPath := filepath.Join(dir, "projects.json")

	entry := ProjectEntry{
		Path:     "/nonexistent/project",
		Packages: map[string]string{},
	}
	RegisterProject(regPath, entry)

	result, err := GarbageCollect(dir, regPath, 90, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.ProjectsRemoved != 1 {
		t.Errorf("dry run should count removed")
	}

	reg, _ := ReadRegistry(regPath)
	if len(reg.Projects) != 1 {
		t.Error("dry run should not modify registry")
	}
}
