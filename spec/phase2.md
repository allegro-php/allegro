# Allegro Phase 2 — Incremental Install & Developer Experience

**Parent spec:** `spec/allegro.md` (Phase 1 MVP — fully implemented)

**Note:** This Phase pulls several items originally scoped to Phase 3 in `allegro.md §16` (global config, composer scripts, smart prune, dependency resolution commands). The Phase 1 spec's §16 phased delivery plan should be updated to reflect this re-scoping.

**Prerequisites:** Composer >= 2.7 is required. Allegro checks the Composer version at startup via `composer --version` and exits with code 5 if the version is below 2.7 (message: `"Composer >= 2.7 required, found {version}"`). Exit code 5 is intentionally reused from Phase 1 — it covers all Composer-related failures (binary not found, version too old, dumpautoload failed). Callers distinguish via stderr message. The `--lock` flag was deprecated in Composer 2.7 (now default behavior); Allegro's internal delegation relies on `--no-install` being sufficient.

## 1. Design Philosophy

Allegro's core mission: **fast package linking + disk savings via CAS**. Everything else delegates to Composer. Allegro is not a full Composer replacement — it is a Composer accelerator that serves as the primary CLI entry point for day-to-day package operations.

Users typically need not run `composer` directly. Allegro wraps the most common Composer commands (`install`, `update`, `require`, `remove`) and accelerates the file I/O portion via its CAS pipeline.

## 2. Features

| # | Feature | Impact |
|---|---------|--------|
| 1 | Incremental install | Noop < 0.1s, partial update < 1s |
| 2 | `--dev` / `--no-dev` flag | Production deploys exclude dev deps |
| 3 | `allegro verify` | Vendor integrity check |
| 4 | Colored status diff | Show what changed before install |
| 5 | Global config file | Persistent settings |
| 6 | Composer script delegation | `post-install-cmd` via Composer subprocess |
| 7 | Smart store prune | Project-aware garbage collection |
| 8 | Dependency resolution commands | `allegro update`, `require`, `remove` |
| 9 | Lock file generation | Work without existing `composer.lock` |
| 10 | Parallel vendor linking | Goroutine pool for file I/O |

## 3. Incremental Install

### 3.1 Problem

Phase 1 rebuilds `vendor/` from scratch every time. Benchmark: noop takes 1.70s vs Composer's 0.43s.

### 3.2 Algorithm

1. Read `vendor/.allegro-state.json` → get stored `lock_hash` and `dev` flag
2. Compute SHA-256 of current `composer.lock` raw bytes
3. Determine current `dev` mode using precedence: `--dev`/`--no-dev` flag > `ALLEGRO_NO_DEV` env var > `no_dev` config key > default (`true`)
4. If lock hash matches AND dev flag matches → **noop**: print "Vendor is up to date", exit 0. All subsequent steps (diff, link, autoload, scripts, state write) are skipped. Target: < 0.1s.
5. If lock hash differs OR dev flag changed → compute package diff:

```go
type PackageDiff struct {
    Added     []Package       // in new lock, not in state
    Removed   []Package       // in state, not in new lock
    Updated   []PackageUpdate // in both, version differs
    Unchanged []Package       // in both, version matches
}

type PackageUpdate struct {
    Name       string
    OldVersion string
    NewVersion string
}
```

6. Apply diff:
   - `added` → download if not in CAS, link into `vendor/`
   - `removed` → delete package directory from `vendor/`
   - `updated` → delete old directory, link new version from CAS
   - `unchanged` → skip
7. Regenerate `installed.json`, `installed.php` (always — they reflect full state). Clear `vendor/bin/` completely and regenerate all proxies from scratch (respecting `--no-dev` per §4.3). This ensures stale proxies from removed packages are deleted.
8. Run `composer dumpautoload --optimize` (unless `--no-autoload`). Append `--no-dev` if active.
9. Update `vendor/.allegro-state.json` — **must happen before flock release** (see §3.3)

--- *flock released here* ---

10. Run Composer scripts (unless `--no-scripts`, see Section 8). Runs **after** flock release.

Comparison key: `package.name` + `package.version`. Name comparison is case-insensitive (Packagist convention).

### 3.3 Locking and Crash Safety

Incremental install acquires the same `.allegro.lock` flock as Phase 1 (per Phase 1 §6.3 step 1) before modifying `vendor/`.

**Flock is held for steps 1-9** of §3.2 (diff → link → autoload → state write). **Flock is released after step 9.** Step 10 (Composer scripts) runs after flock release to avoid blocking concurrent installs.

The state file must be written inside the flock window because it is the basis for the incremental diff — writing it outside the flock would allow a concurrent process to read stale state.

Incremental install modifies `vendor/` in-place — no `vendor.allegro.tmp/` staging. This is a deliberate trade-off: faster than atomic swap, but a crash mid-update leaves `vendor/` partially updated. Recovery: `allegro install --force` triggers a full Phase 1 rebuild with atomic swap.
### 3.4 The `--force` Flag

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--force` | `ALLEGRO_FORCE` | `false` | Skip incremental, full vendor rebuild via Phase 1 atomic swap path |

### 3.5 Performance Targets

| Scenario | Target |
|----------|--------|
| Noop (lock + dev flag unchanged) | < 0.1s |
| Single package update | < 1s (excluding download) |
| 10 packages updated | < 2s (excluding download) |

## 4. `--dev` / `--no-dev` Flag

### 4.1 Behavior

| Flag | Effect |
|------|--------|
| `--dev` | Install `packages` + `packages-dev` (default, same as Phase 1) |
| `--no-dev` | Install only `packages`, exclude `packages-dev` |

These flags are accepted by `install`, `update`, `require`, and `remove` commands.

### 4.2 State File Dev Fields

The `dev_packages` array in `vendor/.allegro-state.json` is **always written** — both when `dev: true` and `dev: false`. It lists all package names from `composer.lock`'s `packages-dev` array. This is required for the incremental diff when switching between dev modes (§4.3).

### 4.3 When `--no-dev` Is Active

1. `packages-dev` excluded from install plan
2. `installed.json` → `"dev": false`, `"dev-package-names": []`
3. `installed.php` → `'dev' => false`, dev packages excluded from `versions` array
4. `vendor/.allegro-state.json` → `"dev": false` (but `dev_packages` still lists all dev package names per §4.2)
5. `vendor/bin/` proxies for dev packages NOT generated
6. `composer dumpautoload --optimize --no-dev`

### 4.4 Switching Between Dev and No-Dev

When the `dev` flag in state file differs from the current flag:
- The diff algorithm reads the `dev_packages` list from state (always populated per §4.2) to identify packages that need to be added or removed
- For `--dev` → `--no-dev`: dev packages appear as `removed` in the diff
- For `--no-dev` → `--dev`: dev packages (from `composer.lock` `packages-dev`) appear as `added`
- This is detected by comparing the `dev` field in state vs current flag (step 3-4 of §3.2)

### 4.5 Environment Variable

`ALLEGRO_NO_DEV=1` equivalent to `--no-dev`. CLI flag > env var > config file > default (per §7.3).

## 5. `allegro verify`

### 5.1 Purpose

Verify `vendor/` integrity against CAS manifests. Detects: missing files, modified files (hash mismatch), permission mismatches.

### 5.2 Command

```
allegro verify [--fix]
```

`allegro verify` does NOT accept `--no-dev` or `--dev`. It always reads the `dev` flag from `vendor/.allegro-state.json` and verifies only the packages that were actually linked. This ensures verify checks the state vendor was actually built in, not a hypothetical different mode.

### 5.3 Algorithm

1. Read `vendor/.allegro-state.json` → get `packages` map (the actually-linked packages), `link_strategy`, and `dev` flag
2. Iterate only the packages present in the `packages` map. Dev packages listed in `dev_packages` but NOT in `packages` (i.e., when `dev: false`) are skipped.
3. For each package, read its CAS manifest from store. If manifest is missing → treat as all files missing (full package re-download needed in `--fix` mode).
4. Determine expected permissions from `link_strategy`: `0644`/`0755` for reflink/copy, `0444`/`0555` for hardlink
5. For each file in manifest:
   a. Check file exists in `vendor/{name}/{path}`
   b. Compute SHA-256 of vendor file
   c. Compare with manifest hash
   d. Check permissions match expected values
6. Report results

### 5.4 Output

```
allegro verify

  Checking 106 packages (4,832 files)...

  ✓ monolog/monolog (42 files) — OK
  ✗ laravel/framework (1,247 files) — 2 issues:
      modified: src/Illuminate/Support/helpers.php
      missing:  src/Illuminate/Foundation/bootstrap.php
  ✓ symfony/console (89 files) — OK

  Summary: 104 OK, 1 failed
  Run `allegro verify --fix` to repair
```

### 5.5 `--fix` Mode

1. Missing files → re-link from CAS. If CAS file also missing → re-download entire package, rebuild manifest, then re-link.
2. Missing manifest → re-download entire package, rebuild manifest, then re-link all files.
3. Modified files → delete and re-link from CAS
4. Permission mismatches → `chmod` to correct value
5. After fixes → run `composer dumpautoload --optimize` (append `--no-dev` if `state.dev == false`)
6. Exit 0 if all issues fixed. Exit 1 if any issue could not be fixed (e.g., re-download failed).

### 5.6 Parallel Verification

Use a goroutine pool (same worker count as `--workers`) to hash files concurrently. SHA-256 computation is CPU-bound; parallelism gives near-linear speedup on multi-core systems.

### 5.7 Exit Codes

| Code | Meaning |
|------|---------|
| 0 | All files OK, or `--fix` repaired all issues |
| 1 | Issues found, or `--fix` could not repair some issues |

## 6. Colored Status Diff

### 6.1 Enhanced `allegro status` Output

When lock hash differs from state OR `dev` flag differs from current mode, show a package-level diff (same condition as the install noop check in §3.2 step 4):

```
allegro status

  Vendor is outdated — 3 changes detected:

    + symfony/mailer 7.2.0           (new)
    ↑ monolog/monolog 3.8.0 → 3.9.0 (updated)
    - phpunit/phpunit 10.0.0         (removed)

  Run `allegro install` to apply changes.
```

All Phase 1 §7.5 status conditions (no vendor, no lock, corrupt state, etc.) remain unchanged. The diff display only appears for the "outdated" condition.

### 6.2 Color Scheme

| Symbol | Color | Meaning |
|--------|-------|---------|
| `+` | Green | Added |
| `↑` | Yellow | Updated |
| `-` | Red | Removed |

### 6.3 Color Disable

Colors are disabled globally for all Allegro output when any of these conditions is true:
- `--no-color` flag is passed
- `NO_COLOR` env var is set (standard [no-color.org](https://no-color.org) convention)
- `no_color: true` in config file (§7.5)
- stdout is not a TTY (piped output)

## 7. Global Config File

### 7.1 Location

`~/.allegro/config.json` — created only on explicit `allegro config set`.

### 7.2 Format

```json
{
  "store_path": "~/.allegro/store",
  "workers": 8,
  "link_strategy": "auto",
  "no_progress": false,
  "no_color": false,
  "composer_path": "",
  "no_dev": false,
  "no_scripts": false,
  "prune_stale_days": 90
}
```

### 7.3 Precedence (Extended)

CLI flag > environment variable > **config file** > default

This extends Phase 1 §10's precedence rule with a new config file tier.

### 7.4 Commands

| Command | Description |
|---------|-------------|
| `allegro config list` | Show all values with source (flag/env/config/default) |
| `allegro config get <key>` | Show single value |
| `allegro config set <key> <value>` | Set value (creates file if needed). Validates at write time — rejects invalid values with error. |
| `allegro config unset <key>` | Remove override |
| `allegro config path` | Print config file path |

### 7.5 Valid Keys

| Key | Type | Valid Values | Default |
|-----|------|-------------|---------|
| `store_path` | string | directory path | `~/.allegro/store` |
| `workers` | integer | 1-32 (decimal string, e.g., `"8"`) | `8` |
| `link_strategy` | string | `auto`, `reflink`, `hardlink`, `copy` | `auto` |
| `no_progress` | boolean | `true`, `false` (case-insensitive) | `false` |
| `no_color` | boolean | `true`, `false` (case-insensitive) | `false` |
| `composer_path` | string | binary path | auto-detect |
| `no_dev` | boolean | `true`, `false` | `false` |
| `no_scripts` | boolean | `true`, `false` | `false` |
| `prune_stale_days` | integer | 1-365 | `90` |

### 7.6 Error Handling

| Condition | Behavior |
|-----------|----------|
| Malformed JSON | Warn to stderr, use defaults |
| Unknown key | Warn to stderr, ignore |
| Invalid value at load time | Warn to stderr, use default for that key |
| Invalid value at `config set` time | Reject with error, do not write |
| File absent | All defaults (not an error) |

## 8. Composer Script Delegation

### 8.1 Design

Allegro does NOT parse or execute scripts. It delegates to Composer via subprocess:

```bash
composer run-script post-install-cmd --no-interaction
```

### 8.2 Supported Events

| Event | When Allegro Triggers It |
|-------|--------------------------|
| `post-install-cmd` | After `allegro install` (full or incremental), unless noop |
| `post-update-cmd` | After `allegro update`, `allegro require`, or `allegro remove` |
| `post-autoload-dump` | Fired automatically by `composer dumpautoload` (already in Phase 1) |

The correct event is chosen based on which command was invoked. This matches Composer's own behavior: `composer install` fires `post-install-cmd`, `composer update/require/remove` fire `post-update-cmd`. All other Composer events (`pre-install-cmd`, `pre-update-cmd`, etc.) are NOT triggered.

### 8.3 Execution Order

The canonical step numbering is defined in §3.2. Script execution fits into that flow as follows:

**Under flock (§3.2 steps 1-9):**
- Steps 1-4: read state, hash lock, determine dev mode, noop check
- Step 5: compute package diff
- Step 6: apply diff (download if needed, add/remove/update packages in vendor)
- Step 7: regenerate installed.json, installed.php, vendor/bin/ proxies
- Step 8: `composer dumpautoload --optimize` (triggers `post-autoload-dump`)
- Step 9: write `vendor/.allegro-state.json`

**After flock release (§3.2 step 10):**
- Step 10: run Composer scripts — `post-install-cmd` for `install`, `post-update-cmd` for `update`/`require`/`remove` **(new in Phase 2)**

On noop (§3.2 step 4), all steps including scripts are skipped.

### 8.4 The `--no-scripts` Flag

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--no-scripts` | `ALLEGRO_NO_SCRIPTS` | `false` | Skip Composer script execution (`post-install-cmd` for install, `post-update-cmd` for update/require/remove) |

Accepted by `install`, `update`, `require`, and `remove` commands.

### 8.5 Failure Handling

If the Composer script (`post-install-cmd` or `post-update-cmd`, depending on command) fails (non-zero exit):
- Print warning to stderr with Composer's output
- Do NOT fail the install — vendor is already built and functional
- State file is written with `scripts_executed: false` (script was invoked but failed)

If script succeeds: `scripts_executed: true`.
If `--no-scripts` is set: `scripts_executed: false` (script was not invoked).

The `scripts_executed` field has the same semantics for both `post-install-cmd` and `post-update-cmd` — it tracks whether the appropriate event script succeeded.

### 8.6 Hardlink Safety Warning

Before running scripts with hardlink strategy, print to stderr:

```
warning: running composer scripts with hardlink strategy may modify
shared CAS files. Consider --link-strategy copy or reflink for safety.
```

After scripts complete with hardlink strategy:
- Suggest running `allegro verify --fix` to detect and repair any CAS corruption
- Do NOT silently run verify — this could be slow and surprising. Let the user decide.

## 9. Smart Store Prune

### 9.1 Project Registry

`~/.allegro/projects.json`:

```json
{
  "projects": [
    {
      "path": "/Users/dev/laravel-app",
      "last_install": "2026-04-06T08:04:30Z",
      "lock_hash": "sha256:abc...",
      "packages": {"monolog/monolog": "3.9.0"}
    }
  ]
}
```

Every command that modifies `vendor/` (`allegro install`, `allegro update`, `allegro require`, `allegro remove`) registers/updates the project in `projects.json` automatically. The `last_install` timestamp is updated on **every invocation** — including noop (lock unchanged) — to prevent false staleness warnings from `prune --gc`. This is a lightweight write that happens even when vendor is not modified.

Writes to `projects.json` are protected by an advisory flock on `~/.allegro/projects.lock` to handle concurrent installs from different projects.

### 9.2 Enhanced Prune

**Default mode** (`allegro store prune`): Phase 1 behavior — delete orphaned CAS files.

**GC mode** (`allegro store prune --gc`):

1. Read `projects.json`
2. Remove projects whose directory **no longer exists** on disk — their CAS entries become prune candidates
3. For projects whose directory still exists but `last_install` is older than `prune_stale_days` (default 90): **warn** but do NOT remove from registry. Their CAS entries are preserved. Message: `"warning: stale project {path} (last installed {date}) — run 'allegro install' in that project to refresh"`
4. Collect package name+version pairs still referenced by remaining (non-removed) projects
5. Delete manifests not in the referenced set
6. Delete orphaned CAS files
7. Report: `"Pruned X manifests, Y files. Freed N MiB. Z stale projects warned."`

This ensures live projects are never silently broken. Only projects whose directory is physically gone are evicted.

`--gc --dry-run`: preview without deleting.

### 9.3 Enhanced `allegro store status`

```
Store: ~/.allegro/store
  Files: 12,459 (348 MiB)
  Manifests: 156
  Projects: 3 (1 stale)
  Most shared: monolog/monolog@3.9.0 (used by 3 projects)
```

## 10. Dependency Resolution & Lock File

### 10.1 Principle

Users typically need not run `composer` directly. Allegro wraps common Composer commands and accelerates the I/O-heavy portions. Composer is used internally as a **resolution engine** — the user types `allegro`, not `composer`.

### 10.2 Commands

| Command | What It Does |
|---------|-------------|
| `allegro install` | Install from `composer.lock`. If lock absent, auto-resolve first (§10.4). |
| `allegro update [pkg...]` | Re-resolve dependencies → update `composer.lock` → install via CAS |
| `allegro require <pkg> [constraint]` | Add package to `composer.json` → resolve → update lock → install |
| `allegro remove <pkg>` | Remove package from `composer.json` → resolve → update lock → install |

All four commands accept `--no-dev`, `--no-scripts`, `--force`, `--no-autoload` flags. The `--frozen-lockfile` flag is **only valid for `install`** — passing it to `update`, `require`, or `remove` produces a warning and is ignored (those commands inherently modify the lock file).

### 10.3 Internal Delegation

All resolution commands delegate to Composer internally:

| Allegro Command | Internal Composer Call |
|-----------------|----------------------|
| `allegro update` | `composer update --no-install --no-scripts --no-interaction` |
| `allegro update monolog/monolog` | `composer update monolog/monolog --no-install --no-scripts --no-interaction` |
| `allegro require monolog/monolog ^3.0` | `composer require monolog/monolog ^3.0 --no-install --no-scripts --no-interaction` |
| `allegro remove phpunit/phpunit` | `composer remove phpunit/phpunit --no-install --no-scripts --no-interaction` |

When `--no-dev` is active, the `--no-dev` flag is forwarded to the Composer delegation call **only for `update`**. **Exceptions:** `--no-dev` is NOT forwarded to `composer require` or `composer remove` — in Composer, these commands interpret `--no-dev` as affecting which section of `composer.json` to target, not which packages to install. Allegro's `--no-dev` flag only affects the linking step (which packages are linked into vendor). Examples:
- `allegro update --no-dev` → `composer update --no-dev --no-install --no-scripts --no-interaction`
- `allegro require guzzle/guzzle --no-dev` → `composer require guzzle/guzzle --no-install --no-scripts --no-interaction` (--no-dev NOT forwarded; Allegro handles dev exclusion at link time)
- `allegro remove phpunit/phpunit --no-dev` → `composer remove phpunit/phpunit --no-install --no-scripts --no-interaction` (--no-dev NOT forwarded)

After Composer writes/updates `composer.lock`, Allegro takes over: CAS download → link → autoload → scripts.

### 10.4 Lock File Auto-Resolution

The following pseudocode shows the complete `allegro install` decision tree:
```
allegro install
  IF --frozen-lockfile:
    (1) composer.lock absent?            → exit 2 "composer.lock not found (--frozen-lockfile is set)"
    (2) state file exists, hash differs? → exit 2 "composer.lock out of sync with vendor"
    (3) state file exists, hash matches, dev mismatch? → exit 2 "vendor dev mode does not match --no-dev/--dev flag"
    (4) state file absent?               → proceed with full install (fresh deploy)
    (5) all checks pass?                 → noop (vendor matches lock)

  IF NOT --frozen-lockfile:
    → composer.lock exists?              → normal install (Phase 1 / incremental)
    → composer.lock absent?
      → composer.json exists?
        YES → run "composer update --no-install --no-scripts --no-interaction"
              → composer.lock now exists → normal install
        NO  → exit 2 "neither composer.lock nor composer.json found"
```

Note: `composer.json` is always required (even with `--frozen-lockfile`) unless `--no-autoload` is set — same as Phase 1 §8.1 step 1.

This means `git clone project && allegro install` always works — even without a committed lock file.

### 10.5 The `--frozen-lockfile` Flag

For CI/production where lock must exist and be in sync:

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--frozen-lockfile` | `ALLEGRO_FROZEN_LOCKFILE` | `false` | Error if `composer.lock` is absent or out of sync with vendor state |

Check order: (1) lock file presence — if absent, exit 2. (2) If state file exists, lock hash match — if differs, exit 2 `"composer.lock out of sync with vendor"`. (3) If state file exists and hashes match, dev flag match — if `--no-dev` was passed but state has `dev: true` (or vice versa), exit 2 `"vendor dev mode does not match --no-dev/--dev flag"`. (4) If state file is absent (fresh clone, never installed), proceed with full install. (5) If all checks pass (hash match + dev match), the noop path applies — `--frozen-lockfile` does NOT suppress the noop. The `--frozen-lockfile` check runs before any auto-resolve or install logic.

### 10.6 Lock File Format

Allegro uses standard `composer.lock` format. No custom lock file. Reasons:
- `composer dumpautoload` reads `composer.lock`
- IDE integrations expect `composer.lock`
- CI tools expect `composer.lock`
- Full ecosystem compatibility

### 10.7 The Complete Allegro Workflow

```bash
# New project (no lock file)
git clone https://github.com/example/project
cd project
allegro install          # auto-resolves → generates lock → installs via CAS

# Add a package
allegro require guzzlehttp/guzzle ^7.0

# Update all dependencies
allegro update

# Update one package
allegro update monolog/monolog

# Remove a dev package
allegro remove phpunit/phpunit

# Production deploy (lock must exist)
allegro install --frozen-lockfile --no-dev --no-scripts
```

## 11. Parallel Vendor Linking

### 11.1 Problem

Phase 1 links files sequentially. For Laravel's 4,832 files, this takes ~1s. With goroutine parallelism, this can be significantly faster.

### 11.2 Design

Use a goroutine worker pool for the linking phase:

1. Collect all directories that need to be created → create them sequentially (avoid `MkdirAll` races)
2. Collect all file link operations: `[]LinkOp{src, dst, executable}`
3. Send to a buffered channel
4. N goroutines (same as `--workers`) consume and execute link operations
5. Each goroutine: `lnk.LinkFile(src, dst)` then `chmod` if needed
6. Collect errors via error channel

### 11.3 Applicability

Parallel linking applies to:
- Full rebuild (Phase 1 path)
- `added` and `updated` packages in incremental install
- `allegro verify --fix` re-linking

NOT parallel:
- Directory creation (sequential to avoid races)
- Manifest reads (sequential, fast)

### 11.4 Performance Target

| Scenario | Phase 1 (sequential) | Phase 2 (parallel, 8 workers) |
|----------|---------------------|-------------------------------|
| Link 4,832 files (Laravel) | ~1.0s | < 0.3s |

## 12. Updated Vendor State File

### 12.1 New Fields

```json
{
  "allegro_version": "0.2.0",
  "schema_version": 2,
  "link_strategy": "reflink",
  "lock_hash": "sha256:...",
  "installed_at": "2026-04-06T08:04:30Z",
  "dev": true,
  "dev_packages": ["phpunit/phpunit", "mockery/mockery"],
  "scripts_executed": true,
  "packages": {
    "monolog/monolog": "3.9.0"
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `schema_version` | integer | State file schema: 1 = Phase 1, 2 = Phase 2 |
| `dev` | boolean | Whether dev deps were installed |
| `dev_packages` | string[] | Names of packages from `packages-dev` (for diff when switching modes) |
| `scripts_executed` | boolean | `true` if the appropriate Composer script (`post-install-cmd` or `post-update-cmd`) succeeded, `false` if failed or skipped |

### 12.2 Backward Compatibility

- Missing `schema_version` → treat as 1
- Missing `dev` field → treat as `true` (Phase 1 always installed dev)
- Missing `dev_packages` → if the current operation would change the `dev` flag (e.g., switching to `--dev` or `--no-dev`), skip incremental diff and fall back to a full Phase 1 rebuild. If the `dev` flag is unchanged, treat as `[]` and proceed normally.
- Missing `scripts_executed` → treat as `false`

## 13. New CLI Summary

### 13.1 New Commands

| Command | Description |
|---------|-------------|
| `allegro update [pkg...]` | Re-resolve dependencies → update lock → install |
| `allegro require <pkg> [constraint]` | Add package → resolve → install |
| `allegro remove <pkg>` | Remove package → resolve → install |
| `allegro verify [--fix]` | Verify/repair vendor integrity |
| `allegro config list\|get\|set\|unset\|path` | Config management |
| `allegro store prune --gc [--dry-run]` | Smart garbage collection |

### 13.2 New Flags

| Flag | Env Var | Scope | Description |
|------|---------|-------|-------------|
| `--force` | `ALLEGRO_FORCE` | `install` | Full rebuild via Phase 1 path |
| `--dev` | — | `install`, `update`, `require`, `remove` | Explicitly install dev deps (default; overrides `ALLEGRO_NO_DEV` / config) |
| `--no-dev` | `ALLEGRO_NO_DEV` | `install`, `update`, `require`, `remove` | Exclude dev deps |
| `--no-scripts` | `ALLEGRO_NO_SCRIPTS` | `install`, `update`, `require`, `remove` | Skip Composer scripts |
| `--no-color` | `NO_COLOR` | global | Disable colors |
| `--frozen-lockfile` | `ALLEGRO_FROZEN_LOCKFILE` | `install` | Error if lock missing/out of sync |
| `--fix` | — | `verify` | Repair issues |
| `--gc` | — | `store prune` | Full GC with project awareness |

## 14. Project Structure Changes

New files added to the Phase 1 structure:

```
internal/
  cli/
    verify.go          # verify command
    config.go          # config subcommands
    update.go          # update command (delegates to composer)
    require.go         # require command (delegates to composer)
    remove.go          # remove command (delegates to composer)
  orchestrator/
    diff.go            # package diff algorithm
    incremental.go     # incremental install logic
    scripts.go         # composer script delegation
    resolve.go         # dependency resolution + lock generation via composer
    parallel.go        # parallel linking worker pool
  store/
    projects.go        # project registry
    gc.go              # garbage collection
  config/
    config.go          # config file read/write/precedence
```

## 15. Testing Strategy

### 15.1 Unit Tests (per feature)

| Feature | Test Focus |
|---------|------------|
| Package diff | Added/removed/updated/unchanged detection, case-insensitive names |
| Incremental install | Selective re-link, directory removal, noop path (hash + dev flag) |
| Dev/no-dev | Flag switching via dev_packages list, installed.json/php field changes |
| Verify | Hash comparison, missing/modified detection, missing manifest, fix mode, parallel hashing |
| Config | Precedence chain (flag > env > config > default), CRUD, write-time validation |
| Project registry | Registration, staleness, cleanup, flock on projects.json |
| Script delegation | Subprocess execution via ComposerRunner interface, failure warning, scripts_executed field |
| Lock generation | Missing lock → composer update --no-install, frozen-lockfile error |
| Parallel linking | Goroutine pool, error collection, directory pre-creation |
| Composer version | Version check >= 2.7, error on older versions |

### 15.2 Integration Tests

1. **Noop** — install twice, second < 0.1s
2. **Single package update** — modify lock, verify only that package changes
3. **Dev/no-dev switch** — install `--dev`, then `--no-dev`, verify removal; then back
4. **Verify + fix** — corrupt a file, verify detects, fix repairs
5. **Config persistence** — set value, verify across runs
6. **Smart prune** — 2 projects, delete one, `--gc`, verify cleanup
7. **No lock file** — project with only composer.json, verify lock generated then installed
8. **Frozen lockfile** — missing lock with `--frozen-lockfile`, verify exit 2
9. **Script execution** — project with post-install-cmd, verify it runs
10. **Parallel linking** — large project, verify correct output with parallel workers
11. **allegro update** — run update, verify lock changes, verify vendor updated
12. **allegro require** — add a new package, verify composer.json + lock + vendor updated
13. **allegro remove** — remove a package, verify composer.json + lock + vendor updated
14. **allegro update single pkg** — update one package, verify only that package changes

## 16. Migration from Phase 1

Fully backward-compatible:
- Phase 1 state files handled gracefully (missing `schema_version`/`dev`/`dev_packages`/`scripts_executed` fields)
- Phase 1 store layout unchanged
- `allegro install --force` always falls back to Phase 1 full rebuild with atomic swap
- All new commands and flags are additive — no existing behavior changes
- Existing projects auto-registered in `projects.json` on next `allegro install`
- Crash safety trade-off: incremental mode has weaker atomicity than Phase 1's full rebuild. Users can always fall back to `--force` for atomic behavior.
