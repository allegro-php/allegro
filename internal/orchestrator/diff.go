package orchestrator

import (
	"strings"

	"github.com/allegro-php/allegro/internal/parser"
)

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

// ComputeDiff compares the old installed packages (from state file) with the
// new desired packages (from composer.lock). Name comparison is case-insensitive.
func ComputeDiff(oldPkgs map[string]string, newPkgs []parser.Package) PackageDiff {
	var diff PackageDiff

	// Build case-insensitive map of old packages
	oldLower := make(map[string]string, len(oldPkgs))
	oldOrigName := make(map[string]string, len(oldPkgs))
	for name, version := range oldPkgs {
		lower := strings.ToLower(name)
		oldLower[lower] = version
		oldOrigName[lower] = name
	}

	// Track which old packages are seen in new
	seen := make(map[string]bool)

	for _, pkg := range newPkgs {
		lower := strings.ToLower(pkg.Name)
		seen[lower] = true

		oldVersion, exists := oldLower[lower]
		if !exists {
			diff.Added = append(diff.Added, pkg)
		} else if oldVersion != pkg.Version {
			diff.Updated = append(diff.Updated, PackageUpdate{
				Name:       pkg.Name,
				OldVersion: oldVersion,
				NewVersion: pkg.Version,
			})
		} else {
			diff.Unchanged = append(diff.Unchanged, pkg)
		}
	}

	// Packages in old but not in new → removed
	for lower, version := range oldLower {
		if !seen[lower] {
			diff.Removed = append(diff.Removed, parser.Package{
				Name:    oldOrigName[lower],
				Version: version,
			})
		}
	}

	return diff
}
