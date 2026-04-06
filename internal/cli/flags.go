package cli

import (
	"fmt"
	"os"
	"strconv"

	cfg "github.com/allegro-php/allegro/internal/config"
)

var (
	flagStorePath      string
	flagNoAutoload     bool
	flagLinkStrategy   string
	flagWorkers        int
	flagVerbose        bool
	flagQuiet          bool
	flagNoProgress     bool
	flagDryRun         bool
	// Phase 2 flags
	flagForce          bool
	flagNoDev          bool
	flagDev            bool
	flagNoScripts      bool
	flagNoColor        bool
	flagFrozenLockfile bool
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
	// Phase 2
	pf.BoolVar(&flagForce, "force", false, "Full rebuild, skip incremental")
	pf.BoolVar(&flagNoDev, "no-dev", false, "Exclude dev dependencies")
	pf.BoolVar(&flagDev, "dev", false, "Explicitly install dev dependencies (overrides --no-dev)")
	pf.BoolVar(&flagNoScripts, "no-scripts", false, "Skip Composer script execution")
	pf.BoolVar(&flagNoColor, "no-color", false, "Disable colored output")
	pf.BoolVar(&flagFrozenLockfile, "frozen-lockfile", false, "Error if lock missing/out of sync")
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

// loadedConfig caches the config file for flag resolution.
var loadedConfig *cfg.Config

func getConfig() cfg.Config {
	if loadedConfig != nil {
		return *loadedConfig
	}
	c, _ := cfg.ReadConfig(cfg.DefaultConfigPath())
	loadedConfig = &c
	return c
}

// IsDevMode resolves: --dev flag > --no-dev flag > ALLEGRO_NO_DEV env > config > default (true).
func IsDevMode() bool {
	if flagDev {
		return true
	}
	if flagNoDev {
		return false
	}
	if os.Getenv("ALLEGRO_NO_DEV") != "" {
		return false
	}
	if getConfig().NoDev {
		return false
	}
	return true
}

// IsColorEnabled resolves: --no-color > NO_COLOR env > config > default (true).
func IsColorEnabled() bool {
	if flagNoColor {
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if getConfig().NoColor {
		return false
	}
	return true
}

// IsForce returns true if --force flag is set.
func IsForce() bool {
	if flagForce {
		return true
	}
	return os.Getenv("ALLEGRO_FORCE") != ""
}

// IsNoScripts resolves: --no-scripts > ALLEGRO_NO_SCRIPTS env > config > default (false).
func IsNoScripts() bool {
	if flagNoScripts {
		return true
	}
	if os.Getenv("ALLEGRO_NO_SCRIPTS") != "" {
		return true
	}
	return getConfig().NoScripts
}

// IsFrozenLockfile returns true if --frozen-lockfile flag is set.
func IsFrozenLockfile() bool {
	if flagFrozenLockfile {
		return true
	}
	return os.Getenv("ALLEGRO_FROZEN_LOCKFILE") != ""
}
