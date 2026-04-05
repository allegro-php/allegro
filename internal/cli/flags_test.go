package cli

import (
	"testing"
)

func TestResolveWorkersDefault(t *testing.T) {
	flagWorkers = 0
	t.Setenv("ALLEGRO_WORKERS", "")
	got := ResolveWorkers()
	if got != 8 {
		t.Errorf("default = %d, want 8", got)
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
