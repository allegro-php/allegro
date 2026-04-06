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

var updateCmd = &cobra.Command{
	Use:   "update [packages...]",
	Short: "Re-resolve dependencies and update lock file",
	RunE:  runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	projectDir, _ := os.Getwd()

	composerBin, err := autoloader.FindComposer(projectDir)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "exit 5: %v\n", err)
		os.Exit(ExitComposerError)
		return nil
	}

	// Delegate resolution to Composer (--no-dev only forwarded to update per §10.3)
	if err := orchestrator.ComposerUpdate(composerBin, projectDir, args, !IsDevMode()); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "update failed: %v\n", err)
		os.Exit(ExitComposerError)
		return nil
	}

	// Now install from the updated lock
	storePath := store.ResolvePath(flagStorePath, os.Getenv("ALLEGRO_STORE"))
	cfg := orchestrator.Config{
		ProjectDir: projectDir, StorePath: storePath,
		LinkStrategy: ResolveLinkStrategy(), Workers: ResolveWorkers(),
		NoAutoload: flagNoAutoload, Verbose: IsVerbose(), Quiet: IsQuiet(), Version: versionStr,
	}
	orch := orchestrator.New(cfg)
	return orch.Install(context.Background())
}
