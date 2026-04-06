package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/allegro-php/allegro/internal/linker"
	"github.com/allegro-php/allegro/internal/orchestrator"
	"github.com/allegro-php/allegro/internal/store"
	"github.com/spf13/cobra"
)

var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify vendor directory integrity",
	RunE:  runVerify,
}

var flagFix bool

func init() {
	verifyCmd.Flags().BoolVar(&flagFix, "fix", false, "Repair issues found")
	rootCmd.AddCommand(verifyCmd)
}

func runVerify(cmd *cobra.Command, args []string) error {
	projectDir, _ := os.Getwd()
	vendorDir := filepath.Join(projectDir, "vendor")
	storePath := store.ResolvePath(flagStorePath, os.Getenv("ALLEGRO_STORE"))
	s := store.New(storePath)

	state, err := linker.ReadVendorState(vendorDir)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "cannot read vendor state: %v\n", err)
		os.Exit(ExitGeneralError)
		return nil
	}

	result, err := orchestrator.VerifyVendor(vendorDir, s, state, ResolveWorkers())
	if err != nil {
		return err
	}

	// Print results
	fmt.Fprintf(cmd.OutOrStdout(), "Checked %d packages (%d files)\n\n", result.TotalPackages, result.TotalFiles)

	if len(result.Issues) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Summary: %d OK, 0 failed\n", result.OKPackages)
		return nil
	}

	for _, issue := range result.Issues {
		fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s/%s — %s\n", issue.Type, issue.Package, issue.File, issue.Detail)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\nSummary: %d OK, %d failed\n", result.OKPackages, result.FailPackages)

	if !flagFix {
		fmt.Fprintln(cmd.OutOrStdout(), "Run `allegro verify --fix` to repair")
		os.Exit(ExitGeneralError)
	}

	// TODO: implement --fix mode (re-link from CAS, re-download if missing)
	fmt.Fprintln(cmd.OutOrStdout(), "Fix mode: not yet fully implemented")
	return nil
}
