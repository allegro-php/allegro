package cli

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

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
		case strings.Contains(errMsg, "permission") || strings.Contains(errMsg, "disk") || strings.Contains(errMsg, "rename") || strings.Contains(errMsg, "create store") || strings.Contains(errMsg, "link"):
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
	lockPath := projectDir + "/composer.lock"
	lock, err := parser.ParseLockFile(lockPath)
	if err != nil {
		return err
	}

	all := parser.MergePackages(lock)
	fmt.Fprintf(cmd.OutOrStdout(), "Dry run: %d packages would be installed\n", len(all))
	for _, pkg := range all {
		cached := ""
		// Could check CAS here but keeping simple for dry-run
		fmt.Fprintf(cmd.OutOrStdout(), "  %s %s%s\n", pkg.Name, pkg.Version, cached)
	}
	return nil
}
