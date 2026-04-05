package store

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

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
		// Skip symlinks
		if f.Mode()&os.ModeSymlink != 0 {
			continue
		}
		// Skip special files
		if !f.Mode().IsRegular() {
			continue
		}

		path := filepath.Join(destDir, f.Name)
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
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}

		// Preserve executable bit
		if f.Mode()&0111 != 0 {
			os.Chmod(path, 0755)
		}
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

		// Skip directories, symlinks, and special files
		switch hdr.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			// Regular file — process
		case tar.TypeLink:
			// Hard link in tar — extract as independent copy
			// The content will be read from the tar stream
		default:
			continue // Skip symlinks, dirs, devices, etc.
		}

		path := filepath.Join(destDir, hdr.Name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}

		out, err := os.Create(path)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, tr)
		out.Close()
		if err != nil {
			return err
		}

		if hdr.Mode&0111 != 0 {
			os.Chmod(path, 0755)
		}
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
		return fmt.Errorf("empty archive (no files after extraction)")
	}

	// Check if there's exactly one top-level entry and it's a directory
	if len(entries) != 1 || !entries[0].IsDir() {
		return nil // Multiple entries or single file — don't strip
	}

	topDir := filepath.Join(dir, entries[0].Name())
	subEntries, err := os.ReadDir(topDir)
	if err != nil {
		return err
	}

	// Move all contents up
	for _, entry := range subEntries {
		src := filepath.Join(topDir, entry.Name())
		dst := filepath.Join(dir, entry.Name())
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("strip top-level: rename %s: %w", entry.Name(), err)
		}
	}

	// Remove the now-empty top-level directory
	return os.Remove(topDir)
}

// ExtractByType extracts an archive based on dist.type.
func ExtractByType(data []byte, distType, destDir string) error {
	switch strings.ToLower(distType) {
	case "zip":
		return ExtractZip(data, destDir)
	case "tar":
		return ExtractTar(bytes.NewReader(data), destDir)
	case "gzip":
		return ExtractGzip(data, destDir)
	case "xz":
		return fmt.Errorf("xz extraction not yet implemented")
	default:
		return fmt.Errorf("unsupported dist type: %s", distType)
	}
}
