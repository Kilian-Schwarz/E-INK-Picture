package services

// Negative fetch cache (spec E5.5, decision 2).
//
// A failed widget fetch (transport error or non-200) is remembered per source
// for negativeCacheTTL. While an entry is fresh, the fetch sites short-circuit
// and return EXACTLY the same value as a live fetch failure, so a cache hit
// produces byte-identical render output to the error case. Only the failure
// STATE is cached, never content — content caching is E4.4 territory.
//
// Keys: "url:<url>" for defaultHTTPClient sources (RSS, iCal, custom API),
// "weather:<lat,lon>" for open-meteo (aligned with the positive cache key).

import (
	"sync"
	"time"
)

// negativeCacheTTL is how long a failed fetch attempt suppresses retries for
// the same source. Deliberately short: one timeout pulse per window instead of
// per render, while recovery after an outage stays fast.
const negativeCacheTTL = 2 * time.Minute

// failureEntry is one remembered fetch failure: when it expires and — for
// sites whose live failure value varies (custom API: "Error" vs
// "HTTP <code>") — the exact fallback value a cache hit must reproduce.
// Only the failure STATE is stored, never fetched content.
type failureEntry struct {
	expiry   time.Time
	fallback string
}

// failureCache remembers failed fetch attempts per source key. All methods
// are safe for concurrent use; the clock is injectable for tests.
type failureCache struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[string]failureEntry
}

func newFailureCache() *failureCache {
	return &failureCache{
		now:     time.Now,
		entries: make(map[string]failureEntry),
	}
}

// failCache is the package-wide negative fetch cache used by all render-path
// fetch sites. Tests reset it via failCache.reset() in t.Cleanup.
var failCache = newFailureCache()

// blocked reports whether key has a non-expired failure entry. Fetch sites
// with a constant failure value use this; sites with a varying value use
// blockedFallback.
func (c *failureCache) blocked(key string) bool {
	_, hit := c.blockedFallback(key)
	return hit
}

// blockedFallback reports whether key has a non-expired failure entry and
// returns the stored fallback value ("" when the failure was recorded via
// markFailure, i.e. the site derives its constant fallback itself).
func (c *failureCache) blockedFallback(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return "", false
	}
	if !c.now().Before(entry.expiry) {
		delete(c.entries, key)
		return "", false
	}
	return entry.fallback, true
}

// markFailure records a failed fetch for key (constant-fallback sites).
func (c *failureCache) markFailure(key string) {
	c.markFailureValue(key, "")
}

// markFailureValue records a failed fetch for key together with the exact
// fallback value a cache hit must reproduce, and opportunistically prunes
// expired entries, bounding the map by the set of configured source URLs.
func (c *failureCache) markFailureValue(key, fallback string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	for k, e := range c.entries {
		if !now.Before(e.expiry) {
			delete(c.entries, k)
		}
	}
	c.entries[key] = failureEntry{expiry: now.Add(negativeCacheTTL), fallback: fallback}
}

// markSuccess removes the failure entry for key (a successful fetch clears
// the negative state immediately).
func (c *failureCache) markSuccess(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// setNow injects a test clock.
func (c *failureCache) setNow(now func() time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = now
}

// reset clears all entries and restores the real clock (test hook).
func (c *failureCache) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]failureEntry)
	c.now = time.Now
}
