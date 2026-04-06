package orchestrator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/allegro-php/allegro/internal/linker"
	"github.com/allegro-php/allegro/internal/store"
)

func TestLinkOpFields(t *testing.T) {
	op := LinkOp{Src: "/store/ab/abc", Dst: "/vendor/pkg/file.php", Executable: true}
	if op.Src != "/store/ab/abc" || op.Dst != "/vendor/pkg/file.php" || !op.Executable {
		t.Error("LinkOp fields")
	}
}

func TestCollectDirectories(t *testing.T) {
	ops := []LinkOp{
		{Dst: "/v/a/b/file1.php"},
		{Dst: "/v/a/b/file2.php"},
		{Dst: "/v/c/d/file3.php"},
	}
	dirs := CollectDirectories(ops)
	if len(dirs) != 2 {
		t.Errorf("dirs = %v, want 2 unique", dirs)
	}
}

func TestCollectLinkOps(t *testing.T) {
	dir := t.TempDir()
	s := store.New(filepath.Join(dir, "store"))
	s.EnsureDirectories()

	m := &store.Manifest{
		Name:    "a/b",
		Version: "1.0",
		Files: []store.FileEntry{
			{Path: "src/A.php", Hash: "sha256:abc123", Size: 10, Executable: false},
			{Path: "bin/run", Hash: "sha256:def456", Size: 5, Executable: true},
		},
	}
	ops := CollectLinkOps(s, "a/b", m, "/vendor")
	if len(ops) != 2 {
		t.Fatalf("ops = %d, want 2", len(ops))
	}
	if ops[0].Dst != "/vendor/a/b/src/A.php" {
		t.Errorf("dst = %q", ops[0].Dst)
	}
	if ops[1].Executable != true {
		t.Error("executable flag lost")
	}
}

func TestParallelLinkCopy(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	dstDir := filepath.Join(dir, "dst")
	os.MkdirAll(srcDir, 0755)

	// Create source files
	os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("aaa"), 0644)
	os.WriteFile(filepath.Join(srcDir, "b.txt"), []byte("bbb"), 0644)

	ops := []LinkOp{
		{Src: filepath.Join(srcDir, "a.txt"), Dst: filepath.Join(dstDir, "sub", "a.txt"), Executable: false},
		{Src: filepath.Join(srcDir, "b.txt"), Dst: filepath.Join(dstDir, "sub", "b.txt"), Executable: true},
	}

	err := ParallelLink(ops, &linker.CopyLinker{}, linker.Copy, 2)
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dstDir, "sub", "a.txt"))
	if string(data) != "aaa" {
		t.Errorf("content = %q", data)
	}

	info, _ := os.Stat(filepath.Join(dstDir, "sub", "b.txt"))
	if info.Mode().Perm() != 0755 {
		t.Errorf("perm = %o, want 0755", info.Mode().Perm())
	}
}
