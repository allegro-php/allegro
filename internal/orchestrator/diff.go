package orchestrator

import "github.com/allegro-php/allegro/internal/parser"

// PackageDiff represents the difference between installed and desired packages.
type PackageDiff struct {
	Added     []parser.Package  // in new lock, not in state
	Removed   []parser.Package  // in state, not in new lock
	Updated   []PackageUpdate   // in both, version differs
	Unchanged []parser.Package  // in both, version matches
}

// PackageUpdate represents a version change for a single package.
type PackageUpdate struct {
	Name       string
	OldVersion string
	NewVersion string
}
