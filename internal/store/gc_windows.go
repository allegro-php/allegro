//go:build windows

package store

import "log"

// GarbageCollect on Windows — no flock, runs unprotected with warning.
func GarbageCollect(storePath, registryPath string, staleDays int, dryRun bool) (*GCResult, error) {
	log.Printf("warning: file locking not available on Windows; GC runs unprotected")
	return garbageCollectImpl(storePath, registryPath, staleDays, dryRun)
}
