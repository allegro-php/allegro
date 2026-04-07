# Phase 3 — Download Performance Optimization

**Goal:** Bring cold-cache install speed closer to uv/pnpm by fixing connection reuse, pipelining extraction, and increasing parallelism.

**Scope:** Only the download + extract path. No changes to linking, autoloader, verify, or CAS storage logic.

**Definitions:**
- **Cold cache:** CAS store removed (`rm -rf ~/.allegro/store`) before each run.
- **Warm cache:** CAS store populated, vendor/ removed.
- **`DownloadResult`:** Struct with `Task DownloadTask`, `Data []byte` (raw archive bytes in memory), `Error error`. After `extractAndStore` consumes `Data`, the slice becomes eligible for GC.

---

## 1. Tune HTTP Transport

**Problem:** `NewClient()` in `internal/fetcher/fetcher.go` constructs an `http.Transport` with only `ResponseHeaderTimeout: 30s`. All other values are Go defaults, critically `MaxIdleConnsPerHost=2`. With 16 workers hitting the same host, only 2 TCP connections are reused.

**Fix:** Update `NewClient()` — the single constructor for all HTTP clients — to use a tuned transport:

```go
Transport: &http.Transport{
    MaxIdleConns:          100,
    MaxIdleConnsPerHost:   32,
    IdleConnTimeout:       90 * time.Second,
    ResponseHeaderTimeout: 30 * time.Second,
    TLSHandshakeTimeout:  10 * time.Second,
    ForceAttemptHTTP2:     true,
    DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
}
```

**Rationale:**
- `MaxIdleConns=100` — ≥ 3× `MaxIdleConnsPerHost` × expected hosts (~3). Ensures per-host cap is never the bottleneck.
- `MaxIdleConnsPerHost=32` — fixed upper bound for max worker count. Not tied to default.
- `ForceAttemptHTTP2=true` — HTTP/2 multiplexes over single TCP connection. Falls back to HTTP/1.1 gracefully. `HTTP_PROXY`/`HTTPS_PROXY` respected.
- `DialContext` timeout 10s — prevents indefinite blocking on stalled TCP dials (Go default is infinite).
- `DisableKeepAlives` left at default `false` — keepalives are required for connection reuse.

Since `NewClient()` is used by Pool workers, `redownloadPackage`, and `extractAndStore` re-download, this fix applies globally.

---

## 2. Increase Default Download Worker Count

**Problem:** Default 8 download workers. Composer uses 12, pnpm uses 16.

**Fix:** Change download worker default from 8 to 16:

| Location | Current | New |
|----------|---------|-----|
| `internal/cli/flags.go` `ResolveWorkers` default | 8 | 16 |
| `internal/orchestrator/orchestrator.go` `downloadAndStore` display fallback | 8 | 16 |

**Note:** `buildVendorTree` uses `o.config.Workers` directly for linking parallelism. Since `o.config.Workers` now defaults to 16, linking also runs with 16 workers. This is acceptable — APFS/ext4 SSDs handle parallel hardlink creation well, and our benchmarks showed `cp -Rl` is already the bottleneck (kernel I/O), not goroutine count. The `buildVendorTree` fallback `if workers < 1 { workers = 8 }` only triggers when workers is explicitly 0; it remains unchanged.

**Config precedence for workers:** CLI flag (`--workers`) > env var (`ALLEGRO_WORKERS`) > default. No config-file tier exists for workers in the current implementation.

---

## 3. Pipeline Download + Concurrent Extract

### Step 1: Change `Pool.Download` API

**New signature:** `Pool.Download(ctx context.Context, tasks []DownloadTask, resultBuf int) <-chan DownloadResult`

- `resultBuf` — channel buffer capacity, set by caller (= `extractCap`). Controls back-pressure between download and extraction.

**Channel semantics:**
- Buffered to `resultBuf`. When buffer is full, download workers block on send — applying back-pressure.
- Supervisor goroutine: `go func() { wg.Wait(); close(resultCh); cancel() }()` — closes channel after all workers finish, AND cancels the pool's internal derived context to prevent context leaks.
- If `len(tasks) == 0`, return closed channel immediately.

**Internal cancel:** Pool creates `ctx, cancel := context.WithCancel(ctx)` internally. This cancel is called by (1) circuit-breaker on abort threshold, and (2) the supervisor goroutine after all workers finish. It is NOT `defer cancel()` — the pool returns the channel before workers complete.

**Worker send path:** Workers use `select` on both `resultCh <- result` and `ctx.Done()` to guarantee non-blocking exit after cancellation:

```go
select {
case resultCh <- result:
case <-ctx.Done():
    return
}
```

**`OnProgress` callback:** Fires inside each worker goroutine immediately before the channel send (NOT in a collector loop — there is no collector in the streaming design). Uses `atomic.Int32` for the `completed` counter to avoid data races from concurrent workers. `total` = `len(tasks)`, known at pool creation. The `OnProgress` call site moves from the old collector loop (`worker.go` L170) to the worker goroutine send path.

**Migration table:**

| Caller | File | Migration |
|--------|------|-----------|
| `downloadAndStore` | `orchestrator.go` | Pass `extractCap` as `resultBuf`. Display setup uses function-scoped `rendered`/`numWorkers` variables. §3 replaces §2's display fallback change — the fallback is re-applied in the new code. |
| `redownloadPackage` | `orchestrator.go` | Use `Pool.DownloadOne(ctx, task)`. Continue passing single-element `[]parser.Package{pkg}` to `extractAndStore` — this is correct; `extractAndStore` only uses the slice for `bin` field lookup on the specific package being extracted. |
| Tests | `worker_test.go` | Drain: `for r := range ch { ... }`. Add `TestPoolDownloadOne` covering success, error, and context cancellation. |
| `verify.go` | — | Not affected (does not use Pool.Download) |
| `extractAndStore` re-download | `orchestrator.go` | Uses `NewClient()` directly — benefits from §1 transport tuning automatically |
**`Pool.DownloadOne` helper:**

```go
func (p *Pool) DownloadOne(ctx context.Context, task DownloadTask) (DownloadResult, error) {
    for r := range p.Download(ctx, []DownloadTask{task}, 1) {
        if r.Error != nil {
            return DownloadResult{}, r.Error
        }
        return r, nil
    }
    // Channel closed without result — only on context cancellation
    return DownloadResult{}, ctx.Err()
}
```

### Step 2: Consume channel with concurrent extraction

```go
ctx, cancel := context.WithCancel(ctx)
defer cancel()

extractCap := max(2, min(runtime.NumCPU(), 8))
resultCh := pool.Download(ctx, tasks, extractCap)

extractSem := make(chan struct{}, extractCap)
var extractWg sync.WaitGroup
var extractErr error
var errOnce sync.Once

// Display variables declared at function scope so deferred cleanup can access them.
// `rendered` and `numWorkers` are set inside the `if !o.config.Quiet` display setup block.
var rendered bool
var numWorkers int

// Deferred display cleanup — clears worker lines on any exit path
defer func() {
    if !o.config.Quiet && rendered {
        clearWorkerDisplay(numWorkers)
    }
}()

loop:
for r := range resultCh {
    if ctx.Err() != nil {
        break loop
    }
    if r.Error != nil {
        errOnce.Do(func() {
            extractErr = fmt.Errorf("download %s: %w", r.Task.Name, r.Error)
            cancel()
        })
        break loop
    }
    select {
    case extractSem <- struct{}{}:
    case <-ctx.Done():
        break loop
    }
    extractWg.Add(1)
    go func(r DownloadResult) {
        defer extractWg.Done()
        defer func() { <-extractSem }()
        if ctx.Err() != nil { return }
        if err := o.extractAndStore(ctx, r, packages); err != nil {
            errOnce.Do(func() { extractErr = err; cancel() })
        }
    }(r)
}
extractWg.Wait()
if extractErr != nil { return extractErr }
```

**Labeled break:** `loop:` label on the for-range, `break loop` in all exit paths — follows the existing `break sendLoop` pattern in `parallel_link.go`.

**Extraction concurrency cap:** `max(2, min(runtime.NumCPU(), 8))`:
- `runtime.NumCPU()` returns logical CPUs (including hyperthreads). Intentional — extraction benefits from logical cores for I/O interleaving. `GOMAXPROCS` is not used because it may be artificially limited in containers.
- Floor of 2: pipeline benefit even on single-core CI.
- Ceiling of 8: prevents memory pressure.
- Go 1.21+ builtins `min`/`max`. Current go.mod is Go 1.25.

**Memory budget** (simultaneous — channel buffer + active extractions):
- `extractCap` results buffered in channel (compressed `Data []byte`): typical 8 × 10MB = 80MB, worst 8 × 50MB = 400MB.
- `extractCap` extraction goroutines (decompressed archive in temp dir + hashing): typical 8 × 10MB = 80MB, worst 8 × 50MB = 400MB.
- Total peak RSS: typical ~160MB, worst case ~800MB. Both categories are simultaneous because a goroutine holds `Data` while extracting.

**Thread safety:** `packages` is `[]parser.Package` (read-only). `extractAndStore` uses per-package temp dirs with atomic CAS writes.

### Error Handling

- **Download failure:** `errOnce.Do` + `cancel()` + `break loop`. Workers see `ctx.Err()` in retry loop and in send `select`. Workers exit promptly — no goroutine leak.
- **Extraction failure:** Same pattern.
- **Semaphore acquire after cancel:** `select` with `ctx.Done()` — never blocks.
- **In-flight extractions after cancel:** Check `ctx.Err()` at entry. Mid-`filepath.Walk` goroutines finish their current package (atomic work, bounded — typically < 5s per package for archives under 50MB).
- **CAS concurrent writes:** Idempotent. Same SHA-256 = identical content. Last `os.Rename` wins. POSIX rename is atomic. Windows handled by `StoreFile` existing-file fallback.
- **Closer goroutine lifecycle:** `go func() { wg.Wait(); close(resultCh); cancel() }()`. Exits after workers drain. Workers exit via `ctx.Err()` or task completion. Not a leak.

### Progress Display

`OnProgress` fires inside Pool on download completion. Display callbacks set up before `pool.Download`, same as current. Worker display shows active downloads. After all downloads (`done == total`), display clears. Extraction continues silently — "Installed N packages in Xs" prints after `extractWg.Wait()`.

Deferred cleanup clears display lines on any exit (error break or normal completion).

---

## Implementation Order

1. Transport tuning (§1)
2. Worker count (§2)
3. Pipeline + concurrent extraction (§3)

Each step benchmarked incrementally to isolate contribution.

---

## Benchmark Plan

**Baseline:** Check out tag `v0.3.0`, build binary, record times. Return to branch for "after."

**Reset per run:** `rm -rf ~/.allegro/store` + `composer clear-cache`. OS page cache NOT cleared — median of 3 warm-OS-cache / cold-CAS runs matches real-world usage.

**Command:** `allegro install --no-autoload --no-scripts`. 3 runs, take median.

| Project | Packages | Baseline | After | Target |
|---------|----------|----------|-------|--------|
| Event Machine | 147 | measure | measure | ≥ 20% faster |
| Sylius | 277 | measure | measure | ≥ 20% faster |
| Spryker Suite | 1574 | measure | measure | ≥ 25% faster |

**Pass:** Each project meets its target. Below target = failure.

---

## Non-Goals

- Streaming to disk, range requests, DNS pre-resolution, HTTP/3.
