package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/allegro-php/allegro/internal/linker"
	"github.com/allegro-php/allegro/internal/store"
)

// VerifyIssue represents a single verification issue.
type VerifyIssue struct {
	Package    string
	File       string
	Type       string // "missing", "modified", "permission"
	Detail     string
	Executable bool   // whether the file should be executable (for fix mode)
}

// VerifyResult holds the outcome of a verify operation.
type VerifyResult struct {
	TotalPackages int
	TotalFiles    int64
	Issues        []VerifyIssue
	OKPackages    int
	FailPackages  int
}

// VerifyVendor checks vendor integrity against CAS manifests.
func VerifyVendor(vendorDir string, s *store.Store, state *linker.VendorState, workers int) (*VerifyResult, error) {
	result := &VerifyResult{}

	// Determine expected permissions from link strategy
	var regPerm, execPerm os.FileMode
	switch state.LinkStrategy {
	case "hardlink":
		regPerm, execPerm = 0444, 0555
	default:
		regPerm, execPerm = 0644, 0755
	}

	for pkgName, pkgVersion := range state.Packages {
		result.TotalPackages++

		manifest, err := s.ReadManifest(pkgName, pkgVersion)
		if err != nil {
			result.Issues = append(result.Issues, VerifyIssue{
				Package: pkgName, Type: "missing", Detail: "manifest missing from store",
			})
			result.FailPackages++
			continue
		}

		var pkgIssues []VerifyIssue
		var issueCount int64

		// Parallel file verification
		fileCh := make(chan store.FileEntry, len(manifest.Files))
		issueCh := make(chan VerifyIssue, len(manifest.Files))

		var wg sync.WaitGroup
		w := workers
		if w < 1 { w = 1 }
		for i := 0; i < w; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for f := range fileCh {
					atomic.AddInt64(&result.TotalFiles, 1)
					vendorPath := filepath.Join(vendorDir, pkgName, f.Path)

					// Check exists
					info, err := os.Stat(vendorPath)
					if err != nil {
						issueCh <- VerifyIssue{Package: pkgName, File: f.Path, Type: "missing", Detail: "file not found", Executable: f.Executable}
						atomic.AddInt64(&issueCount, 1)
						continue
					}

					// Check hash
					hash := f.Hash
					if len(hash) > 7 && hash[:7] == "sha256:" {
						hash = hash[7:]
					}
					actualHash, err := store.HashFile(vendorPath)
					if err != nil {
						issueCh <- VerifyIssue{Package: pkgName, File: f.Path, Type: "modified", Detail: fmt.Sprintf("hash error: %v", err), Executable: f.Executable}
						atomic.AddInt64(&issueCount, 1)
						continue
					}
					if actualHash != hash {
						issueCh <- VerifyIssue{Package: pkgName, File: f.Path, Type: "modified",
							Detail: fmt.Sprintf("expected %s, got %s", hash[:8], actualHash[:8]), Executable: f.Executable}
						atomic.AddInt64(&issueCount, 1)
						continue
					}

					// Check permissions
					expectedPerm := regPerm
					if f.Executable { expectedPerm = execPerm }
					if info.Mode().Perm() != expectedPerm {
						issueCh <- VerifyIssue{Package: pkgName, File: f.Path, Type: "permission",
							Detail: fmt.Sprintf("expected %o, got %o", expectedPerm, info.Mode().Perm()), Executable: f.Executable}
						atomic.AddInt64(&issueCount, 1)
					}
				}
			}()
		}

		for _, f := range manifest.Files {
			fileCh <- f
		}
		close(fileCh)
		wg.Wait()
		close(issueCh)

		for issue := range issueCh {
			pkgIssues = append(pkgIssues, issue)
		}

		if len(pkgIssues) > 0 {
			result.Issues = append(result.Issues, pkgIssues...)
			result.FailPackages++
		} else {
			result.OKPackages++
		}
	}

	return result, nil
}
