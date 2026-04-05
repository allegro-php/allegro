package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePathFlag(t *testing.T) {
	got := ResolvePath("/custom/store", "env/store")
	if got != "/custom/store" {
		t.Errorf("flag should win, got %q", got)
	}
}

func TestResolvePathEnv(t *testing.T) {
	got := ResolvePath("", "/env/store")
	if got != "/env/store" {
		t.Errorf("env should win when no flag, got %q", got)
	}
}

func TestResolvePathDefault(t *testing.T) {
	got := ResolvePath("", "")
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".allegro/store")
	if got != want {
		t.Errorf("default = %q, want %q", got, want)
	}
}
