package fetcher

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// DownloadTask represents a single package to download.
type DownloadTask struct {
	Name    string
	URL     string
	Shasum  string // SHA-1 hex digest, empty to skip verification
	DistType string
}

// DownloadResult holds the outcome of a download.
type DownloadResult struct {
	Task  DownloadTask
	Data  []byte
	Error error
}

// Pool manages parallel downloads.
type Pool struct {
	client       *Client
	workers      int
	failureCount int64
	lastFailures []time.Time
	mu           sync.Mutex
}

// NewPool creates a download pool with the given worker count.
func NewPool(workers int) *Pool {
	return &Pool{
		client:  NewClient(),
		workers: workers,
	}
}

// Download downloads all tasks in parallel, returning results.
func (p *Pool) Download(ctx context.Context, tasks []DownloadTask) []DownloadResult {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	taskCh := make(chan DownloadTask, len(tasks))
	resultCh := make(chan DownloadResult, len(tasks))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < p.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range taskCh {
				if ctx.Err() != nil {
					resultCh <- DownloadResult{Task: task, Error: ctx.Err()}
					continue
				}

				// Unified retry loop per spec §11.2:
				// Max 3 retries total. HTTP errors, hash mismatches, and 429s
				// all consume from the same shared 3-retry budget.
				var finalData []byte
				var finalErr error

			retryLoop:
				for attempt := 0; attempt < maxRetries+1; attempt++ {
					if attempt > 0 {
						backoffIdx := attempt - 1
						if backoffIdx >= len(backoffSchedule) {
							backoffIdx = len(backoffSchedule) - 1
						}
						select {
						case <-ctx.Done():
							finalErr = ctx.Err()
							break retryLoop
						case <-time.After(backoffSchedule[backoffIdx]):
						}
					}

					resp, err := p.client.DownloadFull(ctx, task.URL)
					if err != nil {
						p.recordFailure()
						if p.shouldAbort() {
							cancel()
						}
						finalErr = err
						continue
					}

					if resp.StatusCode == 429 {
						retryAfter := RetryAfterDuration(resp.Headers.Get("Retry-After"))
						if retryAfter > 0 {
							select {
							case <-ctx.Done():
								finalErr = ctx.Err()
							case <-time.After(retryAfter):
							}
						}
						p.recordFailure()
						if p.shouldAbort() {
							cancel()
						}
						finalErr = fmt.Errorf("HTTP 429 for %s", task.URL)
						continue
					}

					if resp.StatusCode >= 200 && resp.StatusCode < 300 {
						// Verify SHA-1 if shasum is non-empty
						if task.Shasum != "" {
							if err := verifySHA1(resp.Body, task.Shasum); err != nil {
								p.recordFailure()
								if p.shouldAbort() {
									cancel()
								}
								finalErr = fmt.Errorf("SHA-1 mismatch for %s (attempt %d): %w", task.Name, attempt+1, err)
								continue // Re-download counts as retry
							}
						}
						finalData = resp.Body
						finalErr = nil
						break
					}

					if !IsRetryable(resp.StatusCode) {
						finalErr = fmt.Errorf("HTTP %d for %s (not retryable)", resp.StatusCode, task.URL)
						break
					}

					p.recordFailure()
					if p.shouldAbort() {
						cancel()
					}
					finalErr = fmt.Errorf("HTTP %d for %s", resp.StatusCode, task.URL)
				}

				if finalErr != nil {
					resultCh <- DownloadResult{Task: task, Error: finalErr}
				} else {
					resultCh <- DownloadResult{Task: task, Data: finalData}
				}
			}
		}()
	}

	// Send tasks
	for _, t := range tasks {
		taskCh <- t
	}
	close(taskCh)

	// Collect results
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	results := make([]DownloadResult, 0, len(tasks))
	for r := range resultCh {
		results = append(results, r)
	}
	return results
}

func (p *Pool) recordFailure() {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	p.lastFailures = append(p.lastFailures, now)
	atomic.AddInt64(&p.failureCount, 1)
}

// shouldAbort returns true if 3 failures within 10 seconds.
func (p *Pool) shouldAbort() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.lastFailures) < 3 {
		return false
	}
	window := 10 * time.Second
	recent := 0
	cutoff := time.Now().Add(-window)
	for _, t := range p.lastFailures {
		if t.After(cutoff) {
			recent++
		}
	}
	return recent >= 3
}

func verifySHA1(data []byte, expected string) error {
	h := sha1.Sum(data)
	got := hex.EncodeToString(h[:])
	if got != expected {
		return fmt.Errorf("SHA-1 mismatch: got %s, want %s", got, expected)
	}
	return nil
}
