package services

// Nominatim politeness layer for SearchLocation (spec B1a).
//
// Two additive guards sit in front of the existing Nominatim call so the
// server provably honours the OSM Nominatim usage policy even when a client
// fails to debounce:
//
//   - minIntervalLimiter enforces at most one OUTGOING request per second.
//     It is a reject-style limiter: allow() reports whether a request may go
//     out now and, if so, reserves the slot. A denied caller degrades
//     gracefully (cached-or-empty) instead of blocking, so allow() never
//     sleeps and the min spacing is testable with an injected clock.
//   - locationSearchCache serves repeat queries from memory (keyed by the
//     normalized query) with a TTL and a bounded, oldest-first eviction —
//     geodata is stable, so a cache hit spares Nominatim entirely.
//
// Both mirror the injectable-clock / reset pattern of failureCache
// (negcache.go) so tests drive a fake clock without wall-clock sleeps.

import (
	"strings"
	"sync"
	"time"
)

// nominatimUserAgent identifies this application to Nominatim as required by
// the OSM usage policy ("Provide a valid HTTP Referer or User-Agent
// identifying the application; stock User-Agents such as generic browser
// strings are unacceptable"). It carries the app name, version and a contact
// repository URL — deliberately NOT a stock browser string like "Mozilla/5.0".
const nominatimUserAgent = "E-INK-Picture/1.0 (+https://github.com/Kilian-Schwarz/E-INK-Picture)"

const (
	// nominatimMinInterval is the minimum spacing between two outgoing
	// Nominatim requests (usage policy: at most one request per second).
	nominatimMinInterval = time.Second

	// locationCacheTTL is how long a location result stays cached. Geodata
	// (names/coordinates) is stable, so a long TTL is safe; 12 h keeps the
	// worst-case staleness of a renamed/moved place bounded and a restart
	// clears the in-memory cache anyway.
	locationCacheTTL = 12 * time.Hour

	// maxLocationCacheEntries bounds the location cache so a stream of unique
	// queries cannot grow it without limit (RAM budget); the oldest entry is
	// evicted once the cap is exceeded.
	maxLocationCacheEntries = 128
)

// normalizeLocationQuery is the cache key derivation: trimmed, lower-cased and
// with internal whitespace runs collapsed to single spaces. Two queries that
// differ only in case or surrounding/interior whitespace share one entry.
func normalizeLocationQuery(query string) string {
	return strings.Join(strings.Fields(strings.ToLower(query)), " ")
}

// minIntervalLimiter enforces a minimum spacing between outgoing requests.
// All methods are safe for concurrent use; the clock is injectable for tests.
type minIntervalLimiter struct {
	mu       sync.Mutex
	now      func() time.Time
	interval time.Duration
	last     time.Time
	hasLast  bool
}

func newMinIntervalLimiter(interval time.Duration) *minIntervalLimiter {
	return &minIntervalLimiter{
		now:      time.Now,
		interval: interval,
	}
}

// allow reports whether an outgoing request may proceed now. When it returns
// true it reserves the slot (records the current time), so any allow within
// interval afterwards returns false. It never sleeps: a denied caller must
// degrade gracefully instead of waiting.
func (l *minIntervalLimiter) allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if l.hasLast && now.Sub(l.last) < l.interval {
		return false
	}
	l.last = now
	l.hasLast = true
	return true
}

// setNow injects a test clock.
func (l *minIntervalLimiter) setNow(now func() time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.now = now
}

// reset clears the reserved slot and restores the real clock (test hook).
func (l *minIntervalLimiter) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.now = time.Now
	l.last = time.Time{}
	l.hasLast = false
}

// locationCacheEntry is one cached search result set with its store time.
type locationCacheEntry struct {
	results  []LocationResult
	cachedAt time.Time
}

// locationSearchCache is a bounded, TTL'd, in-memory cache of location search
// results keyed by the normalized query. Safe for concurrent use; the clock is
// injectable for tests.
type locationSearchCache struct {
	mu      sync.Mutex
	now     func() time.Time
	ttl     time.Duration
	max     int
	entries map[string]locationCacheEntry
}

func newLocationSearchCache(ttl time.Duration, max int) *locationSearchCache {
	return &locationSearchCache{
		now:     time.Now,
		ttl:     ttl,
		max:     max,
		entries: make(map[string]locationCacheEntry),
	}
}

// get returns the cached results for a normalized key while the entry is still
// within its TTL. An expired entry is dropped and reported as a miss.
func (c *locationSearchCache) get(key string) ([]LocationResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if !c.now().Before(entry.cachedAt.Add(c.ttl)) {
		delete(c.entries, key)
		return nil, false
	}
	return entry.results, true
}

// put stores results for a normalized key and evicts the oldest entry when the
// cap is exceeded (mirrors evictOldestCache).
func (c *locationSearchCache) put(key string, results []LocationResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = locationCacheEntry{results: results, cachedAt: c.now()}
	c.evictOldestLocked()
}

// evictOldestLocked removes the oldest entry when the cache exceeds max size.
// Must be called with c.mu held.
func (c *locationSearchCache) evictOldestLocked() {
	if len(c.entries) <= c.max {
		return
	}
	var oldestKey string
	var oldestTime time.Time
	for k, v := range c.entries {
		if oldestKey == "" || v.cachedAt.Before(oldestTime) {
			oldestKey = k
			oldestTime = v.cachedAt
		}
	}
	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}

// setNow injects a test clock.
func (c *locationSearchCache) setNow(now func() time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = now
}

// reset clears all entries and restores the real clock (test hook).
func (c *locationSearchCache) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]locationCacheEntry)
	c.now = time.Now
}
