package autoloader

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// FindComposer locates the composer binary by precedence:
// 1. ALLEGRO_COMPOSER_PATH env var
// 2. "composer" in PATH
// 3. "composer.phar" in projectDir
func FindComposer(projectDir string) (string, error) {
	if envPath := os.Getenv("ALLEGRO_COMPOSER_PATH"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return envPath, nil
		}
	}

	if path, err := exec.LookPath("composer"); err == nil {
		return path, nil
	}

	pharPath := filepath.Join(projectDir, "composer.phar")
	if _, err := os.Stat(pharPath); err == nil {
		return pharPath, nil
	}

	return "", fmt.Errorf("composer binary not found. Tried:\n  1. ALLEGRO_COMPOSER_PATH env var (not set or file missing)\n  2. 'composer' in PATH (not found)\n  3. '%s' (not found)\n\nInstall Composer: https://getcomposer.org/download/", pharPath)
}

// RunDumpautoload runs "composer dumpautoload --optimize" in the given directory.
// stderr is forwarded, stdout is captured.
func RunDumpautoload(composerPath, projectDir string, verbose, noDev bool) error {
	args := []string{"dumpautoload", "--optimize"}
	if noDev {
		args = append(args, "--no-dev")
	}
	cmd := exec.Command(composerPath, args...)
	cmd.Dir = projectDir
	cmd.Stderr = os.Stderr
	if verbose {
		cmd.Stdout = os.Stdout
	}
	return cmd.Run()
}
