// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

// Package ttlcache is a singleflight TTL cache for on-demand lookups
// (tool help, OPA policy bundles).
package ttlcache

import (
	"context"
	"sync"
	"time"
)

// Load fetches a value for key. A successful load is cached for the TTL.
type Load[K comparable, V any] func(ctx context.Context, key K) (V, error)

type entry[V any] struct {
	val   V
	until time.Time
	have  bool
}

type flight[V any] struct {
	wg  sync.WaitGroup
	val V
	err error
}

// Cache reuses a successful Load until TTL elapses. A negative TTL is
// treated as zero (always refresh). Concurrent Gets for the same key
// share one Load (singleflight). On Load error, a stale value is
// returned if one exists.
type Cache[K comparable, V any] struct {
	ttl  time.Duration
	load Load[K, V]

	mu       sync.Mutex
	entries  map[K]entry[V]
	inflight map[K]*flight[V]
}

// New returns a cache. ttl < 0 is stored as 0 (no caching).
func New[K comparable, V any](ttl time.Duration, load Load[K, V]) *Cache[K, V] {
	if ttl < 0 {
		ttl = 0
	}
	if load == nil {
		load = func(context.Context, K) (V, error) {
			var zero V
			return zero, nil
		}
	}
	return &Cache[K, V]{
		ttl:      ttl,
		load:     load,
		entries:  map[K]entry[V]{},
		inflight: map[K]*flight[V]{},
	}
}

func (c *Cache[K, V]) fresh(e entry[V]) bool {
	if !e.have || c.ttl <= 0 {
		return false
	}
	return time.Now().Before(e.until)
}

// Get returns a cached value or calls Load. On Load error, a stale value
// is returned together with the error (both the leader and waiters).
func (c *Cache[K, V]) Get(ctx context.Context, key K) (V, error) {
	c.mu.Lock()
	if e, ok := c.entries[key]; ok && c.fresh(e) {
		c.mu.Unlock()
		return e.val, nil
	}
	if f, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		f.wg.Wait()
		return f.val, f.err
	}
	f := &flight[V]{}
	f.wg.Add(1)
	c.inflight[key] = f
	stale, haveStale := c.entries[key]
	c.mu.Unlock()

	defer func() {
		f.wg.Done()
		c.mu.Lock()
		delete(c.inflight, key)
		c.mu.Unlock()
	}()

	val, err := c.load(ctx, key)
	if err != nil {
		if haveStale && stale.have {
			f.val = stale.val
			f.err = err
			return stale.val, err
		}
		f.err = err
		var zero V
		return zero, err
	}

	c.mu.Lock()
	e := entry[V]{val: val, have: true}
	if c.ttl > 0 {
		e.until = time.Now().Add(c.ttl)
	}
	c.entries[key] = e
	c.mu.Unlock()
	f.val = val
	return val, nil
}
