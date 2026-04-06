package orchestrator

import (
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/allegro-php/allegro/internal/linker"
	"github.com/allegro-php/allegro/internal/store"
)

// CollectDirectories extracts all unique parent directories from link ops.
// Returns sorted list for sequential creation (avoids MkdirAll races).
func CollectDirectories(ops []LinkOp) []string {
	seen := make(map[string]bool)
	for _, op := range ops {
		dir := filepath.Dir(op.Dst)
		seen[dir] = true
	}
	dirs := make([]string, 0, len(seen))
	for d := range seen {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs) // parent dirs sort before children
	return dirs
}

// CollectLinkOps builds a list of link operations from CAS manifests.
func CollectLinkOps(s *store.Store, pkgName string, manifest *store.Manifest, vendorTmp string) []LinkOp {
	var ops []LinkOp
	for _, f := range manifest.Files {
		hash := f.Hash
		if len(hash) > 7 && hash[:7] == "sha256:" {
			hash = hash[7:]
		}
		ops = append(ops, LinkOp{
			Src:        s.FilePath(hash),
			Dst:        filepath.Join(vendorTmp, pkgName, f.Path),
			Executable: f.Executable,
		})
	}
	return ops
}

// ParallelLink executes link operations using a goroutine worker pool.
func ParallelLink(ops []LinkOp, lnk linker.Linker, strategy linker.Strategy, workers int) error {
	if workers < 1 {
		workers = 1
	}

	// 1. Create directories sequentially
	dirs := CollectDirectories(ops)
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	// 2. Link files in parallel with cancellation on first error
	var firstErr error
	var errOnce sync.Once
	opCh := make(chan LinkOp, len(ops))
	done := make(chan struct{})

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for op := range opCh {
				select {
				case <-done:
					return // cancelled
				default:
				}
				if err := lnk.LinkFile(op.Src, op.Dst); err != nil {
					errOnce.Do(func() {
						firstErr = err
						close(done) // signal other workers to stop
					})
					return
				}
				if strategy != linker.Hardlink {
					perm := os.FileMode(0644)
					if op.Executable {
						perm = 0755
					}
					os.Chmod(op.Dst, perm)
				}
			}
		}()
	}

	for _, op := range ops {
		select {
		case <-done:
			break // stop sending if cancelled
		case opCh <- op:
		}
	}
	close(opCh)
	wg.Wait()

	if firstErr != nil {
		return firstErr
	}
	return nil
}
