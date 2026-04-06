package linker

import (
	"context"
	"testing"
)

func TestAcquireAndReleaseLock(t *testing.T) {
	dir := t.TempDir()

	lock, err := AcquireLock(context.Background(), dir)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}

	if err := lock.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
}

func TestAcquireLockCreatesFile(t *testing.T) {
	dir := t.TempDir()

	lock, err := AcquireLock(context.Background(), dir)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	defer lock.Release()

	if lock.file == nil {
		t.Error("lock file is nil")
	}
}

func TestAcquireLockReentrant(t *testing.T) {
	dir := t.TempDir()

	lock1, err := AcquireLock(context.Background(), dir)
	if err != nil {
		t.Fatalf("AcquireLock 1: %v", err)
	}
	defer lock1.Release()
}
