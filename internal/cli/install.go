package cli

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

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

	// Check required files
	if _, err := os.Stat("composer.lock"); os.IsNotExist(err) {
		fmt.Fprintln(cmd.ErrOrStderr(), "composer.lock not found. Run `composer install` first.")
		os.Exit(ExitProjectFile)
	}

	if !flagNoAutoload {
		if _, err := os.Stat("composer.json"); os.IsNotExist(err) {
			fmt.Fprintln(cmd.ErrOrStderr(), "composer.json not found. Required for autoload generation.")
			os.Exit(ExitProjectFile)
		}
	}

	if flagDryRun {
		return runDryRun(cmd, projectDir)
	}

	storePath := store.ResolvePath(flagStorePath, os.Getenv("ALLEGRO_STORE"))

	cfg := orchestrator.Config{
		ProjectDir:   projectDir,
		StorePath:    storePath,
		LinkStrategy: ResolveLinkStrategy(),
		Workers:      ResolveWorkers(),
		NoAutoload:   flagNoAutoload,
		Verbose:      IsVerbose(),
		Quiet:        IsQuiet(),
		Version:      versionStr,
	}

	orch := orchestrator.New(cfg)
	if err := orch.Install(context.Background()); err != nil {
		errMsg := err.Error()
		fmt.Fprintf(cmd.ErrOrStderr(), "install failed: %v\n", err)
		// Map errors to exit codes per spec §7.3
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

	// Windows bin proxy warning
	if runtime.GOOS == "windows" {
		fmt.Fprintln(os.Stderr, "warning: vendor/bin/ proxy scripts are Unix-only; Windows .bat proxies will be available in a future version")
	}

	return nil
}

func runDryRun(cmd *cobra.Command, projectDir string) error {
	// Step 1: Locate and parse
	lockPath := projectDir + "/composer.lock"
	lock, err := parser.ParseLockFile(lockPath)
	if err != nil {
		return err
	}

	// Step 2: Detect link strategy
	storePath := store.ResolvePath(flagStorePath, os.Getenv("ALLEGRO_STORE"))
	s := store.New(storePath)
	s.EnsureDirectories()
	strategy, _ := linker.DetectStrategy(s.Root, projectDir, ResolveLinkStrategy())

	// Step 3: Parse packages
	all := parser.MergePackages(lock)

	// Step 4: Build install plan (check CAS for new vs cached)
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
