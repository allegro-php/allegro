package orchestrator

import "github.com/allegro-php/allegro/internal/parser"

// InstallPlan represents the plan for an install operation.
type InstallPlan struct {
	NewPackages    []parser.Package // Need to download
	CachedPackages []parser.Package // Already in CAS
	SkippedPackages []SkippedPackage // Skipped (null dist, path type)
	AllPackages    []parser.Package // All installable packages
}

// SkippedPackage records why a package was skipped.
type SkippedPackage struct {
	Name   string
	Reason string
}
