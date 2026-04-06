package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/allegro-php/allegro/internal/autoloader"
	"github.com/allegro-php/allegro/internal/orchestrator"
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
	if len(args) > 1 {
		constraint = args[1]
	}

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
		NoAutoload: flagNoAutoload, Verbose: IsVerbose(), Quiet: IsQuiet(), Version: versionStr,
	}
	orch := orchestrator.New(cfg)
	return orch.Install(context.Background())
}
