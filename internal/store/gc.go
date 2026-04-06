//go:build !windows

package store

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// GarbageCollect performs smart prune with project awareness (Unix: with flock).
func GarbageCollect(storePath, registryPath string, staleDays int, dryRun bool) (*GCResult, error) {
	lockPath := filepath.Join(filepath.Dir(registryPath), "projects.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return &GCResult{}, fmt.Errorf("create gc lock: %w", err)
	}
	defer lockFile.Close()
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return &GCResult{}, fmt.Errorf("acquire gc lock: %w", err)
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)

	return garbageCollectImpl(storePath, registryPath, staleDays, dryRun)
}
