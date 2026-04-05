//go:build darwin

package linker

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// ReflinkLinker links files via clonefile on macOS (APFS).
type ReflinkLinker struct{}

// clonefile syscall number on macOS
const sysClonefile = 462

func (r *ReflinkLinker) LinkFile(src, dst string) error {
	srcBytes, err := syscall.BytePtrFromString(src)
	if err != nil {
		return err
	}
	dstBytes, err := syscall.BytePtrFromString(dst)
	if err != nil {
		return err
	}

	// clonefile(src, dst, 0)
	_, _, errno := syscall.Syscall(
		uintptr(sysClonefile),
		uintptr(unsafe.Pointer(srcBytes)),
		uintptr(unsafe.Pointer(dstBytes)),
		0,
	)
	if errno != 0 {
		return fmt.Errorf("clonefile: %w", errno)
	}
	return nil
}

// TryReflink attempts a reflink and returns whether it succeeded.
func TryReflink(src, dst string) bool {
	r := &ReflinkLinker{}
	err := r.LinkFile(src, dst)
	if err != nil {
		os.Remove(dst) // clean up partial
	}
	return err == nil
}
