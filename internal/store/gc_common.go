package store

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
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

// FormatGCResult formats the result for display.
func (r *GCResult) String() string {
	return fmt.Sprintf("Pruned %d manifests, %d files. Freed %d MiB. %d stale projects warned.",
		r.ManifestsPruned, r.FilesPruned, r.BytesFreed/(1024*1024), r.StaleWarned)
}

// garbageCollectImpl is the shared GC logic (no locking — caller must lock).
func garbageCollectImpl(storePath, registryPath string, staleDays int, dryRun bool) (*GCResult, error) {
	result := &GCResult{}

	reg, err := ReadRegistry(registryPath)
	if err != nil {
		return result, err
	}

	cutoff := time.Now().AddDate(0, 0, -staleDays)
	var kept []ProjectEntry

	for _, p := range reg.Projects {
		if _, err := os.Stat(p.Path); os.IsNotExist(err) {
			log.Printf("removing project %s (directory no longer exists)", p.Path)
			result.ProjectsRemoved++
			continue
		}
		if p.LastInstall.Before(cutoff) {
			log.Printf("warning: stale project %s (last installed %s) — run 'allegro install' in that project to refresh",
				p.Path, p.LastInstall.Format("2006-01-02"))
			result.StaleWarned++
		}
		kept = append(kept, p)
	}

	if !dryRun {
		reg.Projects = kept
		if err := writeRegistryAtomic(registryPath, reg); err != nil {
			return result, err
		}

		referencedPkgs := make(map[string]bool)
		for _, p := range kept {
			for name, version := range p.Packages {
				referencedPkgs[name+"@"+version] = true
			}
		}

		s := New(storePath)

		packagesDir := filepath.Join(s.Root, "packages")
		referencedHashes := make(map[string]bool)
		filepath.Walk(packagesDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || filepath.Ext(path) != ".json" {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil { return nil }
			var m Manifest
			if json.Unmarshal(data, &m) != nil { return nil }

			key := m.Name + "@" + m.Version
			if !referencedPkgs[key] {
				os.Remove(path)
				result.ManifestsPruned++
			} else {
				for _, f := range m.Files {
					hash := f.Hash
					if len(hash) > 7 && hash[:7] == "sha256:" { hash = hash[7:] }
					referencedHashes[hash] = true
				}
			}
			return nil
		})

		filesDir := filepath.Join(s.Root, "files")
		filepath.Walk(filesDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() { return nil }
			if !referencedHashes[filepath.Base(path)] {
				result.BytesFreed += info.Size()
				os.Remove(path)
				result.FilesPruned++
			}
			return nil
		})
	}

	return result, nil
}

func writeRegistryAtomic(path string, reg *ProjectRegistry) error {
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil { return err }
	return WriteFileAtomic(path, data, 0644)
}
