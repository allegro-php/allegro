package cli

import (
	"fmt"
	"os"
	"strconv"
)

var (
	flagStorePath    string
	flagNoAutoload   bool
	flagLinkStrategy string
	flagWorkers      int
	flagVerbose      bool
	flagQuiet        bool
	flagNoProgress   bool
	flagDryRun       bool
)

func init() {
	pf := rootCmd.PersistentFlags()
	pf.StringVar(&flagStorePath, "store-path", "", "Override store directory")
	pf.BoolVar(&flagNoAutoload, "no-autoload", false, "Skip composer dumpautoload")
	pf.StringVar(&flagLinkStrategy, "link-strategy", "", "Force link strategy: reflink, hardlink, copy")
	pf.IntVar(&flagWorkers, "workers", 0, "Parallel download workers (1-32)")
	pf.BoolVarP(&flagVerbose, "verbose", "v", false, "Verbose output")
	pf.BoolVarP(&flagQuiet, "quiet", "q", false, "Suppress non-error output")
	pf.BoolVar(&flagNoProgress, "no-progress", false, "Disable progress bars")
	pf.BoolVar(&flagDryRun, "dry-run", false, "Show what would be installed")
}

// ResolveWorkers returns the effective worker count with clamping.
func ResolveWorkers() int {
	w := flagWorkers
	if w == 0 {
		if envVal := os.Getenv("ALLEGRO_WORKERS"); envVal != "" {
			if v, err := strconv.Atoi(envVal); err == nil {
				w = v
			}
		}
	}
	if w == 0 {
		return 8 // default
	}
	if w < 1 {
		fmt.Fprintf(os.Stderr, "warning: --workers %d out of range [1,32], clamped to 1\n", w)
		return 1
	}
	if w > 32 {
		fmt.Fprintf(os.Stderr, "warning: --workers %d out of range [1,32], clamped to 32\n", w)
		return 32
	}
	return w
}

// IsVerbose returns true if verbose output should be shown.
func IsVerbose() bool {
	if flagQuiet {
		return false // quiet takes precedence
	}
	if flagVerbose {
		return true
	}
	if os.Getenv("ALLEGRO_QUIET") != "" {
		return false
	}
	if os.Getenv("ALLEGRO_VERBOSE") != "" {
		return true
	}
	return false
}

// IsQuiet returns true if non-error output should be suppressed.
func IsQuiet() bool {
	if flagQuiet {
		return true
	}
	if os.Getenv("ALLEGRO_QUIET") != "" {
		return true
	}
	return false
}

// ShouldShowProgress returns true if progress bars should be shown.
func ShouldShowProgress() bool {
	if flagNoProgress {
		return false
	}
	if os.Getenv("ALLEGRO_NO_PROGRESS") != "" {
		return false
	}
	return !IsQuiet()
}

// ResolveLinkStrategy returns the forced link strategy or empty string.
func ResolveLinkStrategy() string {
	if flagLinkStrategy != "" {
		return flagLinkStrategy
	}
	return os.Getenv("ALLEGRO_LINK_STRATEGY")
}
