package linker

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// CleanStaleVendorDirs removes leftover vendor.allegro.old/ and vendor.allegro.tmp/.
func CleanStaleVendorDirs(projectDir string) {
	for _, name := range []string{"vendor.allegro.old", "vendor.allegro.tmp"} {
		path := filepath.Join(projectDir, name)
		if _, err := os.Stat(path); err == nil {
			log.Printf("cleaning stale directory: %s", path)
			os.RemoveAll(path)
		}
	}
}

// CleanStaleStoreTmp removes leftover temp directories in store/tmp/.
// Only removes entries matching the current PID or entries older than 1 hour.
func CleanStaleStoreTmp(storeTmpDir string) {
	entries, err := os.ReadDir(storeTmpDir)
	if err != nil {
		return
	}

	currentPID := fmt.Sprintf("tmp-%d-", os.Getpid())
	cutoff := time.Now().Add(-1 * time.Hour)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		path := filepath.Join(storeTmpDir, name)

		// Remove if it belongs to current PID
		if strings.HasPrefix(name, currentPID) {
			log.Printf("cleaning stale tmp (own PID): %s", path)
			os.RemoveAll(path)
			continue
		}

		// Remove if older than 1 hour
		if strings.HasPrefix(name, "tmp-") {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoff) {
				log.Printf("cleaning stale tmp (>1h old): %s", path)
				os.RemoveAll(path)
			}
		}
	}
}

// CreateTempDir creates a process-unique temp directory in store/tmp/.
func CreateTempDir(storeTmpDir string) (string, error) {
	name := fmt.Sprintf("tmp-%d-%s", os.Getpid(), randomHex(8))
	path := filepath.Join(storeTmpDir, name)
	if err := os.MkdirAll(path, 0755); err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	return path, nil
}

// ParsePIDFromTmpName extracts PID from a tmp-{pid}-{random} directory name.
func ParsePIDFromTmpName(name string) (int, bool) {
	if !strings.HasPrefix(name, "tmp-") {
		return 0, false
	}
	parts := strings.SplitN(name[4:], "-", 2)
	if len(parts) < 1 {
		return 0, false
	}
	pid, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, false
	}
	return pid, true
}
