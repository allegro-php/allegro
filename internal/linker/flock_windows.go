//go:build windows

package linker

import "fmt"

// FileLock is a no-op stub on Windows.
type FileLock struct{}

// AcquireLock is not supported on Windows.
func AcquireLock(projectDir string) (*FileLock, error) {
	return nil, fmt.Errorf("file locking not supported on Windows")
}

// Release is a no-op on Windows.
func (fl *FileLock) Release() error {
	return nil
}
