package platform

import (
	"runtime"
	"testing"
)

func TestIsWindows(t *testing.T) {
	got := IsWindows()
	want := runtime.GOOS == "windows"
	if got != want {
		t.Errorf("IsWindows() = %v, want %v", got, want)
	}
}

func TestIsDarwin(t *testing.T) {
	got := IsDarwin()
	want := runtime.GOOS == "darwin"
	if got != want {
		t.Errorf("IsDarwin() = %v, want %v", got, want)
	}
}

func TestIsLinux(t *testing.T) {
	got := IsLinux()
	want := runtime.GOOS == "linux"
	if got != want {
		t.Errorf("IsLinux() = %v, want %v", got, want)
	}
}
