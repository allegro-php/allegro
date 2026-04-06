# CLAUDE.md — Agent Context for Allegro

This file provides context for AI agents working on this codebase. Read this first.

## What is Allegro?

A Go CLI tool that accelerates PHP Composer installs using a content-addressable store (CAS). Think "pnpm for PHP". Allegro downloads packages once, stores them by SHA-256 hash, and links them into `vendor/` via reflink/hardlink/copy. Composer is used internally as a resolution engine only (via `--no-install` flag).

## Current State

### Phase 1 (MVP) — COMPLETE
- `allegro install` from `composer.lock`
- CAS with SHA-256, reflink → hardlink → copy fallback
- Parallel downloads (8 workers), retry policy (3 retries, backoff)
- `composer dumpautoload` delegation
- `allegro status`, `store status/prune/path`, `version`
- Bin proxy generation (PHP no-shebang, PHP with-shebang/BinProxyWrapper, shell)
- All with unit tests and QA tests

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

### Phase 3-4 — NOT STARTED
See `spec/allegro.md` §16 for the phased delivery plan. Phase 3 covers private repos, auth, workspace support. Phase 4 covers IDE plugins and Docker optimization.

## Project Structure

```
allegro/
  cmd/allegro/main.go          # entry point, ldflags vars
  internal/
    cli/                        # cobra commands, flags, colored output
    parser/                     # composer.lock parsing, lock hash
    fetcher/                    # HTTP client, retry, worker pool
    store/                      # CAS, manifests, extraction, GC, projects registry
    linker/                     # reflink/hardlink/copy, flock, vendor state
    autoloader/                 # installed.json/php, bin proxies, composer detection
    orchestrator/               # install pipeline, diff, parallel linking, verify
    config/                     # config file read/write/precedence
    platform/                   # OS detection
  qa/                           # comprehensive QA tests (Phase 1 + Phase 2)
  spec/
    allegro.md                  # Phase 1 spec (963 lines)
    phase2.md                   # Phase 2 spec (679 lines)
    allegro.tasks.json          # Phase 1 tp task file (165 tasks, all done)
    phase2.tasks.json           # Phase 2 tp task file (222 tasks, all done)
```

## Key Design Decisions

1. **Composer is never replaced** — Allegro delegates resolution (`composer update --no-install`), autoloading (`composer dumpautoload`), and scripts (`composer run-script`). Allegro only replaces the download + extract + copy step.

2. **`--no-dev` not forwarded to `composer require/remove`** — In Composer, these commands interpret `--no-dev` as affecting which `composer.json` section to target. Allegro's `--no-dev` only affects the linking step. Only `composer update` receives `--no-dev`.

3. **Lock file is always `composer.lock`** — No custom format. Full ecosystem compatibility.

4. **Hardlink strategy = read-only vendor** — CAS files are `0444`/`0555`. Hardlinked vendor files share the inode. Reflink is preferred for writable vendor.

5. **State file written inside flock** — `vendor/.allegro-state.json` must be written before flock release to prevent concurrent processes from reading stale state.

6. **Flock accepts context** — `AcquireLock(ctx, dir)` respects cancellation.

7. **GC aborts on corrupt manifest** — Rather than silently deleting CAS files that might be referenced, GC fails fast.

8. **WriteFileAtomic everywhere** — All persistent files use temp + fsync + chmod + rename.

## How to Build and Test

```bash
go build ./cmd/allegro           # build binary
go test ./...                    # run all tests
go test -race ./...              # with race detector
go test -v ./qa/...              # QA tests only
```

## How to Run Specs Through tp

```bash
tp use spec/phase2.tasks.json    # set active task file
tp status                        # see progress
tp plan --minimal                # get execution plan
tp next                          # get next task
tp done <id> "reason"            # close a task
```

## How to Audit

```bash
tp audit spec/phase2.md --affected-files internal/...
# Or run manual audit agents checking code against spec
```

## Conventions

- **TDD**: write failing test first, then implement
- **Atomic commits**: one logical change per commit
- **No `os.Exit` in library code** — only in CLI command handlers (known pattern)
- **All errors checked**: every `os.Remove`, `os.Chmod`, `json.Unmarshal`, `filepath.Walk` callback error is either returned or logged
- **Platform guards**: `//go:build !windows` on files using `syscall.Flock`, with `_windows.go` stubs
- **Config precedence**: CLI flag > env var > config file > default (everywhere)

## Important Files to Read First

1. `spec/allegro.md` §4 (Architecture), §8 (Install Pipeline)
2. `spec/phase2.md` §3 (Incremental Install), §10 (Dependency Resolution)
3. `internal/orchestrator/orchestrator.go` — the main install pipeline
4. `internal/cli/install.go` — the install command with noop/frozen-lockfile/auto-resolve
5. `internal/store/store.go` — CAS operations
6. `internal/orchestrator/diff.go` — package diff algorithm

## Common Pitfalls

- `composer.lock` `dist.shasum` is **SHA-1** (not SHA-256). CAS uses SHA-256 internally.
- `MergePackages` merges both `packages` and `packages-dev`. When `--no-dev`, use `FilterInstallable(lock.Packages)` only.
- `VendorState.Dev` has `omitempty` removed — `false` must be persisted. Same for `ScriptsExecuted`.
- `ReadRegistry` returns error (not empty registry) on corrupt JSON — this prevents GC from wiping the store.
- `ParallelLink` uses labeled `break sendLoop` — plain `break` inside `select` only exits the select.
