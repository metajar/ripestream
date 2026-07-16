// Package api embeds a tiny refresh-cache for expensive read queries.
package api

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// refreshCache stores a single computed result, refreshed on a schedule by a
// background goroutine. Callers read the last good value instantly; a refresh
// that fails keeps serving the previous value and logs the error.
//
// It exists because the Overview query runs several full-graph aggregations
// that grow slower as FalkorDB ingests more data, eventually exceeding the
// query timeout. Serving from cache decouples page load from graph size.
type refreshCache[T any] struct {
	load  func(ctx context.Context) (T, error)
	ttl   time.Duration

	val      atomic.Pointer[T]
	stale    atomic.Bool
	lastMS   atomic.Int64
	lastOK   atomic.Bool
	stopOnce sync.Once
	stopCh   chan struct{}
}

// newRefreshCache builds (but does not start) a cache. Call start to begin
// refreshing. ttl is the interval between refreshes.
func newRefreshCache[T any](load func(ctx context.Context) (T, error), ttl time.Duration) *refreshCache[T] {
	return &refreshCache[T]{load: load, ttl: ttl, stopCh: make(chan struct{})}
}

// start launches the background refresher. It runs one refresh immediately so
// the cache is populated without waiting a full interval, then ticks on ttl.
func (c *refreshCache[T]) start(ctx context.Context) {
	go func() {
		// Immediate first refresh so callers don't see an empty cache for a full ttl.
		c.refresh(ctx)
		t := time.NewTicker(c.ttl)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.stopCh:
				return
			case <-t.C:
				c.refresh(ctx)
			}
		}
	}()
}

// stop halts the refresher (idempotent).
func (c *refreshCache[T]) stop() {
	c.stopOnce.Do(func() { close(c.stopCh) })
}

// refresh runs one load, stores the result on success, and emits a log line
// with the elapsed time as requested.
func (c *refreshCache[T]) refresh(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	start := time.Now()
	// Use a detached context for the load so a request cancellation can't abort
	// a background refresh (the load has its own internal query timeout).
	loadCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	val, err := c.load(loadCtx)
	elapsed := time.Since(start)
	ms := elapsed.Milliseconds()
	c.lastMS.Store(ms)

	if err != nil {
		// Keep serving the previous value; mark stale so callers know.
		c.stale.Store(true)
		c.lastOK.Store(false)
		prev := c.val.Load()
		hasPrev := prev != nil
		slog.Warn("cache refresh failed; serving previous value",
			"err", err, "took_ms", ms, "have_previous", hasPrev, "stale", true)
		return
	}

	c.val.Store(&val)
	c.stale.Store(false)
	c.lastOK.Store(true)
	slog.Info("cache refresh ok", "took_ms", ms)
}

// get returns the cached value, whether it's stale, and whether a value exists
// yet (the very first refresh may still be in flight).
func (c *refreshCache[T]) get() (val T, stale bool, ok bool) {
	p := c.val.Load()
	if p == nil {
		return val, true, false // zero value, stale, not ready
	}
	return *p, c.stale.Load(), true
}
