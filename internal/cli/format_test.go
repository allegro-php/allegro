package cli

import "testing"

func TestFormatBinarySize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1 KiB"},
		{1536, "2 KiB"},
		{1048576, "1 MiB"},
		{104857600, "100 MiB"},
		{1073741824, "1 GiB"},
		{5368709120, "5 GiB"},
	}
	for _, tt := range tests {
		got := FormatBinarySize(tt.bytes)
		if got != tt.want {
			t.Errorf("FormatBinarySize(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}

func TestFormatCommaThousands(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{12459, "12,459"},
		{1000000, "1,000,000"},
		{-1234, "-1,234"},
	}
	for _, tt := range tests {
		got := FormatCommaThousands(tt.n)
		if got != tt.want {
			t.Errorf("FormatCommaThousands(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}
