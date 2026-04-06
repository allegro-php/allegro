package config

import (
	"strings"
	"testing"
)

func TestDefaultConfigPath(t *testing.T) {
	p := DefaultConfigPath()
	if !strings.Contains(p, ".allegro") || !strings.HasSuffix(p, "config.json") {
		t.Errorf("config path = %q", p)
	}
}
