package linker

import (
	"fmt"
	"io"
	"os"
)

// CopyLinker links files by copying content.
type CopyLinker struct{}

func (c *CopyLinker) LinkFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("copy open src: %w", err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("copy create dst: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy content: %w", err)
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("copy sync: %w", err)
	}
	return nil
}
