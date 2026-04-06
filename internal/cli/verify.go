package cli

import (
	"fmt"

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
	fmt.Fprintln(cmd.OutOrStdout(), "allegro verify: not yet implemented")
	return nil
}
