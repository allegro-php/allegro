//go:build linux

package linker

import (
	"fmt"
	"os"
	"syscall"
)

// ReflinkLinker links files via FICLONE ioctl on Linux (Btrfs/XFS).
type ReflinkLinker struct{}

const ficlone = 0x40049409 // FICLONE ioctl number

func (r *ReflinkLinker) LinkFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("reflink open src: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("reflink create dst: %w", err)
	}
	defer dstFile.Close()

	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, dstFile.Fd(), ficlone, srcFile.Fd())
	if errno != 0 {
		os.Remove(dst)
		return fmt.Errorf("FICLONE ioctl: %w", errno)
	}
	return nil
}

// TryReflink attempts a reflink and returns whether it succeeded.
func TryReflink(src, dst string) bool {
	r := &ReflinkLinker{}
	return r.LinkFile(src, dst) == nil
}
