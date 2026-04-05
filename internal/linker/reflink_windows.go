//go:build windows

package linker

import "fmt"

// ReflinkLinker is a stub on Windows — reflink is not supported.
type ReflinkLinker struct{}

func (r *ReflinkLinker) LinkFile(src, dst string) error {
	return fmt.Errorf("reflink not supported on Windows")
}

// TryReflink always returns false on Windows.
func TryReflink(src, dst string) bool {
	return false
}
