# Phase 3 — Download Performance Optimization

**Goal:** Bring cold-cache install speed to parity with uv/pnpm by fixing connection reuse, pipelining extraction, and increasing parallelism.

**Scope:** Only the download + extract path. No changes to linking, autoloader, or CAS storage logic.

---

## 1. Tune HTTP Transport (5 lines)

**Problem:** `MaxIdleConnsPerHost` defaults to 2. With 8+ workers hitting the same host (api.github.com, codeload.github.com), only 2 TCP connections are reused — the rest do fresh TLS handshakes (~100-200ms each).

**Fix** in `internal/fetcher/fetcher.go`:

```go
Transport: &http.Transport{
    MaxIdleConns:          100,
    MaxIdleConnsPerHost:   32,
    IdleConnTimeout:       90 * time.Second,
    ResponseHeaderTimeout: 30 * time.Second,
    TLSHandshakeTimeout:  10 * time.Second,
    ForceAttemptHTTP2:     true,
}
```

- `MaxIdleConnsPerHost=32` — matches max worker count, ensures full connection reuse
- `ForceAttemptHTTP2=true` — enables HTTP/2 multiplexing on custom transports (multiple requests over single TCP connection)

**Expected speedup:** 10-30% on cold cache.

**Verification:** Run with `GODEBUG=http2debug=1` to confirm HTTP/2 is negotiated with GitHub and Packagist.

---

## 2. Increase Default Worker Count (1 line)

**Problem:** Default 8 workers. Composer uses 12, pnpm uses 16, uv uses 50+. For small HTTP downloads (50KB-5MB), the bottleneck is latency not bandwidth — more workers overlap more RTTs.

**Fix** in `internal/cli/flags.go`: Change default from 8 to 16.

**Expected speedup:** 10-25% (diminishing returns after ~24).

**Verification:** Benchmark cold install with 8 vs 16 vs 24 workers on Filament (198 packages).

---

## 3. Pipeline: Extract As Each Download Completes (~100 lines)

**Problem:** Current flow is download-ALL-then-extract-ALL:

```
[download 1] [download 2] ... [download N] → [extract 1] [extract 2] ... [extract N]
```

Downloads are I/O-bound (network), extractions are CPU-bound (ZIP decompress + SHA-256 hash + file write). These can overlap.

**Target flow:**

```
[download 1] [download 2] ... [download N]
     ↓            ↓                ↓
  [extract 1] [extract 2] ... [extract N]
```

**Implementation:**

1. Change `Pool.Download` to return a `<-chan DownloadResult` channel instead of `[]DownloadResult`
2. In `downloadAndStore`, consume the channel and start extraction immediately for each result
3. Use a semaphore (buffered channel) to limit concurrent extractions to `runtime.NumCPU()` (extraction is CPU-bound, too many would thrash)

```go
func (p *Pool) DownloadStream(ctx context.Context, tasks []DownloadTask) <-chan DownloadResult {
    // ... same worker logic, but don't collect results
    // caller reads from resultCh directly
}
```

**Error handling:** If extraction fails for one package, cancel context to stop remaining downloads. Same as current behavior but streaming.

**Progress display:** OnStart/OnFinish/OnProgress callbacks still work — they fire as each download completes, same as before.

**Expected speedup:** 20-40% on cold cache. For 200 packages, if downloading takes 60s and extraction takes 20s, we save ~20s of sequential extraction wait.

**Verification:** Compare total install time before/after on Spryker (1574 packages) — the largest project with the most extraction overhead.

---

## 4. Parallel Extraction (~50 lines)

**Problem:** Even with pipelining (#3), extraction itself is sequential — one `extractAndStore` at a time. ZIP decompression + SHA-256 hashing benefits from multi-core parallelism.

**Fix:** Use a worker pool (semaphore) for concurrent extraction:

```go
extractSem := make(chan struct{}, runtime.NumCPU())
var extractWg sync.WaitGroup
var extractErr error
var errOnce sync.Once

for r := range resultCh {
    if r.Error != nil { ... }
    extractSem <- struct{}{} // acquire
    extractWg.Add(1)
    go func(r DownloadResult) {
        defer extractWg.Done()
        defer func() { <-extractSem }() // release
        if err := o.extractAndStore(ctx, r, packages); err != nil {
            errOnce.Do(func() { extractErr = err; cancel() })
        }
    }(r)
}
extractWg.Wait()
```

**Safety:** CAS writes are already atomic (temp + rename). `StoreFile` handles concurrent writes to the same hash via `FileExists` check. Manifest writes are per-package (no shared state). `storeExtractedFiles` uses per-invocation temp directories.

**Expected speedup:** 10-20% of extraction phase. Most visible on Spryker with 1574 packages.

**Verification:** Run `go test -race ./...` to confirm no data races.

---

## Implementation Order

1. Transport tuning (#1) — immediate win, zero risk
2. Worker count (#2) — immediate win, zero risk  
3. Pipeline extraction (#3) — biggest impact, medium refactor
4. Parallel extraction (#4) — builds on #3, smaller incremental gain

## Benchmark Plan

Before/after comparison on 3 projects with cleared CAS (`rm -rf ~/.allegro/store`):

| Project | Packages | Before | After | Speedup |
|---------|----------|--------|-------|---------|
| Event Machine | 147 | ? | ? | ? |
| Sylius | 277 | ? | ? | ? |
| Spryker Suite | 1574 | ? | ? | ? |

Each measured 3 times, median taken. Use `--no-scripts --no-autoload` to isolate download+extract+link performance.

## Non-Goals

- Streaming to disk (write directly to file instead of `[]byte`) — useful for memory pressure but doesn't improve speed for typical package sizes. Deferred.
- Range requests / parallel chunk downloads — not beneficial for 50KB-5MB ZIP files.
- DNS pre-resolution — OS cache handles this after Transport fix.
- HTTP/3 (QUIC) — Go's net/http doesn't support it natively yet.
