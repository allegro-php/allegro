package platform

import "runtime"

// IsWindows returns true when running on Windows.
func IsWindows() bool {
	return runtime.GOOS == "windows"
}

// IsDarwin returns true when running on macOS.
func IsDarwin() bool {
	return runtime.GOOS == "darwin"
}

// IsLinux returns true when running on Linux.
func IsLinux() bool {
	return runtime.GOOS == "linux"
}
