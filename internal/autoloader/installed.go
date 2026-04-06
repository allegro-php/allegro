package autoloader

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/allegro-php/allegro/internal/parser"
	"github.com/allegro-php/allegro/internal/store"
)

//go:embed InstalledVersions.php
var installedVersionsPHP embed.FS

// InstalledJSON represents vendor/composer/installed.json.
type InstalledJSON struct {
	Packages       []InstalledPackage `json:"packages"`
	Dev            bool               `json:"dev"`
	DevPackageNames []string          `json:"dev-package-names"`
}

// InstalledPackage represents a single package in installed.json.
// InstalledPackage represents a single package in installed.json.
type InstalledPackage struct {
	Name              string                 `json:"name"`
	Version           string                 `json:"version"`
	VersionNormalized string                 `json:"version_normalized,omitempty"`
	Type              string                 `json:"type"`
	Require           map[string]string      `json:"require,omitempty"`
	Autoload          interface{}            `json:"autoload"`
	InstallPath       string                 `json:"install-path"`
	Extra             map[string]interface{} `json:"extra,omitempty"`
	Description       string                 `json:"description,omitempty"`
	Bin               []string               `json:"bin,omitempty"`
	NotificationURL   string                 `json:"notification-url,omitempty"`
	Replace           map[string]string      `json:"replace,omitempty"`
	Provide           map[string]string      `json:"provide,omitempty"`
	Source            map[string]interface{} `json:"source,omitempty"`
}

// GenerateInstalledJSON creates installed.json content from composer.lock data.
func GenerateInstalledJSON(lock *parser.ComposerLock) ([]byte, error) {
	packages := parser.FilterInstallable(lock.Packages)
	packagesDev := parser.FilterInstallable(lock.PackagesDev)
	all := append(packages, packagesDev...)

	installed := InstalledJSON{
		Dev:             true,
		DevPackageNames: parser.DevPackageNames(lock),
	}

	for _, pkg := range all {
		typ := pkg.Type
		if typ == "" {
			typ = "library"
		}

		var autoload interface{} = map[string]interface{}{}
		if pkg.Autoload != nil {
			autoload = pkg.Autoload
		}

		ip := InstalledPackage{
			Name:              pkg.Name,
			Version:           pkg.Version,
			VersionNormalized: pkg.VersionNormalized,
			Type:              typ,
			Require:           pkg.Require,
			Autoload:          autoload,
			InstallPath:       "../" + pkg.Name,
			Extra:             pkg.Extra,
			Description:       pkg.Description,
			Bin:               pkg.Bin,
			NotificationURL:   pkg.NotificationURL,
			Replace:           pkg.Replace,
			Provide:           pkg.Provide,
			Source:            pkg.Source,
		}
		installed.Packages = append(installed.Packages, ip)
	}

	if installed.Packages == nil {
		installed.Packages = []InstalledPackage{}
	}
	if installed.DevPackageNames == nil {
		installed.DevPackageNames = []string{}
	}

	return json.MarshalIndent(installed, "", "    ")
}

// GenerateInstalledPHP creates installed.php content.
func GenerateInstalledPHP(lock *parser.ComposerLock, composerJSON map[string]interface{}) string {
	// Root entry
	rootName := "'__root__'"
	rootVersion := "'dev-main'"
	rootType := "'project'"

	if v, ok := composerJSON["name"].(string); ok && v != "" {
		rootName = fmt.Sprintf("'%s'", escapePHP(v))
	}
	if v, ok := composerJSON["version"].(string); ok && v != "" {
		rootVersion = fmt.Sprintf("'%s'", escapePHP(v))
	}
	if v, ok := composerJSON["type"].(string); ok && v != "" {
		rootType = fmt.Sprintf("'%s'", escapePHP(v))
	}

	var b strings.Builder
	b.WriteString("<?php return array(\n")
	b.WriteString("    'root' => array(\n")
	b.WriteString(fmt.Sprintf("        'name' => %s,\n", rootName))
	b.WriteString(fmt.Sprintf("        'pretty_version' => %s,\n", rootVersion))
	b.WriteString(fmt.Sprintf("        'version' => %s,\n", rootVersion))
	b.WriteString("        'reference' => NULL,\n")
	b.WriteString(fmt.Sprintf("        'type' => %s,\n", rootType))
	b.WriteString("        'install_path' => __DIR__ . '/../../',\n")
	b.WriteString("        'aliases' => array(),\n")
	b.WriteString("        'dev' => true,\n")
	b.WriteString("    ),\n")
	b.WriteString("    'versions' => array(\n")

	packages := parser.FilterInstallable(lock.Packages)
	packagesDev := parser.FilterInstallable(lock.PackagesDev)
	all := append(packages, packagesDev...)

	// Track emitted names to avoid duplicates from replace/provide
	emitted := make(map[string]bool)

	// Root package version entry (mirrors the 'root' section)
	rootNameStr := "__root__"
	if v, ok := composerJSON["name"].(string); ok && v != "" {
		rootNameStr = v
	}
	b.WriteString(fmt.Sprintf("        '%s' => array(\n", escapePHP(rootNameStr)))
	b.WriteString(fmt.Sprintf("            'pretty_version' => %s,\n", rootVersion))
	b.WriteString(fmt.Sprintf("            'version' => %s,\n", rootVersion))
	b.WriteString("            'reference' => NULL,\n")
	b.WriteString(fmt.Sprintf("            'type' => %s,\n", rootType))
	b.WriteString("            'install_path' => __DIR__ . '/../../',\n")
	b.WriteString("            'aliases' => array(),\n")
	b.WriteString("            'dev_requirement' => false,\n")
	b.WriteString("        ),\n")
	emitted[rootNameStr] = true

	// Emit root package's replace/provide entries.
	// Monorepos like Magento use root replace to declare 241+ sub-packages.
	if rootReplace, ok := composerJSON["replace"].(map[string]interface{}); ok {
		for name, ver := range rootReplace {
			verStr := "*"
			if v, ok := ver.(string); ok {
				verStr = v
			}
			if emitted[name] || parser.IsPlatformPackage(name) {
				continue
			}
			b.WriteString(fmt.Sprintf("        '%s' => array(\n", escapePHP(name)))
			b.WriteString("            'dev_requirement' => false,\n")
			b.WriteString(fmt.Sprintf("            'replaced' => array('%s'),\n", escapePHP(verStr)))
			b.WriteString("        ),\n")
			emitted[name] = true
		}
	}
	if rootProvide, ok := composerJSON["provide"].(map[string]interface{}); ok {
		for name, ver := range rootProvide {
			verStr := "*"
			if v, ok := ver.(string); ok {
				verStr = v
			}
			if emitted[name] || parser.IsPlatformPackage(name) {
				continue
			}
			b.WriteString(fmt.Sprintf("        '%s' => array(\n", escapePHP(name)))
			b.WriteString("            'dev_requirement' => false,\n")
			b.WriteString(fmt.Sprintf("            'provided' => array('%s'),\n", escapePHP(verStr)))
			b.WriteString("        ),\n")
			emitted[name] = true
		}
	}

	for _, pkg := range all {
		isDev := parser.IsDevPackage(pkg.Name, lock)
		ref := "NULL"
		if pkg.Dist != nil && pkg.Dist.Reference != "" {
			ref = fmt.Sprintf("'%s'", escapePHP(pkg.Dist.Reference))
		}
		typ := pkg.Type
		if typ == "" {
			typ = "library"
		}
		version := pkg.VersionNormalized
		if version == "" {
			version = pkg.Version
		}

		b.WriteString(fmt.Sprintf("        '%s' => array(\n", escapePHP(pkg.Name)))
		b.WriteString(fmt.Sprintf("            'pretty_version' => '%s',\n", escapePHP(pkg.Version)))
		b.WriteString(fmt.Sprintf("            'version' => '%s',\n", escapePHP(version)))
		b.WriteString(fmt.Sprintf("            'reference' => %s,\n", ref))
		b.WriteString(fmt.Sprintf("            'type' => '%s',\n", escapePHP(typ)))
		b.WriteString(fmt.Sprintf("            'install_path' => __DIR__ . '/../%s',\n", escapePHP(pkg.Name)))
		b.WriteString("            'aliases' => array(),\n")
		devReq := "false"
		if isDev {
			devReq = "true"
		}
		b.WriteString(fmt.Sprintf("            'dev_requirement' => %s,\n", devReq))
		b.WriteString("        ),\n")
		emitted[pkg.Name] = true
	}

	// Emit replaced/provided virtual packages.
	// e.g. laravel/framework replaces illuminate/auth, illuminate/contracts, etc.
	// Without these, InstalledVersions::isInstalled('illuminate/contracts') returns false.
	// Skip platform packages (ext-*, php, lib-*) and already-emitted names.
	for _, pkg := range all {
		isDev := parser.IsDevPackage(pkg.Name, lock)
		devReq := "false"
		if isDev {
			devReq = "true"
		}
		for name, ver := range pkg.Replace {
			if emitted[name] || parser.IsPlatformPackage(name) {
				continue
			}
			b.WriteString(fmt.Sprintf("        '%s' => array(\n", escapePHP(name)))
			b.WriteString(fmt.Sprintf("            'dev_requirement' => %s,\n", devReq))
			b.WriteString(fmt.Sprintf("            'replaced' => array('%s'),\n", escapePHP(ver)))
			b.WriteString("        ),\n")
			emitted[name] = true
		}
		for name, ver := range pkg.Provide {
			if emitted[name] || parser.IsPlatformPackage(name) {
				continue
			}
			b.WriteString(fmt.Sprintf("        '%s' => array(\n", escapePHP(name)))
			b.WriteString(fmt.Sprintf("            'dev_requirement' => %s,\n", devReq))
			b.WriteString(fmt.Sprintf("            'provided' => array('%s'),\n", escapePHP(ver)))
			b.WriteString("        ),\n")
			emitted[name] = true
		}
	}

	b.WriteString("    ),\n")
	b.WriteString(");\n")
	return b.String()
}

// WriteInstalledFiles writes installed.json and installed.php to vendor/composer/.
func WriteInstalledFiles(vendorDir string, lock *parser.ComposerLock, composerJSON map[string]interface{}) error {
	composerDir := filepath.Join(vendorDir, "composer")
	if err := os.MkdirAll(composerDir, 0755); err != nil {
		return fmt.Errorf("create composer dir: %w", err)
	}

	jsonData, err := GenerateInstalledJSON(lock)
	if err != nil {
		return fmt.Errorf("generate installed.json: %w", err)
	}
	if err := store.WriteFileAtomic(filepath.Join(composerDir, "installed.json"), jsonData, 0644); err != nil {
		return fmt.Errorf("write installed.json: %w", err)
	}

	phpContent := GenerateInstalledPHP(lock, composerJSON)
	if err := store.WriteFileAtomic(filepath.Join(composerDir, "installed.php"), []byte(phpContent), 0644); err != nil {
		return fmt.Errorf("write installed.php: %w", err)
	}

	// Write InstalledVersions.php — Composer runtime class needed by autoloader
	ivData, err := installedVersionsPHP.ReadFile("InstalledVersions.php")
	if err != nil {
		return fmt.Errorf("read embedded InstalledVersions.php: %w", err)
	}
	if err := store.WriteFileAtomic(filepath.Join(composerDir, "InstalledVersions.php"), ivData, 0644); err != nil {
		return fmt.Errorf("write InstalledVersions.php: %w", err)
	}

	return nil
}

func escapePHP(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	return s
}
