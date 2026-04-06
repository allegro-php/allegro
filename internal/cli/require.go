package cli

import (
	"fmt"

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
	fmt.Fprintln(cmd.OutOrStdout(), "allegro require: not yet implemented")
	return nil
}
