# Allegro — pnpm-inspired Package Linker for PHP

Allegro is a fast, disk-efficient alternative to `composer install`. It uses a content-addressable store (CAS) to deduplicate PHP packages across projects and links them into `vendor/` via reflink, hardlink, or copy.

**Allegro is not a Composer replacement** — it's a Composer accelerator. Composer handles dependency resolution; Allegro handles the file I/O.

## Why Allegro?

| Problem | Allegro's Solution |
|---------|-------------------|
| `vendor/` is 100-200 MB per project | Shared CAS — install once, link everywhere |
| `composer install` re-downloads cached packages | CAS skip — already-stored packages are linked in <1s |
| 10 projects = 1-2 GB of duplicated files | Hardlink/reflink deduplication across projects |
| Slow CI/CD vendor rebuilds | Warm CAS installs complete in ~1 second |

## Quick Start

```bash
# Build
go build -o allegro ./cmd/allegro

# Install from composer.lock (like composer install, but faster)
allegro install

# No lock file? Allegro generates one automatically
allegro install  # resolves via Composer, then installs via CAS

# Add a package
allegro require guzzlehttp/guzzle ^7.0

# Update dependencies
allegro update

# Remove a package
allegro remove phpunit/phpunit

# Production deploy
allegro install --frozen-lockfile --no-dev --no-scripts
```

## Benchmark (Laravel, 106 packages)

| Scenario | Composer | Allegro |
|----------|---------|---------|
| Cold install | 14.06s | 14.35s |
| Warm cache, no vendor | 3.90s | **1.12s** |
| Warm cache + vendor (noop) | 0.43s | **<0.1s** |

## How It Works

```
allegro install
  1. Read composer.lock (or generate it via Composer)
  2. Check CAS — skip already-stored packages
  3. Download missing packages in parallel (8 workers)
  4. Store in CAS (~/.allegro/store/) with SHA-256 content hashing
  5. Link into vendor/ via reflink → hardlink → copy fallback
  6. Run composer dumpautoload --optimize
  7. Run post-install-cmd scripts
```

Allegro never downloads packages that are already in the CAS. On the second install (or on any other project with overlapping dependencies), it's just a filesystem link operation.

## Commands

| Command | Description |
|---------|-------------|
| `allegro install` | Install from `composer.lock` |
| `allegro update [pkg...]` | Re-resolve dependencies and install |
| `allegro require <pkg> [constraint]` | Add a package |
| `allegro remove <pkg>` | Remove a package |
| `allegro status` | Show vendor state with colored diff |
| `allegro verify [--fix]` | Verify vendor integrity against CAS |
| `allegro store status` | Show store statistics |
| `allegro store prune [--gc]` | Clean orphaned CAS files |
| `allegro store path` | Print store directory |
| `allegro config list\|get\|set\|unset\|path` | Manage configuration |
| `allegro version` | Print version |

## Flags

| Flag | Description |
|------|-------------|
| `--no-dev` | Exclude dev dependencies |
| `--no-scripts` | Skip Composer script execution |
| `--frozen-lockfile` | Error if lock missing/out of sync (CI mode) |
| `--force` | Full rebuild, skip incremental |
| `--no-autoload` | Skip `composer dumpautoload` |
| `--link-strategy` | Force `reflink`, `hardlink`, or `copy` |
| `--workers N` | Parallel download workers (1-32, default 8) |
| `--no-color` | Disable colored output |
| `--dry-run` | Show what would be installed |

## Configuration

Allegro follows a 4-tier precedence: **CLI flag > env var > config file > default**.

Config file: `~/.allegro/config.json` (created via `allegro config set`).

| Env Var | Description |
|---------|-------------|
| `ALLEGRO_STORE` | Store directory path |
| `ALLEGRO_WORKERS` | Download workers |
| `ALLEGRO_LINK_STRATEGY` | Force link strategy |
| `ALLEGRO_NO_DEV` | Exclude dev deps |
| `ALLEGRO_NO_SCRIPTS` | Skip scripts |
| `ALLEGRO_FROZEN_LOCKFILE` | CI lock enforcement |
| `NO_COLOR` | Disable colors (standard) |

## Store Layout

```
~/.allegro/
  allegro.json              # store metadata
  store/
    files/ab/abcdef...      # content-addressed files (SHA-256)
    packages/vendor/pkg/     # package manifests
    tmp/                     # extraction staging
  config.json               # user configuration
  projects.json             # project registry (for smart prune)
```

## Requirements

- Go 1.23+ (build)
- Composer >= 2.0 (runtime, for dependency resolution and autoload generation)
- PHP (for `composer dumpautoload`)

## Building

```bash
go build -ldflags "-s -w \
  -X main.version=0.2.0 \
  -X main.commit=$(git rev-parse --short HEAD) \
  -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o allegro ./cmd/allegro
```

## Architecture

Written in Go. Key packages:

| Package | Purpose |
|---------|---------|
| `cmd/allegro` | Entry point |
| `internal/cli` | Cobra commands, flags, colored output |
| `internal/parser` | `composer.lock` parsing, platform filtering |
| `internal/fetcher` | HTTP downloads, retry policy, circuit breaker |
| `internal/store` | CAS operations, manifests, extraction, GC |
| `internal/linker` | Reflink/hardlink/copy, flock, vendor state |
| `internal/autoloader` | `installed.json`/`installed.php` generation, bin proxies |
| `internal/orchestrator` | Install pipeline, diff algorithm, parallel linking |
| `internal/config` | Config file read/write/precedence |

## Specs

- [`spec/allegro.md`](spec/allegro.md) — Phase 1 (MVP) specification
- [`spec/phase2.md`](spec/phase2.md) — Phase 2 specification

## License

See [LICENSE](LICENSE).
