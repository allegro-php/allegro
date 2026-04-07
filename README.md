<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="art/logo-dark.svg">
    <source media="(prefers-color-scheme: light)" srcset="art/logo-light.svg">
    <img alt="Allegro" src="art/logo-light.svg" width="120">
  </picture>
</p>

# Allegro — Fast, Disk-Efficient PHP Package Linker

Allegro is a fast, disk-efficient alternative to `composer install`. It uses a content-addressable store (CAS) to deduplicate PHP packages across projects and links them into `vendor/` via reflink, hardlink, or copy.

**Allegro is not a Composer replacement** — it's a Composer accelerator. Composer handles dependency resolution; Allegro handles the file I/O.

## Why Allegro?

| Problem | Allegro's Solution |
|---------|-------------------|
| `vendor/` is 100-200 MB per project | Shared CAS — install once, link everywhere |
| `composer install` re-downloads cached packages | CAS skip — already-stored packages are linked in ~1s |
| 10 projects = 1-2 GB of duplicated files | Hardlink/reflink deduplication across projects |
| Slow CI/CD vendor rebuilds | Warm CAS installs in ~1 second |

## Quick Start

```bash
# Install via Go
go install github.com/allegro-php/allegro/cmd/allegro@latest

# Or build from source
go build -o allegro ./cmd/allegro
# Install dependencies (reads composer.lock, downloads via CAS, links into vendor/)
allegro install

# No lock file? Allegro generates one via Composer automatically
allegro install

# Add a package (delegates resolution to Composer, installs via CAS)
allegro require guzzlehttp/guzzle ^7.0

# Update all dependencies
allegro update

# Production deploy (lock file must exist, no dev deps, no scripts)
allegro install --frozen-lockfile --no-dev --no-scripts
```

## Benchmark

### Speed

Measured on Apple M-series (macOS, APFS). Run `./benchmark/run.sh` to reproduce.

| Scenario | Project | Composer | Allegro | Speedup |
|----------|---------|---------|---------|---------:|
| Warm cache, no vendor | Laravel (106 pkgs) | 4.68s | **1.14s** | **4.1x** |
| Warm cache, no vendor | Koel (194 pkgs) | 8.53s | **2.43s** | **3.5x** |
| Warm cache, no vendor | Matomo (85 pkgs) | 3.90s | **1.69s** | **2.3x** |
| Warm cache, no vendor | Spryker (1574 pkgs) | 106s | **23s** | **4.6x** |
| Noop (vendor up-to-date) | Laravel | 0.41s | **0.03s** | **14x** |
| Noop (vendor up-to-date) | Spryker | 3.96s | **0.05s** | **79x** |

Cold installs are network-bound so both tools perform similarly. The real advantage is on subsequent installs: Allegro skips downloads entirely and links from the local CAS.

### Disk Savings

With hardlinks, multiple projects sharing the same dependencies use a single copy on disk. Run `./benchmark/disk-savings.sh` to reproduce.

| Setup | Disk Usage | Savings |
|-------|-----------|---------|
| 10 × Laravel with Composer | 773 MB | — |
| 10 × Laravel with Allegro (shared CAS) | **84 MB** | **89%** |

The CAS stores each file once by SHA-256 hash. Every project's `vendor/` hardlinks to the same inodes — no duplication.


```
allegro install
  1. Read composer.lock (or generate it via Composer if absent)
  2. Check which packages are already in CAS — skip those
  3. Download missing packages in parallel (8 workers default)
  4. Extract and store in CAS (~/.allegro/store/) by SHA-256 hash
  5. Link into vendor/ via reflink → hardlink → copy fallback
  6. Run composer dumpautoload --optimize
  7. Run Composer scripts (post-install-cmd)
  8. Register project for smart store pruning
```

## Commands

| Command | Description |
|---------|-------------|
| `allegro install` | Install from `composer.lock` |
| `allegro update [pkg...]` | Re-resolve dependencies and install |
| `allegro require <pkg> [constraint]` | Add a package |
| `allegro remove <pkg>` | Remove a package |
| `allegro status` | Show vendor state (with colored diff when outdated) |
| `allegro verify [--fix]` | Check vendor integrity against CAS |
| `allegro store status` | Show CAS statistics and registered projects |
| `allegro store prune [--gc]` | Clean orphaned files (--gc for project-aware cleanup) |
| `allegro config list\|get\|set\|unset\|path` | Manage persistent configuration |
| `allegro version` | Print version |

## Flags

| Flag | Description | Scope |
|------|-------------|-------|
| `--no-dev` | Exclude dev dependencies | install, update, require, remove |
| `--no-scripts` | Skip Composer script execution | install, update, require, remove |
| `--frozen-lockfile` | Error if lock missing/out of sync | install |
| `--force` | Full rebuild, skip incremental | install |
| `--link-strategy` | Force `reflink`, `hardlink`, or `copy` | install |
| `--workers N` | Parallel download workers (1-32) | install |
| `--no-autoload` | Skip `composer dumpautoload` | install |
| `--no-color` | Disable colored output | global |
| `--dry-run` | Show install plan without executing | install |

## Configuration

Precedence: **CLI flag > environment variable > config file > default**

Config file location: `~/.allegro/config.json` (created via `allegro config set`)

| Env Variable | Description |
|-------------|-------------|
| `ALLEGRO_STORE` | Store directory path |
| `ALLEGRO_WORKERS` | Download workers (1-32) |
| `ALLEGRO_LINK_STRATEGY` | Force link strategy |
| `ALLEGRO_NO_DEV` | Exclude dev deps |
| `ALLEGRO_NO_SCRIPTS` | Skip scripts |
| `ALLEGRO_FROZEN_LOCKFILE` | CI lock enforcement |
| `NO_COLOR` | Disable colors ([no-color.org](https://no-color.org) standard) |

## Store Layout

```
~/.allegro/
  allegro.json                 # store metadata (version)
  config.json                  # user configuration (optional)
  projects.json                # project registry (for smart prune)
  store/
    files/ab/abcdef1234...     # content-addressed files (SHA-256)
    packages/vendor/pkg/1.0.json  # package manifests
    tmp/                       # extraction staging (cleaned automatically)
```

## Requirements

- **Go 1.23+** for building
- **Composer >= 2.0** at runtime (for dependency resolution and autoload generation)
- **PHP** (required by Composer)

## Building

```bash
go build -ldflags "-s -w \
  -X main.version=$(git describe --tags --always) \
  -X main.commit=$(git rev-parse --short HEAD) \
  -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o allegro ./cmd/allegro
```

Cross-compilation targets: `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`, `windows/amd64`.

## Running Tests

```bash
go test ./...              # all tests
go test -race ./...        # with race detector
go test -v ./qa/...        # QA test suite (92 tests)
```

## Tested With

Allegro is QA-tested against real-world projects spanning the PHP ecosystem. Every install is verified for file integrity (hash + permissions) and autoloader correctness.

| Project | Type | Packages | Files | Notes |
|---------|------|----------|-------|-------|
| [Spryker Suite](https://github.com/spryker-shop/suite) | Enterprise e-commerce | 1,574 | 81,775 | Largest PHP Composer project |
| [Sylius](https://github.com/Sylius/Sylius) | Symfony e-commerce | 277 | 26,662 | |
| [Mautic](https://github.com/mautic/mautic) | Marketing automation | 255 | 30,029 | Path repo, composer-patches plugin |
| [Magento 2](https://github.com/magento/magento2) | E-commerce monorepo | 214 | 24,623 | 241 root replace entries |
| [Filament](https://github.com/filamentphp/filament) | Laravel admin monorepo | 198 | 21,473 | Path repository |
| [Drupal](https://github.com/drupal/recommended-project) | CMS | 154 | 30,722 | 8 composer-plugins, metapackages |
| [Laravel + Filament](https://github.com/laravel/laravel) | Laravel app | 138 | 15,940 | |
| [Symfony](https://github.com/symfony/symfony) | Framework monorepo | 95 | 5,150 | Path + null dist packages |

**Total: 2,905 packages, 236K files — all verified, zero failures.**

## Architecture

| Package | Purpose |
|---------|---------|
| `internal/cli` | Cobra commands, flags, colored output |
| `internal/parser` | `composer.lock` parsing, platform package filtering |
| `internal/fetcher` | HTTP client, retry policy (3 retries, backoff), worker pool |
| `internal/store` | CAS operations, manifests, archive extraction, GC, project registry |
| `internal/linker` | Reflink/hardlink/copy strategies, flock, vendor state |
| `internal/autoloader` | `installed.json`/`installed.php` generation, bin proxy scripts |
| `internal/orchestrator` | Install pipeline, package diff, parallel linking, verify |
| `internal/config` | Config file read/write with 4-tier precedence |

## License

See [LICENSE](LICENSE).
