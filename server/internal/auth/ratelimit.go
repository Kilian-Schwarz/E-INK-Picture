package auth

import (
	"sync"
	"time"
)

const (
	// RateLimitAttempts is the maximum number of login/setup attempts per IP
	// within one window; the next attempt is rejected.
	RateLimitAttempts = 5
	// RateLimitWindow is the fixed rate-limit window.
	RateLimitWindow = 60 * time.Second
)

type rateWindow struct {
	start time.Time
	count int
}

// RateLimiter is a per-IP fixed-window limiter for the login and setup
// endpoints (5 attempts / 60 s, in-memory). X-Forwarded-For is deliberately
// not parsed (spoofable without a trust concept, spec non-goal).
type RateLimiter struct {
	mu      sync.Mutex
	entries map[string]rateWindow
	limit   int
	window  time.Duration
	now     func() time.Time
}

// NewRateLimiter creates a limiter with the spec defaults (5 attempts / 60 s).
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		entries: make(map[string]rateWindow),
		limit:   RateLimitAttempts,
		window:  RateLimitWindow,
		now:     time.Now,
	}
}

// SetClock injects a time source for tests.
func (l *RateLimiter) SetClock(now func() time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.now = now
}

// Allow records an attempt for ip and reports whether it is within the
// limit. When rejected, retryAfter is the time until the window resets
// (for the Retry-After header).
func (l *RateLimiter) Allow(ip string) (ok bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()

	w, exists := l.entries[ip]
	if !exists || now.Sub(w.start) >= l.window {
		l.entries[ip] = rateWindow{start: now, count: 1}
		return true, 0
	}
	if w.count >= l.limit {
		return false, w.start.Add(l.window).Sub(now)
	}
	w.count++
	l.entries[ip] = w
	return true, 0
}

// Reset clears the attempt counter for ip (called after a successful login).
func (l *RateLimiter) Reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, ip)
}

// Cleanup drops expired windows and returns how many were removed.
func (l *RateLimiter) Cleanup() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	removed := 0
	for ip, w := range l.entries {
		if now.Sub(w.start) >= l.window {
			delete(l.entries, ip)
			removed++
		}
	}
	return removed
}

// StartJanitor runs Cleanup on the given interval until the returned stop
// function is called.
func (l *RateLimiter) StartJanitor(interval time.Duration) (stop func()) {
	done := make(chan struct{})
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				l.Cleanup()
			case <-done:
				return
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(done) }) }
}
