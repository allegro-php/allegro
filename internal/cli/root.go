package cli

import (
	"runtime/debug"

	"github.com/spf13/cobra"
)

var (
	versionStr string
	commitStr  string
	buildStr   string
)

func SetVersionInfo(version, commit, buildDate string) {
	versionStr = version
	commitStr = commit
	buildStr = buildDate

	// Fallback for `go install`: ldflags aren't set, but Go embeds
	// the module version automatically via debug.ReadBuildInfo.
	if versionStr == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			versionStr = info.Main.Version
		}
	}
}

var rootCmd = &cobra.Command{
	Use:   "allegro",
	Short: "pnpm-inspired package linker for PHP",
}

func Execute() error {
	return rootCmd.Execute()
}
