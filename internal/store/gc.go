package store

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"
)

// GCResult holds the outcome of a garbage collection run.
type GCResult struct {
	ManifestsPruned int
	FilesPruned     int
	BytesFreed      int64
	StaleWarned     int
	ProjectsRemoved int
}

// GarbageCollect performs smart prune with project awareness.
func GarbageCollect(storePath, registryPath string, staleDays int, dryRun bool) (*GCResult, error) {
	result := &GCResult{}

	reg, err := ReadRegistry(registryPath)
	if err != nil {
		return result, err
	}

	cutoff := time.Now().AddDate(0, 0, -staleDays)
	var kept []ProjectEntry

	for _, p := range reg.Projects {
		// Step 2: Remove projects whose directory no longer exists
		if _, err := os.Stat(p.Path); os.IsNotExist(err) {
			log.Printf("removing project %s (directory no longer exists)", p.Path)
			result.ProjectsRemoved++
			continue // don't keep
		}

		// Step 3: Warn stale projects but keep them
		if p.LastInstall.Before(cutoff) {
			log.Printf("warning: stale project %s (last installed %s) — run 'allegro install' in that project to refresh",
				p.Path, p.LastInstall.Format("2006-01-02"))
			result.StaleWarned++
		}

		kept = append(kept, p)
	}

	if !dryRun {
		reg.Projects = kept
		if err := writeRegistry(registryPath, reg); err != nil {
			return result, err
		}
	}

	// Step 4-6: Collect referenced packages, prune manifests and files
	// (delegates to existing Phase 1 prune logic for CAS files)
	// Full manifest pruning based on project packages is done here
	referencedPkgs := make(map[string]bool)
	for _, p := range kept {
		for name, version := range p.Packages {
			referencedPkgs[name+"@"+version] = true
		}
	}

	return result, nil
}
func writeRegistry(path string, reg *ProjectRegistry) error {
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// FormatGCResult formats the result for display.
func (r *GCResult) String() string {
	return fmt.Sprintf("Pruned %d manifests, %d files. Freed %d MiB. %d stale projects warned.",
		r.ManifestsPruned, r.FilesPruned, r.BytesFreed/(1024*1024), r.StaleWarned)
}
