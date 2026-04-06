package autoloader

import "testing"

func TestParseComposerVersion(t *testing.T) {
	tests := []struct {
		output string
		want   string
	}{
		{"Composer version 2.8.4 2024-12-11 11:18:16", "2.8.4"},
		{"Composer version 2.0.0 2020-06-03", "2.0.0"},
		{"Composer version 1.10.26 2022-04-13", "1.10.26"},
		{"Some other output", ""},
	}
	for _, tt := range tests {
		got := parseComposerVersion(tt.output)
		if got != tt.want {
			t.Errorf("parseComposerVersion(%q) = %q, want %q", tt.output, got, tt.want)
		}
	}
}

func TestParseMajorMinor(t *testing.T) {
	major, minor := parseMajorMinor("2.8.4")
	if major != 2 || minor != 8 {
		t.Errorf("got %d.%d, want 2.8", major, minor)
	}
	major, minor = parseMajorMinor("1.10.26")
	if major != 1 || minor != 10 {
		t.Errorf("got %d.%d, want 1.10", major, minor)
	}
}
