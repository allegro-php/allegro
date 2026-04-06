package cli

import (
	"testing"

	"github.com/allegro-php/allegro/internal/orchestrator"
	"github.com/allegro-php/allegro/internal/parser"
)

func TestPrintDiffDoesNotPanic(t *testing.T) {
	// Just verify it doesn't panic with various inputs
	diff := orchestrator.PackageDiff{
		Added:   []parser.Package{{Name: "a/b", Version: "1.0"}},
		Updated: []orchestrator.PackageUpdate{{Name: "c/d", OldVersion: "1.0", NewVersion: "2.0"}},
		Removed: []parser.Package{{Name: "e/f", Version: "3.0"}},
	}
	flagNoColor = true
	PrintDiff(diff) // should not panic
	flagNoColor = false
}

func TestIsTTY(t *testing.T) {
	// In test environment, stdout is usually not a TTY
	_ = IsTTY() // just verify it doesn't panic
}
