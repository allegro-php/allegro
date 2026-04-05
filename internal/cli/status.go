package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/allegro-php/allegro/internal/linker"
	"github.com/allegro-php/allegro/internal/parser"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show vendor state",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	projectDir, _ := os.Getwd()

	// Check composer.lock
	lockPath := filepath.Join(projectDir, "composer.lock")
	if _, err := os.Stat(lockPath); err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(cmd.ErrOrStderr(), "composer.lock not found")
			os.Exit(ExitProjectFile)
		}
		if os.IsPermission(err) {
			fmt.Fprintln(cmd.ErrOrStderr(), "composer.lock: permission denied")
			os.Exit(ExitProjectFile)
		}
		return err
	}

	vendorDir := filepath.Join(projectDir, "vendor")

	// Check vendor/
	if _, err := os.Stat(vendorDir); os.IsNotExist(err) {
		fmt.Fprintln(cmd.OutOrStdout(), "No vendor directory found — run `allegro install`")
		return nil
	}

	// Check state file
	state, err := linker.ReadVendorState(vendorDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(cmd.OutOrStdout(), "Vendor exists but was not installed by Allegro")
			return nil
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Vendor state file is corrupt — run `allegro install` to rebuild")
		return nil
	}

	// Compare lock hash
	lockHash, err := parser.ComputeLockHash(lockPath)
	if err != nil {
		return err
	}

	if state.LockHash != lockHash {
		fmt.Fprintln(cmd.OutOrStdout(), "Vendor is outdated — run `allegro install` to update")
		return nil
	}

	pkgCount := len(state.Packages)
	date := state.InstalledAt.UTC().Format("2006-01-02")
	fmt.Fprintf(cmd.OutOrStdout(), "Vendor is up to date (%d packages, %s, installed %s)\n",
		pkgCount, state.LinkStrategy, date)
	return nil
}
