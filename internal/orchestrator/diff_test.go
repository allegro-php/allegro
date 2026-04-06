package orchestrator

import (
	"testing"

	"github.com/allegro-php/allegro/internal/parser"
)

func TestPackageDiffFields(t *testing.T) {
	diff := PackageDiff{
		Added:     []parser.Package{{Name: "a/b", Version: "1.0"}},
		Removed:   []parser.Package{{Name: "c/d", Version: "2.0"}},
		Updated:   []PackageUpdate{{Name: "e/f", OldVersion: "1.0", NewVersion: "2.0"}},
		Unchanged: []parser.Package{{Name: "g/h", Version: "3.0"}},
	}
	if len(diff.Added) != 1 || diff.Added[0].Name != "a/b" {
		t.Error("Added field")
	}
	if len(diff.Removed) != 1 {
		t.Error("Removed field")
	}
	if len(diff.Updated) != 1 || diff.Updated[0].OldVersion != "1.0" {
		t.Error("Updated field")
	}
	if len(diff.Unchanged) != 1 {
		t.Error("Unchanged field")
	}
}

func TestPackageUpdateFields(t *testing.T) {
	u := PackageUpdate{Name: "monolog/monolog", OldVersion: "3.8.0", NewVersion: "3.9.0"}
	if u.Name != "monolog/monolog" || u.OldVersion != "3.8.0" || u.NewVersion != "3.9.0" {
		t.Error("PackageUpdate fields")
	}
}
