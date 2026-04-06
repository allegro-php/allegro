package cli

import (
	"fmt"

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
	fmt.Fprintln(cmd.OutOrStdout(), "allegro update: not yet implemented")
	return nil
}
