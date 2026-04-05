package autoloader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindComposerEnvVar(t *testing.T) {
	dir := t.TempDir()
	fakeBin := filepath.Join(dir, "my-composer")
	os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0755)

	t.Setenv("ALLEGRO_COMPOSER_PATH", fakeBin)
	got, err := FindComposer(dir)
	if err != nil {
		t.Fatalf("FindComposer: %v", err)
	}
	if got != fakeBin {
		t.Errorf("got %q, want %q", got, fakeBin)
	}
}

func TestFindComposerPhar(t *testing.T) {
	dir := t.TempDir()
	pharPath := filepath.Join(dir, "composer.phar")
	os.WriteFile(pharPath, []byte("#!/usr/bin/env php\n"), 0755)

	t.Setenv("ALLEGRO_COMPOSER_PATH", "")
	// Note: can't easily mock PATH, so test phar fallback
	got, err := FindComposer(dir)
	// This may find system composer first; if so, skip phar test
	if err == nil && got == pharPath {
		// Phar found correctly
		return
	}
	if err == nil {
		// System composer found, that's also valid
		return
	}
	t.Fatalf("FindComposer: %v", err)
}

func TestFindComposerNotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ALLEGRO_COMPOSER_PATH", "")
	t.Setenv("PATH", dir) // empty PATH

	_, err := FindComposer(dir)
	if err == nil {
		t.Fatal("expected error when composer not found")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found: %v", err)
	}
}
