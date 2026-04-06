package linker

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// DetectStrategy probes the filesystem to determine the best link strategy.
// If forcedStrategy is non-empty, returns it directly (skipping probe).
func DetectStrategy(storeDir, projectDir, forcedStrategy string) (Strategy, Linker) {
	if forcedStrategy != "" {
		switch forcedStrategy {
		case "reflink":
			return Reflink, &ReflinkLinker{}
		case "hardlink":
			return Hardlink, &HardLinker{}
		default:
			return Copy, &CopyLinker{}
		}
	}

	// Create temp file in store/tmp
	storeTmp := filepath.Join(storeDir, "tmp")
	probeFile := filepath.Join(storeTmp, "probe-"+randomHex(8))
	if err := os.WriteFile(probeFile, []byte("probe"), 0644); err != nil {
		log.Printf("warning: cannot create probe file in store, falling back to copy: %v", err)
		return Copy, &CopyLinker{}
	}

	// Create temp dir in project root
	probeName := ".allegro-probe-" + randomHex(8)
	probeDir := filepath.Join(projectDir, probeName)
	if err := os.MkdirAll(probeDir, 0755); err != nil {
		log.Printf("warning: cannot create probe dir in project root, falling back to copy: %v", err)
		os.Remove(probeFile)
		return Copy, &CopyLinker{}
	}

	probeDst := filepath.Join(probeDir, "probe")

	// Try reflink
	if TryReflink(probeFile, probeDst) {
		cleanup(probeFile, probeDir)
		return Reflink, &ReflinkLinker{}
	}
	os.Remove(probeDst) // clean failed attempt

	// Try hardlink
	if err := os.Link(probeFile, probeDst); err == nil {
		cleanup(probeFile, probeDir)
		return Hardlink, &HardLinker{}
	}

	// Fallback to copy
	cleanup(probeFile, probeDir)
	return Copy, &CopyLinker{}
}

func cleanup(probeFile, probeDir string) {
	if err := os.Remove(probeFile); err != nil {
		log.Printf("warning: probe cleanup failed for %s: %v", probeFile, err)
	}
	if err := os.RemoveAll(probeDir); err != nil {
		log.Printf("warning: probe cleanup failed for %s: %v", probeDir, err)
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand unavailable: %v", err))
	}
	return hex.EncodeToString(b)
}

// FormatStrategy converts a string to Strategy enum.
func FormatStrategy(s string) Strategy {
	switch s {
	case "reflink":
		return Reflink
	case "hardlink":
		return Hardlink
	default:
		return Copy
	}
}
