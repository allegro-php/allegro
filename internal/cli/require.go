package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/allegro-php/allegro/internal/autoloader"
	"github.com/allegro-php/allegro/internal/orchestrator"
	"github.com/allegro-php/allegro/internal/parser"
	"github.com/allegro-php/allegro/internal/store"
	"github.com/spf13/cobra"
)

var requireCmd = &cobra.Command{
	Use:   "require <package> [constraint]",
	Short: "Add a package and install",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runRequire,
}

func init() {
	rootCmd.AddCommand(requireCmd)
}

func runRequire(cmd *cobra.Command, args []string) error {
	projectDir, _ := os.Getwd()
	composerBin, err := autoloader.FindComposer(projectDir)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "exit 5: %v\n", err)
		os.Exit(ExitComposerError)
		return nil
	}

	pkg := args[0]
	constraint := ""
	if len(args) > 1 { constraint = args[1] }

	// --no-dev NOT forwarded to composer require (§10.3)
	if err := orchestrator.ComposerRequire(composerBin, projectDir, pkg, constraint); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "require failed: %v\n", err)
		os.Exit(ExitComposerError)
		return nil
	}

	storePath := store.ResolvePath(flagStorePath, os.Getenv("ALLEGRO_STORE"))
	cfg := orchestrator.Config{
		ProjectDir: projectDir, StorePath: storePath,
		LinkStrategy: ResolveLinkStrategy(), Workers: ResolveWorkers(),
		NoAutoload: flagNoAutoload, Verbose: IsVerbose(), Quiet: IsQuiet(),
		Version: versionStr, Dev: IsDevMode(), NoScripts: IsNoScripts(),
	}
	orch := orchestrator.New(cfg)
	if err := orch.Install(context.Background()); err != nil { return err }

	if !IsNoScripts() {
		if err := orchestrator.ComposerRunScript(composerBin, projectDir, "post-update-cmd"); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v\n", err)
		}
	}

	lockPath := filepath.Join(projectDir, "composer.lock")
	regPath := store.DefaultRegistryPath()
	lock, _ := parser.ParseLockFile(lockPath)
	if lock != nil {
		h, _ := parser.ComputeLockHash(lockPath)
		pkgMap := make(map[string]string)
		for _, p := range parser.MergePackages(lock) { pkgMap[p.Name] = p.Version }
		store.RegisterProject(regPath, store.ProjectEntry{Path: projectDir, LockHash: h, Packages: pkgMap})
	}
	return nil
}
