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
	if len(diff.Added) != 1 { t.Error("Added") }
	if len(diff.Removed) != 1 { t.Error("Removed") }
	if len(diff.Updated) != 1 { t.Error("Updated") }
	if len(diff.Unchanged) != 1 { t.Error("Unchanged") }
}

func TestComputeDiffAdded(t *testing.T) {
	oldPkgs := map[string]string{"a/b": "1.0"}
	newPkgs := []parser.Package{{Name: "a/b", Version: "1.0"}, {Name: "c/d", Version: "2.0"}}
	diff := ComputeDiff(oldPkgs, newPkgs)
	if len(diff.Added) != 1 || diff.Added[0].Name != "c/d" {
		t.Errorf("added = %v, want [c/d]", diff.Added)
	}
}

func TestComputeDiffRemoved(t *testing.T) {
	oldPkgs := map[string]string{"a/b": "1.0", "c/d": "2.0"}
	newPkgs := []parser.Package{{Name: "a/b", Version: "1.0"}}
	diff := ComputeDiff(oldPkgs, newPkgs)
	if len(diff.Removed) != 1 || diff.Removed[0].Name != "c/d" {
		t.Errorf("removed = %v, want [c/d]", diff.Removed)
	}
}

func TestComputeDiffUpdated(t *testing.T) {
	oldPkgs := map[string]string{"a/b": "1.0"}
	newPkgs := []parser.Package{{Name: "a/b", Version: "2.0"}}
	diff := ComputeDiff(oldPkgs, newPkgs)
	if len(diff.Updated) != 1 || diff.Updated[0].OldVersion != "1.0" || diff.Updated[0].NewVersion != "2.0" {
		t.Errorf("updated = %v", diff.Updated)
	}
}

func TestComputeDiffUnchanged(t *testing.T) {
	oldPkgs := map[string]string{"a/b": "1.0"}
	newPkgs := []parser.Package{{Name: "a/b", Version: "1.0"}}
	diff := ComputeDiff(oldPkgs, newPkgs)
	if len(diff.Unchanged) != 1 || diff.Unchanged[0].Name != "a/b" {
		t.Errorf("unchanged = %v", diff.Unchanged)
	}
}

func TestComputeDiffCaseInsensitive(t *testing.T) {
	oldPkgs := map[string]string{"Monolog/Monolog": "3.0"}
	newPkgs := []parser.Package{{Name: "monolog/monolog", Version: "3.0"}}
	diff := ComputeDiff(oldPkgs, newPkgs)
	if len(diff.Unchanged) != 1 {
		t.Errorf("case-insensitive match failed: added=%d removed=%d updated=%d unchanged=%d",
			len(diff.Added), len(diff.Removed), len(diff.Updated), len(diff.Unchanged))
	}
}

func TestComputeDiffEmpty(t *testing.T) {
	diff := ComputeDiff(map[string]string{}, []parser.Package{})
	if len(diff.Added)+len(diff.Removed)+len(diff.Updated)+len(diff.Unchanged) != 0 {
		t.Error("empty diff should have no entries")
	}
}

func TestComputeDiffFullScenario(t *testing.T) {
	oldPkgs := map[string]string{
		"monolog/monolog":     "3.8.0",
		"phpunit/phpunit":     "10.0.0",
		"laravel/framework":   "11.0.0",
	}
	newPkgs := []parser.Package{
		{Name: "monolog/monolog", Version: "3.9.0"},       // updated
		{Name: "laravel/framework", Version: "11.0.0"},    // unchanged
		{Name: "symfony/mailer", Version: "7.2.0"},        // added
	}
	diff := ComputeDiff(oldPkgs, newPkgs)
	if len(diff.Added) != 1 || diff.Added[0].Name != "symfony/mailer" {
		t.Errorf("added = %v", diff.Added)
	}
	if len(diff.Removed) != 1 || diff.Removed[0].Name != "phpunit/phpunit" {
		t.Errorf("removed = %v", diff.Removed)
	}
	if len(diff.Updated) != 1 || diff.Updated[0].Name != "monolog/monolog" {
		t.Errorf("updated = %v", diff.Updated)
	}
	if len(diff.Unchanged) != 1 || diff.Unchanged[0].Name != "laravel/framework" {
		t.Errorf("unchanged = %v", diff.Unchanged)
	}
}
