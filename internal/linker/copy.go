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

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst) // clean up partial file
		return fmt.Errorf("copy content: %w", err)
	}
	if err := out.Close(); err != nil {
		os.Remove(dst)
		return fmt.Errorf("copy close: %w", err)
	}
	return nil
}
