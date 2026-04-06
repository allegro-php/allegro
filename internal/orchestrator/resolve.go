package orchestrator

import (
	"fmt"
	"os"
	"os/exec"
)

// ComposerResolve runs a Composer command with --no-install --no-scripts --no-interaction.
// This delegates dependency resolution to Composer without downloading packages.
func ComposerResolve(composerPath, projectDir string, args []string) error {
	fullArgs := append(args, "--no-install", "--no-scripts", "--no-interaction")
	cmd := exec.Command(composerPath, fullArgs...)
	cmd.Dir = projectDir
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd.Run()
}

// ComposerUpdate runs "composer update [packages...] --no-install --no-scripts".
func ComposerUpdate(composerPath, projectDir string, packages []string, noDev bool) error {
	args := []string{"update"}
	args = append(args, packages...)
	if noDev {
		args = append(args, "--no-dev")
	}
	return ComposerResolve(composerPath, projectDir, args)
}

// ComposerRequire runs "composer require <pkg> [constraint] --no-install --no-scripts".
// Note: --no-dev is NOT forwarded to composer require (different semantics per spec §10.3).
func ComposerRequire(composerPath, projectDir, pkg, constraint string) error {
	args := []string{"require", pkg}
	if constraint != "" {
		args = append(args, constraint)
	}
	return ComposerResolve(composerPath, projectDir, args)
}

// ComposerRemove runs "composer remove <pkg> --no-install --no-scripts".
// Note: --no-dev is NOT forwarded to composer remove (different semantics per spec §10.3).
func ComposerRemove(composerPath, projectDir, pkg string) error {
	return ComposerResolve(composerPath, projectDir, []string{"remove", pkg})
}

// ComposerGenerateLock runs "composer update --no-install --no-scripts" to generate lock file.
func ComposerGenerateLock(composerPath, projectDir string) error {
	return ComposerResolve(composerPath, projectDir, []string{"update"})
}

// ComposerRunScript runs "composer run-script <event> --no-interaction".
func ComposerRunScript(composerPath, projectDir, event string) error {
	cmd := exec.Command(composerPath, "run-script", event, "--no-interaction")
	cmd.Dir = projectDir
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("composer script %s failed: %w", event, err)
	}
	return nil
}
