package services

import (
	"sync"
	"testing"
	"time"
)

// testClock is a mutex-guarded manual clock injected into the negative cache
// (failureCache.setNow) so TTL expiry is tested without real sleeps.
type testClock struct {
	mu  sync.Mutex
	cur time.Time
}

func newTestClock(start time.Time) *testClock { return &testClock{cur: start} }

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cur
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.cur = c.cur.Add(d)
	c.mu.Unlock()
}

func TestNegCacheBlocksUntilTTL(t *testing.T) {
	c := newFailureCache()
	clock := newTestClock(time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC))
	c.setNow(clock.Now)

	const key = "url:https://example.test/feed"
	if c.blocked(key) {
		t.Error("fresh cache must not block")
	}
	c.markFailure(key)
	if !c.blocked(key) {
		t.Error("entry within TTL must block")
	}
	clock.Advance(negativeCacheTTL - time.Second)
	if !c.blocked(key) {
		t.Error("entry must still block just before TTL expiry")
	}
	clock.Advance(2 * time.Second)
	if c.blocked(key) {
		t.Error("entry must expire after TTL (recovery)")
	}
}

func TestNegCacheSuccessClearsEntry(t *testing.T) {
	c := newFailureCache()

	const key = "weather:52.52,13.41"
	c.markFailure(key)
	if !c.blocked(key) {
		t.Fatal("precondition: entry must block")
	}
	c.markSuccess(key)
	if c.blocked(key) {
		t.Error("a successful fetch must clear the entry immediately")
	}
}

func TestNegCachePrunesExpiredOnInsert(t *testing.T) {
	c := newFailureCache()
	clock := newTestClock(time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC))
	c.setNow(clock.Now)

	c.markFailure("url:a")
	clock.Advance(negativeCacheTTL + time.Second)
	c.markFailure("url:b")

	c.mu.Lock()
	_, aLeft := c.entries["url:a"]
	size := len(c.entries)
	c.mu.Unlock()
	if aLeft {
		t.Error("expired entry must be pruned opportunistically on insert")
	}
	if size != 1 {
		t.Errorf("cache size after prune = %d, want 1", size)
	}
}

func TestNegCacheFallbackValueRoundtrip(t *testing.T) {
	c := newFailureCache()
	clock := newTestClock(time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC))
	c.setNow(clock.Now)

	const key = "url:https://api.example.test/value"
	if _, hit := c.blockedFallback(key); hit {
		t.Error("fresh cache must not report a hit")
	}
	c.markFailureValue(key, "HTTP 503")
	if fallback, hit := c.blockedFallback(key); !hit || fallback != "HTTP 503" {
		t.Errorf("blockedFallback = (%q, %v), want (\"HTTP 503\", true)", fallback, hit)
	}
	// blocked() sees value entries too (shared key space).
	if !c.blocked(key) {
		t.Error("blocked() must report value entries as well")
	}
	clock.Advance(negativeCacheTTL + time.Second)
	if _, hit := c.blockedFallback(key); hit {
		t.Error("value entry must expire after TTL")
	}

	// Entries recorded via plain markFailure carry an empty fallback.
	c.markFailure(key)
	if fallback, hit := c.blockedFallback(key); !hit || fallback != "" {
		t.Errorf("blockedFallback after markFailure = (%q, %v), want (\"\", true)", fallback, hit)
	}
}

func TestNegCacheResetClearsEntries(t *testing.T) {
	c := newFailureCache()
	clock := newTestClock(time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC))
	c.setNow(clock.Now)
	c.markFailure("url:a")

	c.reset()
	if c.blocked("url:a") {
		t.Error("reset must clear all entries")
	}
	// After reset the real clock is active again: a new failure entry must
	// behave normally without the injected clock.
	c.markFailure("url:b")
	if !c.blocked("url:b") {
		t.Error("cache must work with the real clock after reset")
	}
}
