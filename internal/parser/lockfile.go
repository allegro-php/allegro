package parser

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ParseLockFile reads and parses a composer.lock file.
func ParseLockFile(path string) (*ComposerLock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("composer.lock not found. Run `composer install` first.")
		}
		if os.IsPermission(err) {
			return nil, fmt.Errorf("composer.lock: permission denied")
		}
		return nil, fmt.Errorf("read composer.lock: %w", err)
	}

	var lock ComposerLock
	if err := json.Unmarshal(data, &lock); err != nil {
		if synErr, ok := err.(*json.SyntaxError); ok {
			line, col := offsetToLineCol(data, synErr.Offset)
			return nil, fmt.Errorf("invalid composer.lock JSON at line %d, column %d: %w", line, col, err)
		}
		return nil, fmt.Errorf("invalid composer.lock JSON: %w", err)
	}

	return &lock, nil
}

// IsPlatformPackage returns true for php, ext-*, lib-* packages.
func IsPlatformPackage(name string) bool {
	if name == "php" || name == "php-64bit" || name == "hhvm" {
		return true
	}
	if strings.HasPrefix(name, "ext-") || strings.HasPrefix(name, "lib-") {
		return true
	}
	return false
}

// FilterInstallable returns packages that are not platform pseudo-packages
// and not path/null dist types.
func FilterInstallable(packages []Package) []Package {
	var result []Package
	for _, pkg := range packages {
		if IsPlatformPackage(pkg.Name) {
			continue
		}
		result = append(result, pkg)
	}
	return result
}

// MergePackages merges packages and packages-dev into a single list,
// filtering out platform pseudo-packages.
func MergePackages(lock *ComposerLock) []Package {
	all := make([]Package, 0, len(lock.Packages)+len(lock.PackagesDev))
	all = append(all, FilterInstallable(lock.Packages)...)
	all = append(all, FilterInstallable(lock.PackagesDev)...)
	return all
}

// DevPackageNames returns the names from packages-dev (for installed.json).
func DevPackageNames(lock *ComposerLock) []string {
	names := make([]string, 0, len(lock.PackagesDev))
	for _, pkg := range lock.PackagesDev {
		if !IsPlatformPackage(pkg.Name) {
			names = append(names, pkg.Name)
		}
	}
	return names
}

// IsDevPackage checks if a package name is in packages-dev.
func IsDevPackage(name string, lock *ComposerLock) bool {
	for _, pkg := range lock.PackagesDev {
		if pkg.Name == name {
			return true
		}
	}
	return false
}

func offsetToLineCol(data []byte, offset int64) (int, int) {
	line := 1
	col := 1
	for i := int64(0); i < offset && i < int64(len(data)); i++ {
		if data[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}
