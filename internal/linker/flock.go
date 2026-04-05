//go:build !windows

package linker

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const (
	lockFileName = ".allegro.lock"
	lockTimeout  = 30 * time.Second
)

// FileLock represents an advisory file lock on the project directory.
type FileLock struct {
	file *os.File
}

// AcquireLock creates/opens the lock file and acquires an exclusive flock.
// Returns error if lock cannot be acquired within 30 seconds.
func AcquireLock(projectDir string) (*FileLock, error) {
	lockPath := filepath.Join(projectDir, lockFileName)

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("cannot create lock file %s: %w", lockPath, err)
	}

	deadline := time.Now().Add(lockTimeout)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return &FileLock{file: f}, nil
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, fmt.Errorf("another allegro process is running (lock timeout after %s)", lockTimeout)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// Release releases the file lock. The lock file is NOT deleted.
func (fl *FileLock) Release() error {
	if fl.file == nil {
		return nil
	}
	syscall.Flock(int(fl.file.Fd()), syscall.LOCK_UN)
	return fl.file.Close()
}
