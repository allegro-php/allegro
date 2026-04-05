package linker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectStrategyForced(t *testing.T) {
	s, l := DetectStrategy("", "", "copy")
	if s != Copy {
		t.Errorf("strategy = %v, want Copy", s)
	}
	if l == nil {
		t.Error("linker should not be nil")
	}
}

func TestDetectStrategyForcedReflink(t *testing.T) {
	s, _ := DetectStrategy("", "", "reflink")
	if s != Reflink {
		t.Errorf("strategy = %v, want Reflink", s)
	}
}

func TestDetectStrategyForcedHardlink(t *testing.T) {
	s, _ := DetectStrategy("", "", "hardlink")
	if s != Hardlink {
		t.Errorf("strategy = %v, want Hardlink", s)
	}
}

func TestDetectStrategyProbe(t *testing.T) {
	storeDir := t.TempDir()
	projectDir := t.TempDir()

	// Create store/tmp
	os.MkdirAll(filepath.Join(storeDir, "tmp"), 0755)

	s, l := DetectStrategy(storeDir, projectDir, "")
	// On macOS APFS, should get Reflink; on Linux ext4, Hardlink; otherwise Copy
	// Just verify it returns something valid
	if s != Reflink && s != Hardlink && s != Copy {
		t.Errorf("unexpected strategy: %v", s)
	}
	if l == nil {
		t.Error("linker should not be nil")
	}
}

func TestDetectStrategyProbeStoreError(t *testing.T) {
	// Non-existent store should fallback to Copy
	s, _ := DetectStrategy("/nonexistent/store", t.TempDir(), "")
	if s != Copy {
		t.Errorf("strategy = %v, want Copy on store error", s)
	}
}

func TestCopyLinker(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	os.WriteFile(src, []byte("content"), 0644)

	c := &CopyLinker{}
	if err := c.LinkFile(src, dst); err != nil {
		t.Fatalf("CopyLinker.LinkFile: %v", err)
	}

	data, _ := os.ReadFile(dst)
	if string(data) != "content" {
		t.Errorf("content = %q, want content", data)
	}
}

func TestHardLinker(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	os.WriteFile(src, []byte("content"), 0644)

	h := &HardLinker{}
	if err := h.LinkFile(src, dst); err != nil {
		t.Fatalf("HardLinker.LinkFile: %v", err)
	}

	data, _ := os.ReadFile(dst)
	if string(data) != "content" {
		t.Errorf("content = %q, want content", data)
	}

	// Verify same inode
	srcInfo, _ := os.Stat(src)
	dstInfo, _ := os.Stat(dst)
	if !os.SameFile(srcInfo, dstInfo) {
		t.Error("hardlink should share inode")
	}
}

func TestRandomHex(t *testing.T) {
	h := randomHex(8)
	if len(h) != 16 {
		t.Errorf("randomHex(8) length = %d, want 16", len(h))
	}
}
