package cli

import (
	"testing"
)

func TestResolveWorkersDefault(t *testing.T) {
	flagWorkers = 0
	t.Setenv("ALLEGRO_WORKERS", "")
	got := ResolveWorkers()
	if got != 16 {
		t.Errorf("default = %d, want 16", got)
	}
}

func TestResolveWorkersClampLow(t *testing.T) {
	flagWorkers = -5
	got := ResolveWorkers()
	if got != 1 {
		t.Errorf("clamped low = %d, want 1", got)
	}
	flagWorkers = 0
}

func TestResolveWorkersClampHigh(t *testing.T) {
	flagWorkers = 100
	got := ResolveWorkers()
	if got != 32 {
		t.Errorf("clamped high = %d, want 32", got)
	}
	flagWorkers = 0
}

func TestResolveWorkersEnv(t *testing.T) {
	flagWorkers = 0
	t.Setenv("ALLEGRO_WORKERS", "16")
	got := ResolveWorkers()
	if got != 16 {
		t.Errorf("env = %d, want 16", got)
	}
}

func TestIsQuietFlag(t *testing.T) {
	flagQuiet = true
	if !IsQuiet() {
		t.Error("should be quiet")
	}
	flagQuiet = false
}

func TestIsQuietEnv(t *testing.T) {
	flagQuiet = false
	t.Setenv("ALLEGRO_QUIET", "1")
	if !IsQuiet() {
		t.Error("should be quiet from env")
	}
}

func TestQuietOverridesVerbose(t *testing.T) {
	flagQuiet = true
	flagVerbose = true
	if IsVerbose() {
		t.Error("quiet should override verbose")
	}
	flagQuiet = false
	flagVerbose = false
}

func TestResolveLinkStrategyFlag(t *testing.T) {
	flagLinkStrategy = "hardlink"
	if got := ResolveLinkStrategy(); got != "hardlink" {
		t.Errorf("got %q, want hardlink", got)
	}
	flagLinkStrategy = ""
}

func TestResolveLinkStrategyEnv(t *testing.T) {
	flagLinkStrategy = ""
	t.Setenv("ALLEGRO_LINK_STRATEGY", "copy")
	if got := ResolveLinkStrategy(); got != "copy" {
		t.Errorf("got %q, want copy", got)
	}
}

// Phase 2 flag tests

func TestIsDevModeDefault(t *testing.T) {
	flagDev = false; flagNoDev = false
	t.Setenv("ALLEGRO_NO_DEV", "")
	if !IsDevMode() { t.Error("default should be dev=true") }
}

func TestIsDevModeNoDevFlag(t *testing.T) {
	flagDev = false; flagNoDev = true
	if IsDevMode() { t.Error("--no-dev should disable dev") }
	flagNoDev = false
}

func TestIsDevModeDevOverridesNoDev(t *testing.T) {
	flagDev = true; flagNoDev = true
	if !IsDevMode() { t.Error("--dev should override --no-dev") }
	flagDev = false; flagNoDev = false
}

func TestIsDevModeEnvVar(t *testing.T) {
	flagDev = false; flagNoDev = false
	t.Setenv("ALLEGRO_NO_DEV", "1")
	if IsDevMode() { t.Error("ALLEGRO_NO_DEV should disable dev") }
}

func TestIsColorEnabledDefault(t *testing.T) {
	flagNoColor = false
	t.Setenv("NO_COLOR", "")
	if !IsColorEnabled() { t.Error("default should have color") }
}

func TestIsColorDisabledByFlag(t *testing.T) {
	flagNoColor = true
	if IsColorEnabled() { t.Error("--no-color should disable") }
	flagNoColor = false
}

func TestIsColorDisabledByEnv(t *testing.T) {
	flagNoColor = false
	t.Setenv("NO_COLOR", "1")
	if IsColorEnabled() { t.Error("NO_COLOR env should disable") }
}

func TestIsForceFlag(t *testing.T) {
	flagForce = true
	if !IsForce() { t.Error("--force should be true") }
	flagForce = false
}

func TestIsForceEnv(t *testing.T) {
	flagForce = false
	t.Setenv("ALLEGRO_FORCE", "1")
	if !IsForce() { t.Error("ALLEGRO_FORCE should work") }
}

func TestIsNoScriptsFlag(t *testing.T) {
	flagNoScripts = true
	if !IsNoScripts() { t.Error("--no-scripts should be true") }
	flagNoScripts = false
}

func TestIsFrozenLockfileFlag(t *testing.T) {
	flagFrozenLockfile = true
	if !IsFrozenLockfile() { t.Error("--frozen-lockfile should be true") }
	flagFrozenLockfile = false
}

func TestIsFrozenLockfileEnv(t *testing.T) {
	flagFrozenLockfile = false
	t.Setenv("ALLEGRO_FROZEN_LOCKFILE", "1")
	if !IsFrozenLockfile() { t.Error("ALLEGRO_FROZEN_LOCKFILE should work") }
}
