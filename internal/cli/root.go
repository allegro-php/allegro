package cli

import "github.com/spf13/cobra"

var (
	versionStr string
	commitStr  string
	buildStr   string
)

func SetVersionInfo(version, commit, buildDate string) {
	versionStr = version
	commitStr = commit
	buildStr = buildDate
}

var rootCmd = &cobra.Command{
	Use:   "allegro",
	Short: "pnpm-inspired package linker for PHP",
}

func Execute() error {
	return rootCmd.Execute()
}
