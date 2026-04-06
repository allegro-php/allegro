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

var removeCmd = &cobra.Command{
	Use:   "remove <package>",
	Short: "Remove a package and update",
	Args:  cobra.ExactArgs(1),
	RunE:  runRemove,
}

func init() {
	rootCmd.AddCommand(removeCmd)
}

func runRemove(cmd *cobra.Command, args []string) error {
	projectDir, _ := os.Getwd()
	composerBin, err := autoloader.FindComposer(projectDir)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "exit 5: %v\n", err)
		os.Exit(ExitComposerError)
		return nil
	}

	// --no-dev NOT forwarded to composer remove (§10.3)
	if err := orchestrator.ComposerRemove(composerBin, projectDir, args[0]); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "remove failed: %v\n", err)
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
