package store

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)
// ErrEmptyArchive is returned when an archive contains no files.
var ErrEmptyArchive = errors.New("empty archive (no files after extraction)")

// maxExtractedEntry limits per-file extraction size to prevent zip/tar bombs.
const maxExtractedEntry = 512 << 20 // 512 MiB per entry
// isInsideDir checks that the resolved path is inside destDir (prevents path traversal).
func isInsideDir(path, destDir string) bool {
	cleanPath := filepath.Clean(path)
	cleanDest := filepath.Clean(destDir) + string(os.PathSeparator)
	return strings.HasPrefix(cleanPath+string(os.PathSeparator), cleanDest)
}

// ExtractZip extracts a zip archive to destDir.
func ExtractZip(data []byte, destDir string) error {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// Skip symlinks with warning
		if f.Mode()&os.ModeSymlink != 0 {
			log.Printf("warning: skipping symlink %s", f.Name)
			continue
		}
		// Skip special files with warning
		if !f.Mode().IsRegular() {
			log.Printf("warning: skipping special file %s (mode %v)", f.Name, f.Mode())
			continue
		}

		path := filepath.Join(destDir, f.Name)
		// Path traversal check
		if !isInsideDir(path, destDir) {
			log.Printf("warning: skipping path-traversal entry %s", f.Name)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		out, err := os.Create(path)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, io.LimitReader(rc, maxExtractedEntry))
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}

		// Don't trust ZIP executable metadata — it's unreliable for
		// GitHub-generated archives. Executable detection is done later
		// in storeExtractedFiles via shebang inspection.
	}
	return nil
}

// ExtractTar extracts a tar archive from a reader to destDir.
func ExtractTar(r io.Reader, destDir string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}

		switch hdr.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			// Regular file — process
		case tar.TypeLink:
			// Hard link in tar — extract as independent copy
		default:
			log.Printf("warning: skipping %s (type %c)", hdr.Name, hdr.Typeflag)
			continue
		}

		path := filepath.Join(destDir, hdr.Name)
		// Path traversal check
		if !isInsideDir(path, destDir) {
			log.Printf("warning: skipping path-traversal entry %s", hdr.Name)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}

		out, err := os.Create(path)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, io.LimitReader(tr, maxExtractedEntry))
		out.Close()
		if err != nil {
			return err
		}

		// Don't trust tar executable metadata — executable detection
		// is done later in storeExtractedFiles via shebang inspection.
	}
	return nil
}

// ExtractXz decompresses xz then extracts tar using system xz command.
func ExtractXz(data []byte, destDir string) error {
	cmd := exec.Command("xz", "--decompress", "--stdout")
	cmd.Stdin = bytes.NewReader(data)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("xz pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("xz start: %w", err)
	}
	if err := ExtractTar(stdout, destDir); err != nil {
		cmd.Wait()
		return fmt.Errorf("xz+tar extract: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("xz decompress: %w", err)
	}
	return nil
}

// ExtractGzip decompresses gzip then extracts tar.
func ExtractGzip(data []byte, destDir string) error {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("open gzip: %w", err)
	}
	defer gr.Close()
	return ExtractTar(gr, destDir)
}

// StripTopLevelDir detects if all files share a common top-level directory
// and moves them up if so.
func StripTopLevelDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		return ErrEmptyArchive
	}

	if len(entries) != 1 || !entries[0].IsDir() {
		return nil // Multiple entries or single file — don't strip
	}

	topDir := filepath.Join(dir, entries[0].Name())
	subEntries, err := os.ReadDir(topDir)
	if err != nil {
		return err
	}

	for _, entry := range subEntries {
		src := filepath.Join(topDir, entry.Name())
		dst := filepath.Join(dir, entry.Name())
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("strip top-level: rename %s: %w", entry.Name(), err)
		}
	}

	return os.Remove(topDir)
}

// ExtractByType extracts an archive based on dist.type.
func ExtractByType(data []byte, distType, destDir, packageName string) error {
	switch strings.ToLower(distType) {
	case "zip":
		return ExtractZip(data, destDir)
	case "tar":
		return ExtractTar(bytes.NewReader(data), destDir)
	case "gzip":
		return ExtractGzip(data, destDir)
	case "xz":
		return ExtractXz(data, destDir)
	default:
		return fmt.Errorf("unsupported dist type: %s for package %s", distType, packageName)
	}
}
