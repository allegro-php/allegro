package linker

import (
	"fmt"
	"os"
)

// HardLinker links files via os.Link (hardlink).
type HardLinker struct{}

func (h *HardLinker) LinkFile(src, dst string) error {
	if err := os.Link(src, dst); err != nil {
		return fmt.Errorf("hardlink %s -> %s: %w", src, dst, err)
	}
	return nil
}
