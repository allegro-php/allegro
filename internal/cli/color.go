package cli

import (
	"fmt"
	"os"

	"github.com/allegro-php/allegro/internal/orchestrator"
	"github.com/fatih/color"
	"golang.org/x/term"
)

var (
	greenPlus   = color.New(color.FgGreen).SprintFunc()
	yellowArrow = color.New(color.FgYellow).SprintFunc()
	redMinus    = color.New(color.FgRed).SprintFunc()
)

// IsTTY returns true if stdout is a terminal.
func IsTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// PrintDiff prints a colored package diff to stdout.
func PrintDiff(diff orchestrator.PackageDiff) {
	useColor := IsColorEnabled() && IsTTY()

	for _, pkg := range diff.Added {
		if useColor {
			fmt.Printf("    %s %s %s (new)\n", greenPlus("+"), pkg.Name, pkg.Version)
		} else {
			fmt.Printf("    + %s %s (new)\n", pkg.Name, pkg.Version)
		}
	}
	for _, u := range diff.Updated {
		if useColor {
			fmt.Printf("    %s %s %s → %s (updated)\n", yellowArrow("↑"), u.Name, u.OldVersion, u.NewVersion)
		} else {
			fmt.Printf("    ↑ %s %s → %s (updated)\n", u.Name, u.OldVersion, u.NewVersion)
		}
	}
	for _, pkg := range diff.Removed {
		if useColor {
			fmt.Printf("    %s %s %s (removed)\n", redMinus("-"), pkg.Name, pkg.Version)
		} else {
			fmt.Printf("    - %s %s (removed)\n", pkg.Name, pkg.Version)
		}
	}
}
