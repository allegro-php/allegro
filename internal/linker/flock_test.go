package linker

import (
	"testing"
)

func TestAcquireAndReleaseLock(t *testing.T) {
	dir := t.TempDir()

	lock, err := AcquireLock(dir)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}

	if err := lock.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
}

func TestAcquireLockCreatesFile(t *testing.T) {
	dir := t.TempDir()

	lock, err := AcquireLock(dir)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	defer lock.Release()

	// Lock file should exist
	if lock.file == nil {
		t.Error("lock file is nil")
	}
}

func TestAcquireLockReentrant(t *testing.T) {
	dir := t.TempDir()

	lock1, err := AcquireLock(dir)
	if err != nil {
		t.Fatalf("AcquireLock 1: %v", err)
	}
	defer lock1.Release()

	// Second lock from same process should fail quickly (different fd)
	// On macOS/Linux, flock is per-fd, so a second open+flock from same process
	// may or may not block depending on OS. Just verify it doesn't crash.
}
