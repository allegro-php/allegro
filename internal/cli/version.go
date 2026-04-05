package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "allegro %s (commit %s, built %s)\n", versionStr, commitStr, buildStr)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
