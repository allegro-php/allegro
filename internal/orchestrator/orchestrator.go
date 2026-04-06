package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
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
	Version      string
	Dev          bool   // true = install dev deps (default)
	NoScripts    bool   // skip post-install/post-update scripts
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

	// Step 3: Parse and build plan (filter dev packages if --no-dev)
	var allPackages []parser.Package
	if o.config.Dev {
		allPackages = parser.MergePackages(lock)
	} else {
		allPackages = parser.FilterInstallable(lock.Packages) // exclude packages-dev
	}
	plan := o.buildPlan(allPackages)

	if !o.config.Quiet {
		log.Printf("Packages: %d new, %d cached, %d skipped",
			len(plan.NewPackages), len(plan.CachedPackages), len(plan.SkippedPackages))
	}

	// Warn about skipped packages per spec §8.1 step 4
	for _, sp := range plan.SkippedPackages {
		log.Printf("warning: skipping package %s (%s)", sp.Name, sp.Reason)
	}

	// Step 4-5: Download and store new packages
	if len(plan.NewPackages) > 0 {
		if err := o.downloadAndStore(ctx, plan.NewPackages); err != nil {
			return err
		}
	}

	// Step 6: Acquire lock and build vendor
	fl, err := linker.AcquireLock(ctx, o.config.ProjectDir)
	if err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	defer fl.Release()

	// Stale cleanup
	linker.CleanStaleVendorDirs(o.config.ProjectDir)
	linker.CleanStaleStoreTmp(o.store.TmpDir())

	// Build vendor tree
	vendorTmp := filepath.Join(o.config.ProjectDir, "vendor.allegro.tmp")
	if err := o.buildVendorTree(ctx, vendorTmp, plan.AllPackages, lnk, strategy); err != nil {
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
		os.RemoveAll(vendorTmp) // clean up on swap failure
		return err
	}

	// Step 7: Generate autoloader
	if !o.config.NoAutoload {
		composerBin, err := autoloader.FindComposer(o.config.ProjectDir)
		if err != nil {
			return fmt.Errorf("exit 5: %w", err)
		}
		if err := autoloader.RunDumpautoload(composerBin, o.config.ProjectDir, o.config.Verbose, !o.config.Dev); err != nil {
			return fmt.Errorf("composer dumpautoload failed: %w", err)
		}
	}

	// Step 8: Write state file with Phase 2 fields
	lockHash, _ := parser.ComputeLockHash(lockPath)
	pkgMap := make(map[string]string)
	devPkgNames := parser.DevPackageNames(lock)
	for _, pkg := range plan.AllPackages {
		pkgMap[pkg.Name] = pkg.Version
	}
	if err := linker.WriteVendorState(vendorDir, linker.WriteVendorStateOpts{
		Version:         o.config.Version,
		Strategy:        strategy,
		LockHash:        lockHash,
		Packages:        pkgMap,
		Dev:             o.config.Dev,
		DevPackages:     devPkgNames,
		ScriptsExecuted: false, // scripts run after flock release
	}); err != nil {
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

		if err := o.extractAndStore(ctx, r, packages); err != nil {
			return err
		}
	}
	return nil
}

// extractAndStore handles extraction with a single re-download retry per spec §11.1.
// The worker pool has already used its share of the 3-retry budget for HTTP-level
// errors. Extraction failure gets at most 1 re-download attempt to stay within
// the shared budget.
func (o *Orchestrator) extractAndStore(ctx context.Context, r fetcher.DownloadResult, packages []parser.Package) error {
	// Attempt 1: extract the already-downloaded data
	err := o.tryExtract(r.Data, r.Task, packages)
	if err == nil {
		return nil
	}

	// Attempt 2: re-download and retry extraction (1 retry only)
	log.Printf("extraction failed for %s, re-downloading: %v", r.Task.Name, err)
	time.Sleep(time.Second) // backoff

	// Single-shot re-download (no internal retries — budget already spent by worker pool)
	client := fetcher.NewClient()
	resp, dlErr := client.DownloadFull(ctx, r.Task.URL)
	if dlErr != nil {
		return fmt.Errorf("download %s (re-download for extraction retry): %w", r.Task.Name, dlErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download %s: HTTP %d on extraction retry", r.Task.Name, resp.StatusCode)
	}

	if err := o.tryExtract(resp.Body, r.Task, packages); err != nil {
		return fmt.Errorf("archive extraction failed for %s: %w", r.Task.Name, err)
	}
	return nil
}

// tryExtract attempts to extract, strip, hash, and store a package's files.
func (o *Orchestrator) tryExtract(data []byte, task fetcher.DownloadTask, packages []parser.Package) error {
	tmpDir, err := linker.CreateTempDir(o.store.TmpDir())
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	if err := store.ExtractByType(data, task.DistType, tmpDir, task.Name); err != nil {
		return fmt.Errorf("archive extraction failed for %s: %w", task.Name, err)
	}

	if err := store.StripTopLevelDir(tmpDir); err != nil {
		if errors.Is(err, store.ErrEmptyArchive) {
			return fmt.Errorf("archive extraction failed for %s: %w", task.Name, err)
		}
		return fmt.Errorf("strip top-level for %s: %w", task.Name, err)
	}

	manifest, err := o.storeExtractedFiles(tmpDir, task.Name, data, packages)
	if err != nil {
		return err
	}

	return o.store.WriteManifest(manifest)
}

func (o *Orchestrator) storeExtractedFiles(dir, pkgName string, archiveData []byte, packages []parser.Package) (*store.Manifest, error) {
	var pkg parser.Package
	found := false
	for _, p := range packages {
		if p.Name == pkgName {
			pkg = p
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("internal: package %s not in plan", pkgName)
	}

	manifest := &store.Manifest{
		Name:     pkg.Name,
		Version:  pkg.Version,
		DistHash: "sha256:" + store.HashBytes(archiveData),
		Files:    []store.FileEntry{},
		StoredAt: time.Now().UTC(),
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

func (o *Orchestrator) buildVendorTree(ctx context.Context, vendorTmp string, packages []parser.Package, lnk linker.Linker, strategy linker.Strategy) error {
	redownloaded := make(map[string]bool) // track already-redownloaded packages

	// Phase 1: Verify CAS integrity and re-download missing packages
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

			if !o.store.FileExists(hash) {
				pkgKey := pkg.Name + "@" + pkg.Version
				if redownloaded[pkgKey] {
					return fmt.Errorf("CAS file still missing for %s/%s after re-download", pkg.Name, f.Path)
				}
				log.Printf("warning: CAS file missing for %s/%s, re-downloading package", pkg.Name, f.Path)
				if err := o.redownloadPackage(ctx, pkg); err != nil {
					return fmt.Errorf("re-download %s for missing CAS file: %w", pkg.Name, err)
				}
				redownloaded[pkgKey] = true
				if !o.store.FileExists(hash) {
					return fmt.Errorf("CAS file still missing for %s/%s after re-download", pkg.Name, f.Path)
				}
			}
		}
	}

	// Phase 2: Collect all link operations
	var allOps []LinkOp
	for _, pkg := range packages {
		manifest, err := o.store.ReadManifest(pkg.Name, pkg.Version)
		if err != nil {
			return fmt.Errorf("read manifest %s: %w", pkg.Name, err)
		}
		ops := CollectLinkOps(o.store, pkg.Name, manifest, vendorTmp)
		allOps = append(allOps, ops...)
	}

	// Phase 3: Parallel link with worker pool
	workers := o.config.Workers
	if workers < 1 {
		workers = 8
	}
	return ParallelLink(allOps, lnk, strategy, workers)
}

// redownloadPackage re-downloads and re-stores a package when CAS files are missing.
func (o *Orchestrator) redownloadPackage(ctx context.Context, pkg parser.Package) error {
	if pkg.Dist == nil {
		return fmt.Errorf("cannot re-download %s: no dist info", pkg.Name)
	}
	pool := fetcher.NewPool(1)
	results := pool.Download(ctx, []fetcher.DownloadTask{{
		Name: pkg.Name, URL: pkg.Dist.URL,
		Shasum: pkg.Dist.Shasum, DistType: pkg.Dist.Type,
	}})
	if len(results) == 0 || results[0].Error != nil {
		if len(results) > 0 {
			return results[0].Error
		}
		return fmt.Errorf("re-download %s: no results", pkg.Name)
	}

	r := results[0]
	return o.extractAndStore(ctx, r, []parser.Package{pkg})
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
			if rbErr := os.Rename(vendorOld, vendorDir); rbErr != nil {
				log.Printf("warning: failed to restore old vendor: %v", rbErr)
			}
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
