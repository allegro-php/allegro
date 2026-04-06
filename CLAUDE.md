# CLAUDE.md — Agent Context for Allegro

This file provides context for AI agents working on this codebase. Read this first.

## What is Allegro?

A Go CLI tool that accelerates PHP Composer installs using a content-addressable store (CAS). Allegro downloads packages once, stores them by SHA-256 hash, and links them into `vendor/` via reflink/hardlink/copy. Composer is used internally as a resolution engine only (via `--no-install` flag).

**Key metrics:** 3.5–4.7x faster than Composer on warm cache, 89% disk savings across shared projects, 13x faster noop detection.

## Current State

### Phase 1 (MVP) — COMPLETE
- `allegro install` from `composer.lock`
- CAS with SHA-256, reflink → hardlink → copy fallback
- Parallel downloads (8 workers), retry policy (3 retries, backoff)
- `composer dumpautoload` delegation
- `allegro status`, `store status/prune/path`, `version`
- Bin proxy generation (PHP no-shebang, PHP with-shebang/BinProxyWrapper, shell)

### Phase 2 (Incremental & DX) — COMPLETE
- Incremental install with noop detection (hash + dev flag check)
- `--dev` / `--no-dev` flag with package filtering
- `allegro verify [--fix]` — vendor integrity check
- Colored status diff (green/yellow/red)
- Global config file (`~/.allegro/config.json`) with 4-tier precedence
- Composer script delegation (`post-install-cmd`, `post-update-cmd`)
- Smart store prune with project registry (`projects.json`)
- `allegro update/require/remove` — Composer delegation commands
- Lock file auto-resolution (no lock → `composer update --no-install`)
- `--frozen-lockfile` for CI
- Parallel vendor linking (goroutine pool)
- All with extensive audits (24 rounds) and QA tests (92 tests)

### Post-Phase 2 Fixes (after real-world testing)
- **InstalledVersions.php** embedded in binary and written to `vendor/composer/` (Composer runtime class)
- **`installed.json` `require` field** — needed by Composer plugin manager for `composer-plugin-api` verification
- **`installed.php` replaced/provided** — virtual packages (e.g. `illuminate/*` from `laravel/framework`) now emitted so `InstalledVersions::isInstalled()` works correctly
- **`installed.php` root package** entry added to `versions` array
- **Composer-plugin packages** copy-linked (not hardlinked) so plugins can modify their own files at runtime (e.g. `phpstan/extension-installer` writes `GeneratedConfig.php`)
- **Parallel linking in `buildVendorTree`** — was sequential, now uses `ParallelLink` worker pool
- **Per-file fsync removed** from `CopyLinker` — vendor files are in temp dir with atomic swap, fsync was pure overhead

### Phase 3-4 — NOT STARTED
See `spec/allegro.md` §16 for the phased delivery plan. Phase 3 covers private repos, auth, workspace support. Phase 4 covers IDE plugins and Docker optimization.

### Known Limitations
- **`composer/installers`** — packages with custom install paths (WordPress, Drupal, CraftCMS) are not supported; everything goes to `vendor/`
- **`symfony/flex`** — install-time recipe execution not supported (Flex hooks into Composer's install events, not post-install-cmd)
- **Empty directories** — CAS is file-based, empty dirs from archives are not preserved (rarely matters)

## Project Structure

```
allegro/
  cmd/allegro/main.go          # entry point, ldflags vars (version, commit, buildDate)
  internal/
    cli/                        # cobra commands, flags, colored output
    parser/                     # composer.lock parsing, lock hash, platform detection
    fetcher/                    # HTTP client, retry, worker pool
    store/                      # CAS, manifests, extraction, GC, projects registry
    linker/                     # reflink/hardlink/copy, flock, vendor state
    autoloader/                 # installed.json/php, InstalledVersions.php (embedded), bin proxies
    orchestrator/               # install pipeline, diff, parallel linking, verify
    config/                     # config file read/write/precedence
    platform/                   # OS detection
  qa/                           # comprehensive QA tests (Phase 1 + Phase 2)
  benchmark/
    run.sh                      # speed benchmark (3 projects × 3 scenarios)
    disk-savings.sh             # CAS dedup benchmark (N × same project)
  spec/
    allegro.md                  # Phase 1 spec
    phase2.md                   # Phase 2 spec
  .goreleaser.yml               # GoReleaser config (6 platform targets)
  .github/workflows/
    ci.yml                      # test + race detector on push/PR
    release.yml                 # GoReleaser on tag push (v*)
```

## Key Design Decisions

1. **Composer is never replaced** — Allegro delegates resolution (`composer update --no-install`), autoloading (`composer dumpautoload`), and scripts (`composer run-script`). Allegro only replaces the download + extract + link step.

2. **`--no-dev` not forwarded to `composer require/remove`** — In Composer, these commands interpret `--no-dev` as affecting which `composer.json` section to target. Allegro's `--no-dev` only affects the linking step. Only `composer update` receives `--no-dev`.

3. **Lock file is always `composer.lock`** — No custom format. Full ecosystem compatibility.

4. **Hardlink strategy = read-only vendor** — CAS files are `0444`/`0555`. Hardlinked vendor files share the inode. Exception: `composer-plugin` type packages are copy-linked (writable) so plugins can modify their own files.

5. **State file written inside flock** — `vendor/.allegro-state.json` must be written before flock release to prevent concurrent processes from reading stale state.

6. **Flock accepts context** — `AcquireLock(ctx, dir)` respects cancellation.

7. **GC aborts on corrupt manifest** — Rather than silently deleting CAS files that might be referenced, GC fails fast.

8. **WriteFileAtomic everywhere** — All persistent files use temp + fsync + chmod + rename.

9. **`installed.php` must include replaced/provided** — Virtual packages from `replace`/`provide` fields are emitted so `InstalledVersions::isInstalled()` works. Platform packages (`ext-*`, `php`, `lib-*`) are filtered out.

10. **`InstalledVersions.php` is embedded** — Go `embed` directive includes the Composer runtime class in the binary. Written to `vendor/composer/` during install.

## How to Build, Test, and Release

```bash
go build -o allegro ./cmd/allegro  # build binary
go test ./...                      # run all tests
go test -race ./...                # with race detector
go test -v ./qa/...                # QA tests only

GOOS=windows go build ./...        # cross-compile check (also linux, darwin)

# Release (triggers GoReleaser via GitHub Actions)
git tag v0.x.y
git push origin v0.x.y
```

## Benchmarks

```bash
./benchmark/run.sh                 # speed: Allegro vs Composer (Laravel, Koel, Matomo)
./benchmark/disk-savings.sh        # disk: N projects with shared CAS vs separate vendors
```

## Conventions

- **TDD**: write failing test first, then implement
- **Atomic commits**: one logical change per commit (conventional commit messages)
- **No `os.Exit` in library code** — only in CLI command handlers
- **All errors checked**: every `os.Remove`, `os.Chmod`, `json.Unmarshal`, `filepath.Walk` callback error is either returned or logged
- **Platform guards**: `//go:build !windows` on files using `syscall.Flock`, with `_windows.go` stubs. Shared types go in platform-independent files (e.g. `projects_types.go`)
- **Config precedence**: CLI flag > env var > config file > default (everywhere)

## Important Files to Read First

1. `internal/orchestrator/orchestrator.go` — the main install pipeline
2. `internal/cli/install.go` — the install command with noop/frozen-lockfile/auto-resolve
3. `internal/autoloader/installed.go` — installed.json/php generation (including replaced/provided)
4. `internal/store/store.go` — CAS operations
5. `internal/orchestrator/parallel_link.go` — parallel vendor linking
6. `internal/linker/detect.go` — link strategy detection and fallback chain

## Common Pitfalls

- `composer.lock` `dist.shasum` is **SHA-1** (not SHA-256). CAS uses SHA-256 internally.
- `MergePackages` merges both `packages` and `packages-dev`. When `--no-dev`, use `FilterInstallable(lock.Packages)` only.
- `VendorState.Dev` has `omitempty` removed — `false` must be persisted. Same for `ScriptsExecuted`.
- `ReadRegistry` returns error (not empty registry) on corrupt JSON — this prevents GC from wiping the store.
- `ParallelLink` uses labeled `break sendLoop` — plain `break` inside `select` only exits the select.
- `composer-plugin` packages must be copy-linked, not hardlinked — plugins modify their own files (e.g. `GeneratedConfig.php`).
- `installed.php` must include `replaced`/`provided` virtual packages — without them, `InstalledVersions::isInstalled('illuminate/contracts')` returns false.
- Platform packages (`ext-*`, `php`, `lib-*`) must be filtered from replaced/provided entries in `installed.php`.
- Shared types used across `//go:build` files must live in platform-independent files (e.g. `projects_types.go`), not in `_windows.go` or `!windows` guarded files.
