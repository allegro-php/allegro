package autoloader

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// CheckComposerVersion verifies Composer >= 2.0.
func CheckComposerVersion(composerPath string) error {
	out, err := exec.Command(composerPath, "--version", "--no-ansi").Output()
	if err != nil {
		return fmt.Errorf("failed to get composer version: %w", err)
	}
	version := parseComposerVersion(string(out))
	if version == "" {
		return fmt.Errorf("could not parse composer version from: %s", strings.TrimSpace(string(out)))
	}
	major, minor := parseMajorMinor(version)
	if major < 2 {
		return fmt.Errorf("Composer >= 2.0 required, found %s", version)
	}
	_ = minor // reserved for future checks
	return nil
}

var versionRegex = regexp.MustCompile(`Composer\s+version\s+(\d+\.\d+\.\d+)`)

func parseComposerVersion(output string) string {
	matches := versionRegex.FindStringSubmatch(output)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

func parseMajorMinor(version string) (int, int) {
	parts := strings.SplitN(version, ".", 3)
	major, _ := strconv.Atoi(parts[0])
	minor := 0
	if len(parts) > 1 {
		minor, _ = strconv.Atoi(parts[1])
	}
	return major, minor
}
