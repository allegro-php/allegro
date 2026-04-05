package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/allegro-php/allegro/internal/autoloader"
	"github.com/allegro-php/allegro/internal/fetcher"
	"github.com/allegro-php/allegro/internal/linker"
	"github.com/allegro-php/allegro/internal/parser"
	"github.com/allegro-php/allegro/internal/store"
)

// Config holds all configuration for an install run.
type Config struct {
	ProjectDir   string
	StorePath    string
	LinkStrategy string // forced strategy or ""
	Workers      int
	NoAutoload   bool
	Verbose      bool
	Quiet        bool
	Version      string // allegro version for state file
}

// Orchestrator coordinates the install pipeline.
type Orchestrator struct {
	config Config
	store  *store.Store
}

// New creates an Orchestrator.
func New(cfg Config) *Orchestrator {
	return &Orchestrator{
		config: cfg,
		store:  store.New(cfg.StorePath),
	}
}

// Install runs the full install pipeline.
func (o *Orchestrator) Install(ctx context.Context) error {
	start := time.Now()

	// Step 1: Locate project files
	lockPath := filepath.Join(o.config.ProjectDir, "composer.lock")
	lock, err := parser.ParseLockFile(lockPath)
	if err != nil {
		return err
	}

	// Step 2: Detect link strategy
	if err := o.store.EnsureDirectories(); err != nil {
		return err
	}
	if err := o.store.EnsureMetadata(); err != nil {
		return err
	}

	strategy, lnk := linker.DetectStrategy(o.store.Root, o.config.ProjectDir, o.config.LinkStrategy)
	if !o.config.Quiet {
		log.Printf("Link strategy: %s", strategy)
	}

	// Step 3: Parse and build plan
	allPackages := parser.MergePackages(lock)
	plan := o.buildPlan(allPackages)

	if !o.config.Quiet {
		log.Printf("Packages: %d new, %d cached, %d skipped",
			len(plan.NewPackages), len(plan.CachedPackages), len(plan.SkippedPackages))
	}

	// Step 4-5: Download and store new packages
	if len(plan.NewPackages) > 0 {
		if err := o.downloadAndStore(ctx, plan.NewPackages); err != nil {
			return err
		}
	}

	// Step 6: Acquire lock and build vendor
	fl, err := linker.AcquireLock(o.config.ProjectDir)
	if err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	defer fl.Release()

	// Stale cleanup
	linker.CleanStaleVendorDirs(o.config.ProjectDir)
	linker.CleanStaleStoreTmp(o.store.TmpDir())

	// Build vendor tree
	vendorTmp := filepath.Join(o.config.ProjectDir, "vendor.allegro.tmp")
	if err := o.buildVendorTree(vendorTmp, plan.AllPackages, lnk, strategy); err != nil {
		os.RemoveAll(vendorTmp)
		return err
	}

	// Generate installed.json and installed.php
	composerJSON := o.readComposerJSON()
	if err := autoloader.WriteInstalledFiles(vendorTmp, lock, composerJSON); err != nil {
		os.RemoveAll(vendorTmp)
		return err
	}

	// Generate bin proxies
	if err := o.generateBinProxies(vendorTmp, plan.AllPackages); err != nil {
		os.RemoveAll(vendorTmp)
		return err
	}

	// Atomic swap
	vendorDir := filepath.Join(o.config.ProjectDir, "vendor")
	if err := o.atomicSwap(vendorDir, vendorTmp); err != nil {
		return err
	}

	// Step 7: Generate autoloader
	if !o.config.NoAutoload {
		composerBin, err := autoloader.FindComposer(o.config.ProjectDir)
		if err != nil {
			return fmt.Errorf("exit 5: %w", err)
		}
		if err := autoloader.RunDumpautoload(composerBin, o.config.ProjectDir, o.config.Verbose); err != nil {
			return fmt.Errorf("composer dumpautoload failed: %w", err)
		}
	}

	// Step 8: Write state file
	lockHash, _ := parser.ComputeLockHash(lockPath)
	pkgMap := make(map[string]string)
	for _, pkg := range plan.AllPackages {
		pkgMap[pkg.Name] = pkg.Version
	}
	if err := linker.WriteVendorState(vendorDir, o.config.Version, strategy, lockHash, pkgMap); err != nil {
		return err
	}

	elapsed := time.Since(start)
	if !o.config.Quiet {
		log.Printf("Installed %d packages in %.1fs (%s)", len(plan.AllPackages), elapsed.Seconds(), strategy)
	}

	return nil
}

func (o *Orchestrator) buildPlan(packages []parser.Package) InstallPlan {
	plan := InstallPlan{AllPackages: make([]parser.Package, 0)}

	for _, pkg := range packages {
		if pkg.Dist == nil {
			plan.SkippedPackages = append(plan.SkippedPackages, SkippedPackage{
				Name: pkg.Name, Reason: "dist is null",
			})
			continue
		}
		if pkg.Dist.Type == "path" {
			plan.SkippedPackages = append(plan.SkippedPackages, SkippedPackage{
				Name: pkg.Name, Reason: "dist type is path",
			})
			continue
		}

		plan.AllPackages = append(plan.AllPackages, pkg)

		if o.store.ManifestExists(pkg.Name, pkg.Version) {
			plan.CachedPackages = append(plan.CachedPackages, pkg)
		} else {
			plan.NewPackages = append(plan.NewPackages, pkg)
		}
	}
	return plan
}

func (o *Orchestrator) downloadAndStore(ctx context.Context, packages []parser.Package) error {
	tasks := make([]fetcher.DownloadTask, len(packages))
	for i, pkg := range packages {
		tasks[i] = fetcher.DownloadTask{
			Name:     pkg.Name,
			URL:      pkg.Dist.URL,
			Shasum:   pkg.Dist.Shasum,
			DistType: pkg.Dist.Type,
		}
	}

	pool := fetcher.NewPool(o.config.Workers)
	results := pool.Download(ctx, tasks)

	for _, r := range results {
		if r.Error != nil {
			return fmt.Errorf("download %s: %w", r.Task.Name, r.Error)
		}

		// Extract and store
		tmpDir, err := linker.CreateTempDir(o.store.TmpDir())
		if err != nil {
			return err
		}

		if err := store.ExtractByType(r.Data, r.Task.DistType, tmpDir); err != nil {
			os.RemoveAll(tmpDir)
			return fmt.Errorf("extract %s: %w", r.Task.Name, err)
		}

		if err := store.StripTopLevelDir(tmpDir); err != nil && err.Error() != "empty archive (no files after extraction)" {
			// Only fail on actual errors, not "no stripping needed"
			if _, statErr := os.ReadDir(tmpDir); statErr != nil {
				os.RemoveAll(tmpDir)
				return fmt.Errorf("strip %s: %w", r.Task.Name, err)
			}
		}

		// Hash and store each file
		manifest, err := o.storeExtractedFiles(tmpDir, r.Task.Name, packages)
		if err != nil {
			os.RemoveAll(tmpDir)
			return err
		}

		if err := o.store.WriteManifest(manifest); err != nil {
			os.RemoveAll(tmpDir)
			return err
		}

		os.RemoveAll(tmpDir)
	}
	return nil
}

func (o *Orchestrator) storeExtractedFiles(dir, pkgName string, packages []parser.Package) (*store.Manifest, error) {
	var pkg parser.Package
	for _, p := range packages {
		if p.Name == pkgName {
			pkg = p
			break
		}
	}

	manifest := &store.Manifest{
		Name:    pkg.Name,
		Version: pkg.Version,
		Files:   []store.FileEntry{},
		StoredAt: time.Now().UTC(),
	}
	if pkg.Dist != nil {
		manifest.DistHash = "sha256:" + store.HashBytes([]byte(pkg.Dist.Reference))
	}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		relPath, _ := filepath.Rel(dir, path)
		hash, err := store.HashFile(path)
		if err != nil {
			return err
		}

		executable := info.Mode()&0111 != 0

		if err := o.store.StoreFile(path, hash, executable); err != nil {
			return err
		}

		manifest.Files = append(manifest.Files, store.FileEntry{
			Path:       relPath,
			Hash:       "sha256:" + hash,
			Size:       info.Size(),
			Executable: executable,
		})
		return nil
	})

	return manifest, err
}

func (o *Orchestrator) buildVendorTree(vendorTmp string, packages []parser.Package, lnk linker.Linker, strategy linker.Strategy) error {
	for _, pkg := range packages {
		manifest, err := o.store.ReadManifest(pkg.Name, pkg.Version)
		if err != nil {
			return fmt.Errorf("read manifest %s: %w", pkg.Name, err)
		}

		for _, f := range manifest.Files {
			hash := f.Hash
			if len(hash) > 7 && hash[:7] == "sha256:" {
				hash = hash[7:]
			}

			srcPath := o.store.FilePath(hash)
			dstPath := filepath.Join(vendorTmp, pkg.Name, f.Path)

			if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
				return err
			}

			if err := lnk.LinkFile(srcPath, dstPath); err != nil {
				return fmt.Errorf("link %s/%s: %w", pkg.Name, f.Path, err)
			}

			// Set vendor permissions (not for hardlinks — they share CAS inode)
			if strategy != linker.Hardlink {
				perm := os.FileMode(0644)
				if f.Executable {
					perm = 0755
				}
				os.Chmod(dstPath, perm)
			}
		}
	}
	return nil
}

func (o *Orchestrator) generateBinProxies(vendorTmp string, packages []parser.Package) error {
	binDir := filepath.Join(vendorTmp, "bin")

	for _, pkg := range packages {
		if len(pkg.Bin) == 0 {
			continue
		}

		for _, binEntry := range pkg.Bin {
			targetPath := filepath.Join(vendorTmp, pkg.Name, binEntry)
			targetType, err := autoloader.DetectBinTarget(targetPath)
			if err != nil {
				// Target doesn't exist yet in tmp, try to detect from the entry name
				targetType = autoloader.BinNonPHP
			}

			var content string
			switch targetType {
			case autoloader.BinPHPNoShebang:
				content = autoloader.GeneratePHPProxyNoShebang(pkg.Name, binEntry)
			case autoloader.BinPHPWithShebang:
				content = autoloader.GeneratePHPProxyWithShebang(pkg.Name, binEntry)
			default:
				content = autoloader.GenerateShellProxy(pkg.Name, binEntry)
			}

			proxyPath := filepath.Join(binDir, autoloader.BinBasename(binEntry))
			if err := os.MkdirAll(binDir, 0755); err != nil {
				return err
			}
			if err := os.WriteFile(proxyPath, []byte(content), 0755); err != nil {
				return err
			}
		}
	}
	return nil
}

func (o *Orchestrator) atomicSwap(vendorDir, vendorTmp string) error {
	vendorOld := vendorDir + ".allegro.old"

	// Remove stale old dir if exists
	os.RemoveAll(vendorOld)

	// Rename existing vendor/ to vendor.allegro.old/
	if _, err := os.Stat(vendorDir); err == nil {
		if err := os.Rename(vendorDir, vendorOld); err != nil {
			return fmt.Errorf("rename vendor to old: %w", err)
		}
	}

	// Rename vendor.allegro.tmp/ to vendor/
	if err := os.Rename(vendorTmp, vendorDir); err != nil {
		// Try to restore old vendor
		if _, statErr := os.Stat(vendorOld); statErr == nil {
			os.Rename(vendorOld, vendorDir)
		}
		return fmt.Errorf("rename tmp to vendor: %w", err)
	}

	// Delete old vendor
	os.RemoveAll(vendorOld)
	return nil
}

func (o *Orchestrator) readComposerJSON() map[string]interface{} {
	path := filepath.Join(o.config.ProjectDir, "composer.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]interface{}{}
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return map[string]interface{}{}
	}
	return result
}
