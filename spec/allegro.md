# Allegro — pnpm-inspired Package Linker for PHP

## 1. Problem Statement

PHP's Composer copies every dependency file into each project's `vendor/` directory. For organizations running multiple PHP projects with overlapping dependencies, this leads to:

1. Massive disk waste — a typical Laravel project's `vendor/` is ~100-200 MB; 10 projects = 1-2 GB of duplicated files
2. Slow installs — every `composer install` downloads and extracts packages even when identical versions exist locally
3. Slow CI/CD — Docker images bloat because each pipeline stage copies full vendor directories

pnpm solved this for Node.js with content-addressable storage and hardlinks. Allegro brings the same approach to PHP.

## 2. Goals

1. **Zero codebase changes** — existing `composer.json`, `composer.lock`, tests, CI, IDE integrations must work without modification
2. **Identical `vendor/` layout** — the resulting directory must be indistinguishable from Composer's output from PHP's perspective (autoloading, `__DIR__`, file reads, file permissions)
3. **Disk savings** — shared content-addressable store (CAS) across projects, linked via reflink/hardlink
4. **Fast installs** — parallel downloads, skip already-cached packages
5. **Drop-in usage** — `allegro install` replaces `composer install`; no other workflow changes

## 3. Non-Goals

1. Replacing Composer's dependency resolver — Allegro reads `composer.lock`, it does not resolve dependencies
2. Replacing Packagist — Allegro uses Packagist as its package source
3. Supporting `composer.json`-only installs (no lock file) in MVP
4. Composer plugin system compatibility in MVP
5. Private repository authentication in MVP (Satis, GitLab, etc.)
6. Incremental/differential installs in MVP — every install is a full rebuild of `vendor/`
7. Local path-type dependencies (`dist.type: "path"`) in MVP
8. `--dev` / `--no-dev` filtering in MVP — both `packages` and `packages-dev` are always installed

## 4. Architecture Overview

```
┌─────────────────────────────────────────────────────┐
│  CLI (cobra)                                        │
│  allegro install | allegro status | allegro store    │
└────────────┬────────────────────────────────────────┘
             │
┌────────────▼────────────────────────────────────────┐
│  Orchestrator                                        │
│  Reads composer.lock → resolves download plan →      │
│  fetches → stores → links → dumps autoload           │
└──┬──────────┬──────────┬──────────┬─────────────────┘
   │          │          │          │
┌──▼───┐ ┌───▼───┐ ┌───▼────┐ ┌──▼──────────┐
│Parser│ │Fetcher│ │  Store  │ │   Linker    │
│      │ │       │ │  (CAS)  │ │(reflink/    │
│      │ │       │ │         │ │ hardlink/   │
│      │ │       │ │         │ │ copy)       │
└──────┘ └───────┘ └─────────┘ └─────────────┘
```

### 4.1 Component Responsibilities

| Component | Responsibility |
|-----------|---------------|
| CLI | Parses commands and flags, displays progress, calls Orchestrator |
| Parser | Reads and validates `composer.lock`, extracts package list with versions, dist URLs, and hashes |
| Fetcher | Downloads package zip/tar archives from Packagist/GitHub in parallel using a worker pool |
| Store | Manages the content-addressable store: extracts archives, computes file hashes, stores deduplicated files |
| Linker | Creates `vendor/` directory structure using reflink → hardlink → copy fallback chain |
| Orchestrator | Coordinates the full install pipeline: parse → diff → fetch → store → link → autoload |

## 5. Content-Addressable Store (CAS)

### 5.1 Store Location

Default: `~/.allegro/store/`

Configurable via (in precedence order):
1. `--store-path` CLI flag (highest)
2. `ALLEGRO_STORE` environment variable
3. Default `~/.allegro/store` (lowest)

See Section 10 for the general configuration precedence rule (CLI flag > env var > default).

### 5.2 Store Layout

```
~/.allegro/
  store/
    files/
      ab/
        abcdef1234567890...    # regular file (SHA-256 of content)
      cd/
        cdef9876543210...      # another file
    packages/
      monolog/monolog/
        3.9.0.json             # package metadata (file manifest)
      laravel/framework/
        11.0.0.json
    tmp/                       # atomic extraction staging area
  allegro.json                 # store-level config (version, stats)
```

### 5.3 File Storage Rules

1. Each file is stored by its SHA-256 content hash
2. Hash is hex-encoded, first 2 characters form the shard directory
3. **CAS files** are stored with permissions `0444` (non-executable) or `0555` (executable) to prevent accidental mutation of the shared store. Permissions are set with an explicit `os.Chmod` call after file creation, not relying on umask.
4. **Vendor files** use Composer-compatible permissions: `0644` (non-executable) or `0755` (executable) — this ensures Goal 2 compatibility (identical to Composer's output). Permissions are set explicitly after reflink or copy operations; for hardlinks, see Section 6.3 step 3.
5. The manifest's `executable` flag determines which permission set is used during both CAS storage and vendor linking
6. Only individual files are stored in CAS — directory structure is recreated during linking
7. The `sha256:` prefix in manifest JSON fields is for readability; CAS filenames are bare hex-encoded hashes

### 5.4 Package Metadata (Manifest)

Each package version gets a manifest JSON file:

```json
{
  "name": "monolog/monolog",
  "version": "3.9.0",
  "dist_hash": "sha256:abc123...",
  "files": [
    {
      "path": "src/Logger.php",
      "hash": "sha256:abcdef1234...",
      "size": 12345,
      "executable": false
    },
    {
      "path": "bin/console",
      "hash": "sha256:fedcba9876...",
      "size": 890,
      "executable": true
    }
  ],
  "stored_at": "2026-04-05T20:00:00Z"
}
```

### 5.5 Store-Level Metadata (`allegro.json`)

The file `~/.allegro/allegro.json` is created on first use:

```json
{
  "store_version": 1,
  "created_at": "2026-04-05T20:00:00Z"
}
```

`store_version` enables future format migrations. This file is checked on startup (after store path resolution from CLI flags and environment variables, but before any other store operations); if missing, it is created. If `store_version` is higher than the binary supports, Allegro exits with code 1 and the message: `"store version %d is newer than this binary supports (max %d); upgrade allegro"`.

### 5.6 Atomic Storage Operations

1. Ensure the store directory tree exists (`~/.allegro/store/files/`, `~/.allegro/store/packages/`, `~/.allegro/store/tmp/`) — create with `MkdirAll` if missing (first run).
2. Extract archive to `~/.allegro/store/tmp/tmp-{pid}-{random}/` where `{pid}` is the current process ID and `{random}` is a 16-byte hex string from `crypto/rand`. This naming convention enables safe cleanup (see Section 6.3 step 2). The temp directory is always within the store, guaranteeing same-filesystem atomic renames. If the temp directory cannot be created (e.g., disk full), exit 4.
3. Compute SHA-256 hash for each extracted file.
4. Create the shard directory `files/{first2chars}/` with `MkdirAll` if it does not exist.
5. If the target hash path already exists, skip (file is already stored) — this is a performance optimization checked before the rename.
6. Move (rename) each file to its content-addressed path — atomic on same filesystem. A concurrent rename to the same CAS path is safe because content is identical by definition (same SHA-256), and `rename(2)` is atomic on POSIX systems. No lock is required.
7. Symlinks and special file types (device files, FIFOs, sockets, character devices) within archives are skipped with a warning — only regular files and directories are processed (security: prevents path traversal via symlinks, avoids special device issues). Hard links in tar archives are extracted as independent file copies and stored as separate CAS entries by their content hash.
8. **Top-level directory stripping**: if every file path in the archive begins with the same single directory prefix and no files exist at the root level, strip that common prefix so package files are rooted directly (e.g., `src/Logger.php` not `monolog-monolog-abc123/src/Logger.php`). If there are multiple top-level entries or a mix of files and directories at the root, do not strip. An empty archive (no files after extraction) is an extraction error — exit 1.
9. Write package manifest JSON atomically (write to temp file in same directory, then rename). If two processes race to write the same manifest, last-write-wins is safe since content is identical for the same version.
10. Remove temp directory.

## 6. Linking Strategy

### 6.1 Fallback Chain

The linker uses this priority order:

| Priority | Method | Condition | Syscall |
|----------|--------|-----------|------------|
| 1 | Reflink (CoW) | APFS, Btrfs, XFS | `clonefile` (macOS), `FICLONE` ioctl (Linux) |
| 2 | Hardlink | Same filesystem, not CoW-capable | `linkat` / `link` |
| 3 | Copy | Cross-filesystem or fallback | `read`+`write` |

**Windows:** On Windows, the reflink tier is not supported (no `clonefile` or `FICLONE`). The probe will fail reflink and fall through to hardlink or copy. A `reflink_windows.go` stub returning "unsupported" is required for cross-compilation.

### 6.2 Detection Logic

At startup, Allegro performs a one-time probe. **When `--link-strategy` or `ALLEGRO_LINK_STRATEGY` is set, the probe is skipped entirely** and the forced strategy is used directly (no filesystem validation).

**Prerequisite:** Ensure the store directory exists (create if needed via Section 5.6 step 1) before running the probe.

1. Create a temp file in `~/.allegro/store/tmp/` (consistent with other temp file locations)
2. Create a temp directory in the project root (the directory containing `composer.lock`, same filesystem as where `vendor/` will be created). Use a name like `.allegro-probe-{random}` to avoid collisions with other temp directories. If the temp directory cannot be created in the project root (e.g., permission denied), fall back to `copy` strategy with a warning — do not error.
3. Attempt reflink from store temp file to project temp dir
4. If reflink succeeds → use reflink for all files
5. If reflink fails, attempt hardlink to project temp dir
6. If hardlink succeeds → use hardlink for all files
7. If hardlink fails → use copy
8. Clean up both temp files. Probe cleanup failures are non-fatal (log warning, continue).

The detected strategy is cached for the session (not persisted — filesystem conditions may change).

### 6.3 Vendor Directory Construction

In MVP, every install is a full rebuild with atomic replacement:

1. **Acquire lock** — acquire advisory lock via `flock` on `.allegro.lock` in the project root (not inside `vendor/`). The lock file is created with `O_CREAT` if it does not exist. If the lock file cannot be created (e.g., read-only directory, disk full), exit 4 with a descriptive error. If already locked by another process, wait up to 30s, then exit 1. `flock` auto-releases on process exit, so stale locks are not possible. The lock file is not deleted after release (avoids TOCTOU race with another waiting process).
2. **Stale cleanup** — if `vendor.allegro.old/` or `vendor.allegro.tmp/` exist (from a previous crashed run), delete them before proceeding (do not attempt to restore — the previous state may be incomplete; a full re-install is the safe path). Also clean up any leftover temp directories **owned by this process** in `~/.allegro/store/tmp/`: temp directories use a process-unique prefix `tmp-{pid}-{random}/`, so only entries matching the current PID (from a previous crash of the same process) or entries older than 1 hour (from any crashed process) are removed. This avoids deleting temp directories actively used by concurrent installs of other projects.
3. **Build vendor tree** in a temporary directory (`vendor.allegro.tmp/` in the project root):
   - The directory path for each package is derived by splitting `package.name` on `/` — e.g., `monolog/monolog` → `monolog/monolog/`. Packages without a `/` in their name use `{name}/` directly.
   - Create directory tree: `{vendor-name}/{package-name}/{subdirs...}`
   - Link (reflink/hardlink/copy) each file from CAS into its path
   - **Permissions for vendor files**: For reflinks and copies, explicitly `chmod` to `0644` (non-executable) or `0755` (executable) after linking — matching Composer's default permissions. For hardlinks, since they share an inode with CAS (which uses `0444`/`0555`), vendor files will be read-only. This is a known trade-off documented in Section 17.1; reflink is the preferred strategy for full Composer compatibility.
   - If a CAS file referenced by a manifest is missing during linking, re-download that package (subject to the same retry policy as Section 11.2) and update the manifest before retrying the link. If the re-download fails after all retries are exhausted, exit 3.
4. Generate `vendor.allegro.tmp/composer/installed.json` and `installed.php` from `composer.lock` metadata (see Section 9.4)
5. Generate `vendor.allegro.tmp/bin/` proxy scripts for packages declaring `bin` entries (see Section 6.5)
6. **Atomic swap**: rename existing `vendor/` → `vendor.allegro.old/`, then rename `vendor.allegro.tmp/` → `vendor/`, then delete `vendor.allegro.old/`. Both `vendor.allegro.tmp/` and `vendor/` reside in the project root (same filesystem), guaranteeing atomic renames. **Crash recovery**: if the process crashes between the first and second rename, the project is left with no `vendor/` but a `vendor.allegro.old/` directory. Step 2 of the next `allegro install` run will delete it and perform a full re-install. If the user needs to restore the previous vendor state *before* running `allegro install` again, they can manually rename `vendor.allegro.old/` back to `vendor/` — but this must be done before the next install run, which deletes stale directories unconditionally in step 2.
7. Release lock

**Note:** Autoload generation (`composer dumpautoload`) is handled by the Orchestrator as a separate pipeline step after vendor construction (see Section 8.1 step 8).

### 6.4 Vendor State File

After a successful install (including autoload generation), Allegro writes `vendor/.allegro-state.json`:

```json
{
  "allegro_version": "0.1.0",
  "link_strategy": "reflink",
  "lock_hash": "sha256:...",
  "installed_at": "2026-04-05T20:00:00Z",
  "packages": {
    "monolog/monolog": "3.9.0",
    "laravel/framework": "11.0.0"
  }
}
```

The `lock_hash` is the SHA-256 of the raw file bytes of `composer.lock` as read from disk (no JSON normalization or whitespace changes). The `installed_at` timestamp uses UTC in ISO 8601 format. The `packages` map contains only successfully installed packages — packages that were skipped (due to `dist: null` or `dist.type: path`) are excluded.

In MVP this is informational (used by `allegro status`). In Phase 2, it enables incremental installs by comparing `lock_hash` and package versions against the current `composer.lock`.

### 6.5 Vendor Bin Proxy Scripts

`composer dumpautoload` does **not** generate `vendor/bin/` entries — only `composer install/update` does. Allegro must generate these itself, matching Composer's exact format.

**Note:** All paths embedded in proxy scripts use `__DIR__` which is a PHP runtime construct resolved at `include`-time, not at file-write-time. Since proxy scripts are written into `vendor.allegro.tmp/bin/` and then renamed to `vendor/bin/` via atomic swap, `__DIR__` correctly resolves to the final `vendor/bin/` path at runtime.

For each package in `composer.lock` that has a `bin` field:

1. Read the `bin` array from the package entry (e.g., `["bin/phpunit"]`)
2. Detect whether the target is a PHP file: read first 500 bytes and apply this Go RE2 regex: `(?s)^(\#!.*?\r?\n)?[\r\n\t ]*<\?php`. If `<?php` is not found within the first 500 bytes, treat the file as non-PHP. Files starting with a UTF-8 BOM (`\xEF\xBB\xBF`) before `<?php` are treated as PHP (strip BOM before matching).
   - **PHP without shebang**: first non-whitespace content is `<?php` (no `#!` line)
   - **PHP with shebang**: file starts with any `#!` line (e.g., `#!/usr/bin/env php`, `#!/usr/bin/env php8.2`, `#!/usr/bin/php`) followed by `<?php`
   - **Non-PHP**: anything else (shell scripts, Python scripts, etc.)
3. Generate the matching proxy type in `vendor/bin/{basename}`:

**PHP target without shebang** (first non-whitespace content is `<?php`):

```php
#!/usr/bin/env php
<?php

/**
 * Proxy PHP file generated by Allegro
 *
 * @generated
 */

namespace Composer;

$GLOBALS['_composer_bin_dir'] = __DIR__;
$GLOBALS['_composer_autoload_path'] = __DIR__ . '/..'.'/autoload.php';

return include __DIR__ . '/..'.'/vendor-name/package-name/bin/tool';
```

**PHP target with shebang** (starts with `#!` line):

```php
#!/usr/bin/env php
<?php

/**
 * Proxy PHP file generated by Allegro
 *
 * @generated
 */

namespace Composer;

if (PHP_VERSION_ID < 80000) {
    if (!class_exists('Composer\BinProxyWrapper')) {
        /**
         * @internal
         */
        final class BinProxyWrapper
        {
            private $handle;
            private $position = 0;
            private $content;

            public function stream_open($path, $mode, $options, &$opened_path)
            {
                $this->content = file_get_contents(substr($path, 17)); // 17 = strlen('phpvfscomposer://')
                if ($this->content === false) {
                    return false;
                }
                // Strip shebang line
                if (strpos($this->content, '#!') === 0) {
                    $newlinePos = strpos($this->content, "\n");
                    if ($newlinePos !== false) {
                        $this->content = substr($this->content, $newlinePos + 1);
                    }
                }
                return true;
            }

            public function stream_read($count)
            {
                $ret = substr($this->content, $this->position, $count);
                $this->position += strlen($ret);
                return $ret;
            }

            public function stream_tell()
            {
                return $this->position;
            }

            public function stream_eof()
            {
                return $this->position >= strlen($this->content);
            }

            public function stream_seek($offset, $whence)
            {
                switch ($whence) {
                    case SEEK_SET:
                        $this->position = $offset;
                        return true;
                    case SEEK_CUR:
                        $this->position += $offset;
                        return true;
                    case SEEK_END:
                        $this->position = strlen($this->content) + $offset;
                        return true;
                    default:
                        return false;
                }
            }

            public function stream_stat()
            {
                return array();
            }
        }
    }

    if (
        (function_exists('stream_get_wrappers') && in_array('phpvfscomposer', stream_get_wrappers(), true))
        || (function_exists('stream_wrapper_register') && stream_wrapper_register('phpvfscomposer', 'Composer\BinProxyWrapper'))
    ) {
        return include 'phpvfscomposer://' . __DIR__ . '/..'.'/vendor-name/package-name/bin/tool';
    }
}

$GLOBALS['_composer_bin_dir'] = __DIR__;
$GLOBALS['_composer_autoload_path'] = __DIR__ . '/..'.'/autoload.php';

return include __DIR__ . '/..'.'/vendor-name/package-name/bin/tool';
```

This `BinProxyWrapper` stream wrapper is based on Composer v2.8.x (`src/Composer/Util/Platform.php` and bin proxy generation in `src/Composer/Installer/BinaryInstaller.php`). It strips the shebang line from PHP files when included on PHP < 8.0 (which would otherwise emit a parse error from the `#!` line). PHP 8.0+ natively ignores shebang lines in included files.

**Non-PHP target** (shell script, etc.):

```sh
#!/bin/sh

# Proxy shell script generated by Allegro
# @generated

dir=$(cd "$(dirname "$0")" && pwd)
export COMPOSER_RUNTIME_BIN_DIR="$dir"

"$dir"/../vendor-name/package-name/bin/tool "$@"
```

4. Set executable permissions on all proxy scripts: `chmod 0755` (fixed value, not umask-dependent)
5. MVP generates only Unix proxies. Windows `.bat` proxies are a Phase 2 feature (see Section 17.3).

**Path format:** Paths are constructed as `__DIR__ . '/..'.'/{vendor-name}/{package-name}/{bin-entry}'` — always one `/../` traversal from `vendor/bin/` to `vendor/`, then the package path. The string is split at the `..` boundary following Composer's `findShortestPathCode` convention.

**Global variables set by PHP proxies:**
- `$GLOBALS['_composer_bin_dir']` = `__DIR__` (the `vendor/bin/` directory)
- `$GLOBALS['_composer_autoload_path']` = path to `vendor/autoload.php`

### 6.6 Why Not Symlinks

PHP's `__DIR__` magic constant resolves symlinks to the real filesystem path. If a file at `vendor/monolog/monolog/src/Logger.php` is symlinked to `~/.allegro/store/files/ab/abcdef...`, then `__DIR__` inside that file returns `~/.allegro/store/files/ab/` instead of the expected `vendor/monolog/monolog/src/`. This breaks:

1. Packages using `__DIR__` for relative path calculations
2. Autoloader bootstrap (`__DIR__ . '/../../autoload.php'`)
3. Code coverage path filtering
4. IDE source mapping
5. Stack trace inspection

Hardlinks and reflinks do not have this problem — `__DIR__` returns the accessed path.

## 7. CLI Interface

### 7.1 Commands

| Command | Description | Example |
|---------|-------------|---------|
| `allegro install` | Install all dependencies from `composer.lock` | `allegro install` |
| `allegro install --dry-run` | Show what would be installed without making changes | `allegro install --dry-run` |
| `allegro status` | Show vendor state (see Section 7.5 for details and edge cases) | `allegro status` |
| `allegro store status` | Show store statistics (total files, disk usage, package count) | `allegro store status` |
| `allegro store prune` | Remove orphaned CAS files not referenced by any package manifest. In MVP, manifest pruning is not supported — manifests are never deleted by prune. The `--project-dir` flag is reserved for Phase 3 (store garbage collection with project tracking). | `allegro store prune` |
| `allegro store path` | Print the store directory path | `allegro store path` |
| `allegro version` | Print version information: `allegro {version} (commit {sha}, built {date})` | `allegro version` |

**`store prune` algorithm:**
1. Enumerate all manifests in `packages/`
2. Collect all file hashes referenced in all manifests
3. Enumerate all files in `files/`
4. Delete files not in the referenced hash set

**Warning:** `store prune` should not run concurrently with `allegro install`. In MVP, `store prune` takes no action to detect concurrent installs and runs at the user's risk. If run concurrently, prune may delete CAS files that an in-progress install needs, causing that install to re-download affected packages on retry. No data corruption occurs to already-installed vendor directories. Store-level locking is a Phase 3 feature. File deletion uses `os.Remove` which requires write permission on the parent directory (not on the file itself), so CAS files with `0444` permissions can be deleted without `chmod`.

### 7.2 Global Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--store-path` | Override store directory | `~/.allegro/store` |
| `--no-autoload` | Skip `composer dumpautoload` after linking | `false` |
| `--link-strategy` | Force a specific strategy: `reflink`, `hardlink`, `copy`. Skips the filesystem probe (Section 6.2). | auto-detect |
| `--workers` | Number of parallel download workers (valid: 1-32, out of range → clamp with warning to stderr: `"warning: --workers %d out of range [1,32], clamped to %d"`, process continues with exit code 0) | `8` |
| `--verbose` / `-v` | Verbose output (mutually exclusive with `--quiet`) | `false` |
| `--quiet` / `-q` | Suppress non-error output (takes precedence if both specified) | `false` |
| `--no-progress` | Disable progress bars (for CI) | `false` |
| `--dry-run` | **Install-only flag.** Show what would be installed without making changes. Runs steps 1-4 of the install pipeline (locate, detect strategy, parse, build plan) and prints the plan. Does not download, store, link, or generate autoload. Exits 0 on success. Ignored by other commands. | `false` |

### 7.3 Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error (unsupported dist type, archive extraction failure, lock timeout, store version incompatibility) |
| 2 | `composer.lock` or `composer.json` not found or invalid |
| 3 | Network error (download failed) |
| 4 | Filesystem error (linking failed, disk full, permission denied) |
| 5 | `composer dumpautoload` failed or composer binary not found |

**Note:** Exit code 1 is a catch-all for multiple distinct error types. Programmatic callers must parse stderr messages to distinguish between specific error conditions (e.g., unsupported dist type vs. archive extraction failure vs. concurrent lock timeout).

### 7.4 Progress Output

During `allegro install`, display:

```
Allegro v0.1.0 — installing from composer.lock

  Reading composer.lock ... {N} packages
  Resolving store diff ... {new} new, {cached} cached
  Downloading [{bar}] {done}/{total} packages ({time}s)
  Linking vendor/ [{bar}] {files} files ({time}s)
  Running composer dumpautoload ... done ({time}s)

  ✓ Installed {N} packages in {time}s ({strategy})
    Store: {store_path} ({size}, {file_count} files)
    Saved: ~{size} vs full copy
```

**Format rules:**
- Progress bar uses Unicode block character `█` (U+2588), 32 characters wide
- Per-step `{time}s` values are the wall-clock duration of that individual step (not cumulative). The final summary `{time}s` is the total wall-clock time from start to finish.
- Timing values are formatted as `%.1fs` (one decimal place, e.g., `3.2s`)
- Sizes use binary units: bytes < 1024 → `B`, < 1 MiB → `KiB`, < 1 GiB → `MiB`, else `GiB`, formatted as `%.0f` (no decimal places)
- File counts use comma-separated thousands (e.g., `12,459`)
- The `~` prefix on savings is always present (savings are theoretical)


### 7.5 `allegro status` Output

| Condition | Output |
|-----------|--------|
| `vendor/.allegro-state.json` exists, `composer.lock` matches `lock_hash` | "Vendor is up to date ({N} packages, {strategy}, installed {date})" — package count is the number of non-platform packages installed (packages + packages-dev, excluding `php`/`ext-*`/`lib-*`), matching the entries in the state file's `packages` map. |
| State file exists, `lock_hash` differs from current `composer.lock` | "Vendor is outdated — run `allegro install` to update" |
| `vendor/` exists but no `.allegro-state.json` | "Vendor exists but was not installed by Allegro" |
| `vendor/.allegro-state.json` exists but is malformed JSON or missing `lock_hash` field | "Vendor state file is corrupt — run `allegro install` to rebuild" |
| No `vendor/` directory | "No vendor directory found — run `allegro install`" |
| No `composer.lock` file | Exit 2, "composer.lock not found" |
| `composer.lock` exists but is unreadable (permission error) | Exit 2, "composer.lock: permission denied" |

**Date format:** The installed date is displayed as `YYYY-MM-DD` in UTC (e.g., `2026-04-05`), derived from the `installed_at` field in the state file.

## 8. Install Pipeline (Detailed Flow)

### 8.1 Step-by-Step

1. **Locate project files** — search current directory (the "project root") for `composer.lock` (always required). `composer.json` is required unless `--no-autoload` is set (it is only needed for `composer dumpautoload`). Exit 2 if a required file is missing.
2. **Detect link strategy** — run filesystem probe (see Section 6.2). This runs early to fail fast if the project root has filesystem issues. Skipped if `--link-strategy` is forced.
3. **Parse composer.lock** — extract `packages` and `packages-dev` arrays. Both are always installed (no `--no-dev` in MVP). Platform pseudo-packages (names like `php`, `ext-*`, `lib-*`) are skipped — they represent runtime constraints, not installable packages.
4. **Build install plan** — for each package, determine:
   - Is it already in the CAS? (check manifest existence by name + version)
   - Skip packages with `dist` set to `null` or `dist.type` set to `path` — warn and continue
5. **Download missing packages** — parallel worker pool (default 8 workers):
   - Download from `dist.url`
   - Verify `dist.shasum` if non-empty string — `dist.shasum` in Composer lock files is a **SHA-1** hex digest (not SHA-256). Skip verification if `shasum` is empty string or absent.
6. **Store in CAS** — for each downloaded package:
   - Extract archive (method determined by `dist.type` per Section 8.2)
   - Hash each file (SHA-256)
   - Move to content-addressed paths
   - Write package manifest
7. **Build vendor/** — build in temp dir and atomic swap (see Section 6.3)
8. **Generate autoloader** — unless `--no-autoload` is set, run `composer dumpautoload --optimize` with the project root as its working directory. Composer's stderr is forwarded to Allegro's stderr (displayed to the user). Composer's stdout is suppressed unless `--verbose` is set. This is the Orchestrator's responsibility, executed after vendor construction completes. Note: `composer dumpautoload` may regenerate `vendor/composer/installed.php` — Allegro's pre-generated version serves as bootstrap input, and the final version produced by Composer is authoritative.
9. **Write state file** — write `vendor/.allegro-state.json` (see Section 6.4) only after step 8 succeeds. If `--no-autoload` is set, write after step 7. This ensures `allegro status` never reports "up to date" for a vendor directory with incomplete autoloading.
10. **Report** — print summary with timing, strategy used, disk savings

### 8.2 Dist URL Resolution

Composer lock files contain dist URLs in this format:

```json
{
  "name": "monolog/monolog",
  "version": "3.9.0",
  "dist": {
    "type": "zip",
    "url": "https://api.github.com/repos/Seldaek/monolog/zipball/abc123",
    "reference": "abc123def456",
    "shasum": ""
  }
}
```

Allegro downloads from `dist.url` directly. The `dist.type` field is the **authoritative source** for determining extraction method — URL file extension is never consulted.

Supported `dist.type` values:

| dist.type | Extraction |
|-----------|-----------|
| `zip` | Go `archive/zip` |
| `tar` | Go `archive/tar` (uncompressed) |
| `gzip` | `gzip` decompression + `tar` extraction (always treated as tar.gz). If the decompressed stream is not a valid tar archive, treat as an archive extraction failure per Section 11.1. |
| `xz` | `xz` decompression + `tar` extraction. If the decompressed stream is not a valid tar archive, treat as an archive extraction failure per Section 11.1. |
| `path` | Skip — warn user (local path deps not supported in MVP) |
| Other / unknown | Exit 1 with "unsupported dist type: {type} for package {name}" |

## 9. Autoloader Compatibility

### 9.1 Strategy

Allegro does NOT generate autoloader files itself. Instead, it delegates to Composer:

```bash
composer dumpautoload --optimize
```

This ensures 100% compatibility with:
- PSR-4 autoloading
- PSR-0 autoloading (legacy)
- Classmap autoloading
- Files autoloading (helper functions)

Note: `installed.json` and `installed.php` are generated by Allegro before `dumpautoload` is invoked (see Section 9.4). `composer dumpautoload` does **not** generate `vendor/bin/` proxy scripts — only `composer install/update` does, so Allegro generates these itself (see Section 6.5).

### 9.2 Composer Binary Detection

Allegro locates the `composer` binary by:

1. `ALLEGRO_COMPOSER_PATH` environment variable
2. `composer` in `$PATH`
3. `composer.phar` in project directory
4. Error with helpful message if not found

### 9.3 Generated Files

After Allegro's install process completes, these files exist in `vendor/`:

**Generated by Allegro (before dumpautoload):**
- `vendor/composer/installed.json` — package metadata required as INPUT for `dumpautoload`
- `vendor/composer/installed.php` — PHP array format of installed packages. This serves as bootstrap input; `composer dumpautoload` may regenerate it, in which case Composer's version is authoritative.

**Generated by `composer dumpautoload`:**
- `vendor/autoload.php`
- `vendor/composer/autoload_classmap.php`
- `vendor/composer/autoload_namespaces.php`
- `vendor/composer/autoload_psr4.php`
- `vendor/composer/autoload_real.php`
- `vendor/composer/autoload_static.php`
- `vendor/composer/ClassLoader.php`
- `vendor/composer/LICENSE`
- `vendor/composer/platform_check.php`

**Generated by Allegro (after successful autoload generation):**
- `vendor/.allegro-state.json` — vendor state for `allegro status` (written after all steps succeed, see Section 8.1 step 9)

All these files are project-specific and not linked from CAS.

### 9.4 `installed.json` Generation

`composer dumpautoload` reads `vendor/composer/installed.json` as INPUT. Allegro must generate this file from `composer.lock` data **before** running `dumpautoload`.

The format matches Composer's output:

```json
{
  "packages": [
    {
      "name": "monolog/monolog",
      "version": "3.9.0",
      "version_normalized": "3.9.0.0",
      "type": "library",
      "autoload": {
        "psr-4": {
          "Monolog\\": "src/"
        }
      },
      "install-path": "../monolog/monolog"
    }
  ],
  "dev": true,
  "dev-package-names": ["phpunit/phpunit"]
}
```

**Field mapping from `composer.lock` to `installed.json`:**

| `installed.json` field | Source in `composer.lock` |
|------------------------|--------------------------|
| `name` | `package.name` |
| `version` | `package.version` |
| `version_normalized` | `package.version_normalized` |
| `type` | `package.type` (default: `"library"`) |
| `autoload` | `package.autoload` (if absent, use `{}`) |
| `install-path` | Always `../{vendor}/{name}` relative to `vendor/composer/` |
| `extra` | `package.extra` (if present) |
| `description` | `package.description` (if present) |
| `bin` | `package.bin` (if present) |
| `notification-url` | `package.notification-url` (if present) |
| `replace` | `package.replace` (if present, pass through) |
| `provide` | `package.provide` (if present, pass through) |
| `source` | `package.source` (if present, pass through) |

**Note:** For single-segment package names (without `/`), `install-path` is `../{name}` (e.g., `../acme`), consistent with the directory naming rule in Section 6.3 step 3.

**Platform pseudo-packages** (names like `php`, `ext-*`, `lib-*`) are NOT included in `installed.json` — they represent runtime constraints and have no installable files or autoload entries.

The `dev` field is always `true` in MVP (since `--no-dev` is not supported). The `dev-package-names` array is populated with all package names from the `packages-dev` array in `composer.lock`.

**`installed.php` generation:**

```php
<?php return array(
    'root' => array(
        'name' => '__root__',
        'pretty_version' => 'dev-main',
        'version' => 'dev-main',
        'reference' => NULL,
        'type' => 'project',
        'install_path' => __DIR__ . '/../../',
        'aliases' => array(),
        'dev' => true,
    ),
    'versions' => array(
        'monolog/monolog' => array(
            'pretty_version' => '3.9.0',
            'version' => '3.9.0.0',
            'reference' => 'abc123def456',
            'type' => 'library',
            'install_path' => __DIR__ . '/../monolog/monolog',
            'aliases' => array(),
            'dev_requirement' => false,
        ),
        // ... one entry per package
    ),
);
```

**Root entry field mapping from `composer.json`:**

| `installed.php` field | Source | Fallback when absent |
|-----------------------|--------|---------------------|
| `name` | `composer.json` → `name` | `'__root__'` |
| `pretty_version` | `composer.json` → `version` | `'dev-main'` |
| `version` | Use `pretty_version` verbatim (no normalization in MVP — Composer's `VersionParser::normalize` is complex and not reimplemented) | `'dev-main'` |
| `reference` | `NULL` (always) | — |
| `type` | `composer.json` → `type` | `'project'` |
| `aliases` | `array()` (always empty in MVP — branch alias resolution requires Composer's version solver which is a non-goal) | `array()` |

**Package entry field mapping:**

Each package entry maps `reference` from `dist.reference` in `composer.lock`, and `dev_requirement` is `true` for packages from the `packages-dev` array. The `aliases` field is always `array()` in MVP (branch alias resolution is deferred to Phase 3).
**Note:** `installed.php` uses `__DIR__` for all paths. Since this is a PHP runtime construct resolved at `include`-time (not at file-write-time), the paths resolve correctly after the atomic swap from `vendor.allegro.tmp/` to `vendor/`.
## 10. Configuration

**Configuration precedence** (applies to all options): CLI flag > environment variable > default. When both a CLI flag and its corresponding environment variable are set, the CLI flag always wins.

### 10.1 Environment Variables

All environment variables are subject to the same validation rules as their corresponding CLI flags (e.g., `ALLEGRO_WORKERS` is clamped to `[1,32]` with a warning to stderr, same as `--workers`).

| Variable | Description | Default |
|----------|-------------|---------|
| `ALLEGRO_STORE` | Store directory path | `~/.allegro/store` |
| `ALLEGRO_COMPOSER_PATH` | Path to composer binary | auto-detect |
| `ALLEGRO_WORKERS` | Parallel download workers (clamped to [1,32]) | `8` |
| `ALLEGRO_LINK_STRATEGY` | Force link strategy (skips probe) | auto-detect |
| `ALLEGRO_NO_PROGRESS` | Disable progress bars | `false` |
| `ALLEGRO_VERBOSE` | Enable verbose output | `false` |
| `ALLEGRO_QUIET` | Suppress non-error output | `false` |

### 10.2 No Config File in MVP

MVP does not have a project-level or global config file. All configuration is via environment variables and CLI flags. Config file support is a post-MVP feature.
## 11. Error Handling

### 11.1 Error Categories

| Category | Behavior |
|----------|----------|
| Missing `composer.lock` | Exit 2, print "composer.lock not found. Run `composer install` first." |
| Missing `composer.json` (when `--no-autoload` is not set) | Exit 2, print "composer.json not found. Required for autoload generation." |
| Invalid `composer.lock` JSON | Exit 2, print parse error with line/column |
| Store version incompatibility | Exit 1, print "store version %d is newer than this binary supports (max %d); upgrade allegro" |
| Unsupported `dist.type` | Exit 1, print unsupported type name and package |
| Package with `dist: null` | Warn and skip package (not fatal) |
| Download failure (single package) | Retry per policy (Section 11.2), then exit 3 |
| Download timeout | Per-download: 30s connection timeout, 5 min response body timeout. Treated as retryable. |
| Download failure (network down) | If any 3 download failures (from any workers, globally) have completion timestamps within a 10-second sliding window, abort remaining downloads and exit 3. Intervening successes do not reset the window. When abort is triggered, all in-flight downloads are cancelled via Go context cancellation, and any partially written temp files in `store/tmp/` are deleted before exit. |
| Hash mismatch after download (`dist.shasum` SHA-1 check) | The archive's SHA-1 hash does not match `dist.shasum`. Delete the downloaded archive, re-download (fresh HTTP request), and re-verify. This consumes one attempt from the 3-retry budget. After all retries exhausted, exit 3. Note: CAS-internal SHA-256 hashes are computed during storage (Section 5.6 step 3) and are never re-verified against existing CAS files during normal operations — CAS integrity verification is deferred to Phase 2 (`allegro verify`). |
| Archive extraction failure | Delete corrupted archive from tmp, **re-download the package** (fresh HTTP request), and retry extraction. This consumes one attempt from the shared 3-retry budget (same budget as download failures). Uses 1s → 2s → 4s backoff between attempts. After all retries exhausted, exit 1 with stderr message: `"archive extraction failed for {name}: {error}"` |
| Filesystem permission error | Exit 4, print affected path and required permissions |
| Store on different filesystem (hardlink fails) | Detected during probe — auto falls back to copy, warn user |
| Store corruption (CAS file missing or hash mismatch) | Re-download affected package, warn user |
| Composer binary not found | Exit 5, print detection paths tried and install instructions |
| `composer dumpautoload` fails | Exit 5, print Composer's stderr. The vendor directory is left in a linked-but-no-autoloader state; re-run `allegro install` to retry. |
| Disk full during CAS extraction | Clean up partial temp directory, exit 4, print store size and available space |
| Disk full during vendor build | Clean up `vendor.allegro.tmp/`, exit 4, print available space |
| Disk full during atomic swap | Vendor state is unknown (may have no `vendor/` dir); exit 4 with message "vendor state unknown — re-run `allegro install`" |
| Concurrent install on same project | Use `.allegro.lock` file lock in project root (via `flock`); if locked, wait up to 30s then exit 1 with "another allegro process is running" |

### 11.2 Retry Policy

For HTTP downloads:

1. Max 3 retries per package (hash mismatch retries and HTTP 429 retries all count within this budget)
2. Backoff: 1s → 2s → 4s (for non-429 retries)
3. Retry on: HTTP 5xx, connection timeout, connection reset
4. HTTP 429 (rate limit): respect `Retry-After` header, wait up to 60s max, then retry. This counts as one retry within the 3-retry budget. If 3 retries are exhausted (from any combination of 429s, 5xx errors, or hash mismatches), exit 3.
5. Do not retry on: HTTP 4xx (except 429), invalid URL

## 12. Testing Strategy

### 12.1 Unit Tests

| Component | Test Focus |
|-----------|------------|
| Parser | Valid/invalid `composer.lock` parsing, edge cases (empty packages, missing fields) |
| Store | Hash computation, file deduplication, atomic writes, manifest CRUD |
| Linker | Reflink/hardlink/copy detection, file permission preservation, directory creation |
| Fetcher | URL construction, retry logic (with mock HTTP server) |
| Orchestrator | Full pipeline with mock components |

### 12.2 Integration Tests

1. **Real Composer project** — create a minimal `composer.lock`, run `allegro install`, verify `vendor/` matches expected layout
2. **Laravel skeleton** — install a real Laravel project's dependencies, run `php artisan about`
3. **Re-install after lock change** — change one package version in `composer.lock`, run `allegro install` again, verify new version is installed and old version's CAS files remain
4. **Multi-project dedup** — install same dependencies in two project dirs, verify CAS files are shared. For hardlink strategy, verify same inode (`os.SameFile`). For reflink strategy, verify different inodes but reduced disk usage (compare `du` output of store + two vendors vs. two standalone vendors).
5. **Cross-platform** — CI matrix: macOS (APFS/reflink), Linux (ext4/hardlink), Linux (btrfs/reflink)
6. **Concurrent install lock** — spawn two `allegro install` processes on the same project simultaneously, verify the second exits with code 1 within ~30s, and the `vendor/` directory is consistent after the first completes.

### 12.3 Benchmark Tests

1. **vs Composer** — time `allegro install` vs `composer install` on cold and warm cache
2. **Disk savings** — measure CAS size vs total vendor size across N projects

## 13. Project Structure (Go)

```
allegro/
  cmd/
    allegro/
      main.go                 # entry point
  internal/
    cli/
      root.go                 # cobra root command
      install.go              # install command
      status.go               # status command
      store.go                # store subcommands
      version.go              # version command
    parser/
      lockfile.go             # composer.lock parser
      lockfile_test.go
      types.go                # Package, Dist, etc. structs
    fetcher/
      fetcher.go              # parallel downloader
      fetcher_test.go
      worker.go               # download worker
    store/
      store.go                # CAS operations
      store_test.go
      manifest.go             # package manifest read/write
      hasher.go               # SHA-256 file hashing
    linker/
      linker.go               # link strategy interface
      linker_test.go
      reflink.go              # reflink implementation
      reflink_linux.go        # linux-specific reflink
      reflink_darwin.go       # macOS-specific reflink
      reflink_windows.go      # Windows stub (returns "unsupported")
      hardlink.go             # hardlink implementation
      copy.go                 # copy fallback
      detect.go               # strategy detection probe
    autoloader/
      autoloader.go           # composer binary detection, dumpautoload execution
      autoloader_test.go
      installed.go            # installed.json / installed.php generation
      installed_test.go
    orchestrator/
      orchestrator.go         # install pipeline coordinator
      orchestrator_test.go
    platform/
      platform.go             # OS/filesystem detection
      platform_test.go
  go.mod
  go.sum
  LICENSE
  README.md
  spec/
    allegro.md                # this spec
```

## 14. Dependencies (Go Modules)

| Dependency | Purpose |
|------------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/schollz/progressbar/v3` | Progress bar display |
| `github.com/fatih/color` | Colored terminal output |

No SQLite dependency in MVP. The store uses filesystem layout + JSON manifests.

## 15. Build & Distribution

### 15.1 Build

The ldflags variables (`version`, `commit`, `buildDate`) are declared in `cmd/allegro/main.go`.

```bash
go build -ldflags "-s -w \
  -X main.version=0.1.0 \
  -X main.commit=$(git rev-parse --short HEAD) \
  -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o allegro ./cmd/allegro
```

**`allegro version` output format:** `allegro {version} (commit {sha}, built {date})` — e.g., `allegro 0.1.0 (commit abc1234, built 2026-04-05T00:00:00Z)`.
### 15.2 Cross-Compilation Targets

| OS | Arch | Binary Name |
|----|------|-------------|
| darwin | amd64 | `allegro-darwin-amd64` |
| darwin | arm64 | `allegro-darwin-arm64` |
| linux | amd64 | `allegro-linux-amd64` |
| linux | arm64 | `allegro-linux-arm64` |
| windows | amd64 | `allegro-windows-amd64.exe` |

### 15.3 Installation Methods (Post-MVP)

1. Direct binary download from GitHub Releases
2. Homebrew tap: `brew install allegro-php/tap/allegro`
3. Shell installer: `curl -fsSL https://get.allegro.dev | sh`

## 16. Phased Delivery

### Phase 1 — MVP (Current Spec)

- `allegro install` from `composer.lock`
- Content-addressable store with SHA-256
- Reflink → hardlink → copy fallback
- Parallel downloads (8 workers)
- `composer dumpautoload` delegation
- Progress output
- `allegro status`
- `allegro store status`, `allegro store prune`, `allegro store path`
- `allegro version`

### Phase 2 — Incremental & DX

- Incremental install (compare `vendor/.allegro-state.json` with `composer.lock`, only re-link changed packages)
- `allegro install --dev` / `--no-dev` flag (filter `packages-dev`)
- Vendor directory integrity check (`allegro verify`)
- Colored diff output for status
- Shell completions (bash, zsh, fish)
- Windows `.bat` proxy scripts for `vendor/bin/`
- Homebrew tap
- GitHub Actions for CI/CD release pipeline

### Phase 3 — Advanced

- Private repository support (Satis, GitLab, Bitbucket)
- Authentication (token, SSH key, HTTP basic)
- `composer.json`-only install (dependency resolution)
- Workspace support (monorepo with shared store)
- Global config file (`~/.allegro/config.json`)
- Composer script execution (`post-install-cmd`, etc.)
- Store garbage collection with project tracking (smarter prune)

### Phase 4 — Ecosystem

- VS Code extension (store status, link info)
- PHPStorm plugin
- Docker-optimized mode (pre-populated store volumes)
- Benchmarking dashboard (track install times over time)
- Plugin system for custom link strategies

## 17. Backward Compatibility & Known Limitations

### 17.1 Read-Only Vendor Files (Hardlink Strategy)

When using hardlinks, vendor files share an inode with the CAS. CAS files are stored with `0444`/`0555` permissions, so hardlinked vendor files are read-only. Modifying a hardlinked file modifies the CAS copy, affecting all projects. When using reflinks (CoW), modifications are isolated per-project automatically and permissions are set to `0644`/`0755` (matching Composer's output).

**Implications:**
1. **Debugging with vendor edits** — developers who edit vendor files for debugging will modify the CAS copy when hardlinked. Recommendation: use `--link-strategy copy` for development, or use reflink-capable filesystems (APFS, Btrfs).
2. **Composer patches** — plugins like `cweagans/composer-patches` write to vendor files. These are not supported in MVP (Composer plugins are a non-goal). Post-MVP, patches can be applied after linking with copy fallback for patched files.
3. **Running `composer require/update` after `allegro install`** — Composer will overwrite hardlinked files with regular copies. This is safe but loses deduplication benefits. Users should re-run `allegro install` after modifying `composer.lock`.

### 17.2 `vendor/bin/` Directory

Allegro generates `vendor/bin/` proxy scripts itself because `composer dumpautoload` does not generate them (see Section 6.5 for full details and proxy templates).

### 17.3 Windows `vendor/bin/` Proxy Scripts

On Windows, `vendor/bin/` proxy scripts are generated as Unix shell scripts only. Windows `.bat` proxy scripts are not generated in MVP. Windows users must use WSL or invoke PHP bin tools directly (e.g., `php vendor/vendor-name/package-name/bin/tool`). Windows `.bat` proxies are a Phase 2 feature. When `allegro install` completes on a Windows host (detected via `runtime.GOOS == "windows"`), a warning is printed to stderr: `"warning: vendor/bin/ proxy scripts are Unix-only; Windows .bat proxies will be available in a future version"`.

## 18. Security Considerations


1. **Store file permissions** — non-executable files stored as `0444`, executable files as `0555`, to prevent accidental mutation
2. **Download verification** — verify `dist.shasum` from `composer.lock` when non-empty. Composer's `dist.shasum` is a **SHA-1** hex digest (distinct from the CAS-internal SHA-256 hashing). Most GitHub-hosted packages have empty shasum; Allegro trusts `composer.lock` as source of truth. Users should ensure `composer.lock` integrity via git.
3. **TLS** — Allegro uses Go's `net/http` with system certificate store. TLS certificates are verified by default. Custom CA bundles are supported via Go's standard `SSL_CERT_FILE` and `SSL_CERT_DIR` environment variables.
4. **No arbitrary code execution** — Allegro itself does not execute PHP code; only `composer dumpautoload` is run as subprocess
5. **Temp directory cleanup** — clean up this process's `~/.allegro/store/tmp/tmp-{pid}-*/` entries on success or failure using Go `defer`. Note: `defer` does not execute on `SIGKILL` or hard crashes (OOM kill, power loss). Leftover temp directories from hard crashes are cleaned up at the start of the next `allegro install` run: entries matching the current PID or older than 1 hour are removed (see Section 6.3 step 2). This avoids deleting temp directories actively used by concurrent installs of other projects.
6. **No credential storage** — MVP does not handle authentication; no secrets are stored
7. **Supply chain trust** — Allegro downloads from URLs in `composer.lock`. If the lock file is tampered, Allegro will install whatever those URLs point to. This matches Composer's own trust model.

## 19. Performance Targets

These are benchmark targets measured on a reference environment: Apple M-series Mac, macOS with APFS, 100 Mbps+ network, Composer 2.8.x, PHP 8.3. They are non-gating benchmarks, not CI test assertions.

| Metric | Target | Measurement |
|--------|--------|-------------|
| Cold install (Laravel, 47 packages) | < 10s | Time from `allegro install` to completion |
| Warm install (all cached) | < 3s | Time with all packages in CAS (includes dumpautoload) |
| Linking only (no downloads, no autoload) | < 1s | Time to create vendor/ from CAS on local SSD |
| Store overhead per file | < 1 KB | Measured as (total manifest file size across all packages) / (total number of files indexed in those manifests) |
| Memory usage | < 256 MB | Peak RSS during install |

## 20. Glossary

| Term | Definition |
|------|------------|
| CAS | Content-Addressable Store — files indexed by their SHA-256 content hash |
| Reflink | Copy-on-write file clone; shares disk blocks until modified (APFS, Btrfs) |
| Hardlink | Additional directory entry pointing to the same inode; indistinguishable from original |
| Manifest | JSON file listing all files in a package version with their content hashes |
| Vendor | PHP convention: `vendor/` directory containing all third-party dependencies |
| Lock file | `composer.lock` — exact dependency versions and download URLs |
| Shard | Subdirectory named by first 2 hex chars of hash, distributing files across directories |
