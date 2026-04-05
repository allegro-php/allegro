package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	SetVersionInfo("0.1.0", "abc1234", "2026-04-05T00:00:00Z")

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"version"})
	rootCmd.Execute()

	out := buf.String()
	if !strings.Contains(out, "allegro 0.1.0") {
		t.Errorf("output = %q, want version", out)
	}
	if !strings.Contains(out, "abc1234") {
		t.Errorf("output missing commit: %q", out)
	}
}
