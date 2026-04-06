package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/allegro-php/allegro/internal/autoloader"
	"github.com/allegro-php/allegro/internal/linker"
	"github.com/allegro-php/allegro/internal/orchestrator"
	"github.com/allegro-php/allegro/internal/parser"
	"github.com/allegro-php/allegro/internal/store"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install dependencies from composer.lock",
	RunE:  runInstall,
}

func init() {
	rootCmd.AddCommand(installCmd)
}

func runInstall(cmd *cobra.Command, args []string) error {
	projectDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	lockPath := filepath.Join(projectDir, "composer.lock")
	vendorDir := filepath.Join(projectDir, "vendor")
	storePath := store.ResolvePath(flagStorePath, os.Getenv("ALLEGRO_STORE"))

	// --- §10.4/§10.5: Lock file handling ---

	// Frozen-lockfile check (§10.5) — runs first
	if IsFrozenLockfile() {
		if _, err := os.Stat(lockPath); os.IsNotExist(err) {
			fmt.Fprintln(cmd.ErrOrStderr(), "composer.lock not found (--frozen-lockfile is set)")
			os.Exit(ExitProjectFile)
		}
		// Check hash + dev match if state exists
		state, stateErr := linker.ReadVendorState(vendorDir)
		if stateErr == nil {
			currentHash, _ := parser.ComputeLockHash(lockPath)
			if state.LockHash != currentHash {
				fmt.Fprintln(cmd.ErrOrStderr(), "composer.lock out of sync with vendor")
				os.Exit(ExitProjectFile)
			}
			if state.EffectiveDev() != IsDevMode() {
				fmt.Fprintln(cmd.ErrOrStderr(), "vendor dev mode does not match --no-dev/--dev flag")
				os.Exit(ExitProjectFile)
			}
		}
		// State absent = fresh deploy, proceed
	}

	// Auto-resolve when no lock file (§10.4)
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		jsonPath := filepath.Join(projectDir, "composer.json")
		if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
			fmt.Fprintln(cmd.ErrOrStderr(), "neither composer.lock nor composer.json found")
			os.Exit(ExitProjectFile)
		}
		composerBin, findErr := autoloader.FindComposer(projectDir)
		if findErr != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "exit 5: %v\n", findErr)
			os.Exit(ExitComposerError)
		}
		if !IsQuiet() {
			fmt.Fprintln(cmd.OutOrStdout(), "composer.lock not found, resolving dependencies...")
		}
		if err := orchestrator.ComposerGenerateLock(composerBin, projectDir); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "dependency resolution failed: %v\n", err)
			os.Exit(ExitComposerError)
		}
	}

	// composer.json required for autoload (unless --no-autoload)
	if !flagNoAutoload {
		if _, err := os.Stat(filepath.Join(projectDir, "composer.json")); os.IsNotExist(err) {
			fmt.Fprintln(cmd.ErrOrStderr(), "composer.json not found. Required for autoload generation.")
			os.Exit(ExitProjectFile)
		}
	}

	if flagDryRun {
		return runDryRun(cmd, projectDir)
	}

	// --- §3.2: Incremental noop check ---
	if !IsForce() {
		state, stateErr := linker.ReadVendorState(vendorDir)
		if stateErr == nil {
			currentHash, _ := parser.ComputeLockHash(lockPath)
			if orchestrator.IsNoop(state.LockHash, currentHash, state.EffectiveDev(), IsDevMode()) {
				if !IsQuiet() {
					fmt.Fprintln(cmd.OutOrStdout(), "Vendor is up to date")
				}
				// Update projects.json even on noop (§9.1)
				s := store.New(storePath)
				regPath := store.DefaultRegistryPath()
				lock, _ := parser.ParseLockFile(lockPath)
				if lock != nil {
					pkgMap := make(map[string]string)
					for _, p := range parser.MergePackages(lock) {
						pkgMap[p.Name] = p.Version
					}
					store.RegisterProject(regPath, store.ProjectEntry{
						Path: projectDir, LockHash: currentHash, Packages: pkgMap,
					})
				}
				_ = s
				return nil
			}
		}
	}

	// --- Full or incremental install via orchestrator ---
	cfg := orchestrator.Config{
		ProjectDir:   projectDir,
		StorePath:    storePath,
		LinkStrategy: ResolveLinkStrategy(),
		Workers:      ResolveWorkers(),
		NoAutoload:   flagNoAutoload,
		Verbose:      IsVerbose(),
		Quiet:        IsQuiet(),
		Version:      versionStr,
		Dev:          IsDevMode(),
		NoScripts:    IsNoScripts(),
	}

	orch := orchestrator.New(cfg)
	if err := orch.Install(context.Background()); err != nil {
		errMsg := err.Error()
		fmt.Fprintf(cmd.ErrOrStderr(), "install failed: %v\n", err)
		switch {
		case strings.Contains(errMsg, "download") || strings.Contains(errMsg, "HTTP") || strings.Contains(errMsg, "network"):
			os.Exit(ExitNetworkError)
		case strings.Contains(errMsg, "composer dumpautoload") || strings.Contains(errMsg, "exit 5") || strings.Contains(errMsg, "composer binary"):
			os.Exit(ExitComposerError)
		case strings.Contains(errMsg, "permission") || strings.Contains(errMsg, "disk") ||
			strings.Contains(errMsg, "rename") || strings.Contains(errMsg, "create store") ||
			strings.Contains(errMsg, "link") || strings.Contains(errMsg, "create temp dir") ||
			strings.Contains(errMsg, "create lock file") || strings.Contains(errMsg, "shard dir") ||
			strings.Contains(errMsg, "chmod") || strings.Contains(errMsg, "mkdir"):
			os.Exit(ExitFilesystemError)
		default:
			os.Exit(ExitGeneralError)
		}
	}

	// --- Post-flock: Composer scripts (§8.3 step 10) ---
	if !IsNoScripts() {
		composerBin, findErr := autoloader.FindComposer(projectDir)
		if findErr == nil {
			scriptEvent := "post-install-cmd"
			if err := orchestrator.ComposerRunScript(composerBin, projectDir, scriptEvent); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v\n", err)
				// Do NOT fail the install — vendor is already built
			}
		}
	}

	// --- Register project in projects.json (§9.1) ---
	regPath := store.DefaultRegistryPath()
	lock, _ := parser.ParseLockFile(lockPath)
	if lock != nil {
		currentHash, _ := parser.ComputeLockHash(lockPath)
		pkgMap := make(map[string]string)
		for _, p := range parser.MergePackages(lock) {
			pkgMap[p.Name] = p.Version
		}
		store.RegisterProject(regPath, store.ProjectEntry{
			Path: projectDir, LockHash: currentHash, Packages: pkgMap,
		})
	}

	if runtime.GOOS == "windows" {
		fmt.Fprintln(os.Stderr, "warning: vendor/bin/ proxy scripts are Unix-only; Windows .bat proxies will be available in a future version")
	}

	return nil
}

func runDryRun(cmd *cobra.Command, projectDir string) error {
	lockPath := filepath.Join(projectDir, "composer.lock")
	lock, err := parser.ParseLockFile(lockPath)
	if err != nil {
		return err
	}

	storePath := store.ResolvePath(flagStorePath, os.Getenv("ALLEGRO_STORE"))
	s := store.New(storePath)
	s.EnsureDirectories()
	strategy, _ := linker.DetectStrategy(s.Root, projectDir, ResolveLinkStrategy())

	all := parser.MergePackages(lock)

	var newPkgs, cachedPkgs, skippedPkgs int
	for _, pkg := range all {
		if pkg.Dist == nil || pkg.Dist.Type == "path" {
			skippedPkgs++
			continue
		}
		if s.ManifestExists(pkg.Name, pkg.Version) {
			cachedPkgs++
		} else {
			newPkgs++
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Dry run: %d packages (%d new, %d cached, %d skipped), strategy: %s\n",
		len(all), newPkgs, cachedPkgs, skippedPkgs, strategy)
	for _, pkg := range all {
		status := "new"
		if pkg.Dist == nil || pkg.Dist.Type == "path" {
			status = "skip"
		} else if s.ManifestExists(pkg.Name, pkg.Version) {
			status = "cached"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  [%s] %s %s\n", status, pkg.Name, pkg.Version)
	}
	return nil
}
