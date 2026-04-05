package main

import (
	"os"

	"github.com/allegro-php/allegro/internal/cli"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	cli.SetVersionInfo(version, commit, buildDate)
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
