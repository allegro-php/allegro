package fetcher

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// DownloadTask represents a single package to download.
type DownloadTask struct {
	Name     string
	Version  string
	URL      string
	Shasum   string // SHA-1 hex digest, empty to skip verification
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
	lastFailures []time.Time
	mu           sync.Mutex
	OnProgress   func(completed, total int, name string) // optional: called when a download completes
	OnStart      func(workerID int, name, version string) // optional: called when a worker starts a download
	OnFinish     func(workerID int, name, version string) // optional: called when a worker finishes a download
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
		go func(workerID int) {
			defer wg.Done()
			for task := range taskCh {
				if p.OnStart != nil {
					p.OnStart(workerID, task.Name, task.Version)
				}
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
						if p.recordFailureAndCheck() {
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
								break retryLoop
							case <-time.After(retryAfter):
							}
						}
						if p.recordFailureAndCheck() {
							cancel()
						}
						finalErr = fmt.Errorf("HTTP 429 for %s", task.URL)
						continue
					}

					if resp.StatusCode >= 200 && resp.StatusCode < 300 {
						// Verify SHA-1 if shasum is non-empty
						if task.Shasum != "" {
							if err := verifySHA1(resp.Body, task.Shasum); err != nil {
								if p.recordFailureAndCheck() {
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

					if p.recordFailureAndCheck() {
						cancel()
					}
					finalErr = fmt.Errorf("HTTP %d for %s", resp.StatusCode, task.URL)
				}

			if p.OnFinish != nil {
					p.OnFinish(workerID, task.Name, task.Version)
				}
				if finalErr != nil {
					resultCh <- DownloadResult{Task: task, Error: finalErr}
				} else {
					resultCh <- DownloadResult{Task: task, Data: finalData}
				}
			}
		}(i)
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

	total := len(tasks)
	results := make([]DownloadResult, 0, total)
	for r := range resultCh {
		results = append(results, r)
		if p.OnProgress != nil {
			p.OnProgress(len(results), total, r.Task.Name)
		}
	}
	return results
}

// recordFailureAndCheck records a failure timestamp, trims old entries,
// and returns true if abort threshold is reached (3 failures in 10s).
func (p *Pool) recordFailureAndCheck() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	p.lastFailures = append(p.lastFailures, now)
	cutoff := now.Add(-10 * time.Second)
	trimmed := make([]time.Time, 0, len(p.lastFailures))
	for _, t := range p.lastFailures {
		if t.After(cutoff) {
			trimmed = append(trimmed, t)
		}
	}
	p.lastFailures = trimmed
	return len(p.lastFailures) >= 3
}

func verifySHA1(data []byte, expected string) error {
	h := sha1.Sum(data)
	got := hex.EncodeToString(h[:])
	if got != expected {
		return fmt.Errorf("SHA-1 mismatch: got %s, want %s", got, expected)
	}
	return nil
}
