package cli

import "fmt"

// FormatBinarySize formats bytes into human-readable binary units.
func FormatBinarySize(bytes int64) string {
	switch {
	case bytes < 1024:
		return fmt.Sprintf("%d B", bytes)
	case bytes < 1024*1024:
		return fmt.Sprintf("%.0f KiB", float64(bytes)/1024)
	case bytes < 1024*1024*1024:
		return fmt.Sprintf("%.0f MiB", float64(bytes)/(1024*1024))
	default:
		return fmt.Sprintf("%.0f GiB", float64(bytes)/(1024*1024*1024))
	}
}

// FormatCommaThousands formats an integer with comma-separated thousands.
func FormatCommaThousands(n int) string {
	if n < 0 {
		return "-" + FormatCommaThousands(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	result := make([]byte, 0, len(s)+len(s)/3)
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}
