//go:build windows

package linker

import "log"

// FileLock is a no-op on Windows (flock not available).
type FileLock struct{}

// AcquireLock returns a no-op lock on Windows.
// File locking is not available; concurrent install protection is degraded.
func AcquireLock(projectDir string) (*FileLock, error) {
	log.Printf("warning: file locking not available on Windows; concurrent install protection is disabled")
	return &FileLock{}, nil
}

// Release is a no-op on Windows.
func (fl *FileLock) Release() error {
	return nil
}
