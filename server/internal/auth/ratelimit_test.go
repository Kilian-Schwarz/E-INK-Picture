package auth

import (
	"sync"
	"testing"
	"time"
)

func TestRateLimiterAllowsUpToLimit(t *testing.T) {
	l := NewRateLimiter()
	for i := 1; i <= RateLimitAttempts; i++ {
		ok, _ := l.Allow("10.0.0.1")
		if !ok {
			t.Fatalf("attempt %d rejected, want first %d allowed", i, RateLimitAttempts)
		}
	}
	ok, retryAfter := l.Allow("10.0.0.1")
	if ok {
		t.Error("6th attempt within window allowed, want rejected")
	}
	if retryAfter <= 0 || retryAfter > RateLimitWindow {
		t.Errorf("retryAfter = %v, want in (0, %v]", retryAfter, RateLimitWindow)
	}
}

func TestRateLimiterPerIP(t *testing.T) {
	l := NewRateLimiter()
	for i := 0; i < RateLimitAttempts; i++ {
		l.Allow("10.0.0.1")
	}
	if ok, _ := l.Allow("10.0.0.1"); ok {
		t.Error("blocked IP allowed")
	}
	if ok, _ := l.Allow("10.0.0.2"); !ok {
		t.Error("other IP blocked, limiter must be per-IP")
	}
}

func TestRateLimiterWindowExpiry(t *testing.T) {
	clock := newFakeClock()
	l := NewRateLimiter()
	l.SetClock(clock.Now)

	for i := 0; i < RateLimitAttempts; i++ {
		l.Allow("10.0.0.1")
	}
	if ok, _ := l.Allow("10.0.0.1"); ok {
		t.Fatal("expected rejection within window")
	}
	clock.Advance(RateLimitWindow)
	if ok, _ := l.Allow("10.0.0.1"); !ok {
		t.Error("attempt after window expiry rejected")
	}
}

func TestRateLimiterReset(t *testing.T) {
	l := NewRateLimiter()
	for i := 0; i < RateLimitAttempts; i++ {
		l.Allow("10.0.0.1")
	}
	l.Reset("10.0.0.1")
	if ok, _ := l.Allow("10.0.0.1"); !ok {
		t.Error("attempt after Reset rejected")
	}
}

func TestRateLimiterCleanup(t *testing.T) {
	clock := newFakeClock()
	l := NewRateLimiter()
	l.SetClock(clock.Now)

	l.Allow("10.0.0.1")
	clock.Advance(RateLimitWindow)
	l.Allow("10.0.0.2")

	if removed := l.Cleanup(); removed != 1 {
		t.Errorf("Cleanup removed %d entries, want 1", removed)
	}
}

func TestRateLimiterJanitor(t *testing.T) {
	clock := newFakeClock()
	l := NewRateLimiter()
	l.SetClock(clock.Now)

	l.Allow("10.0.0.1")
	clock.Advance(RateLimitWindow)

	stop := l.StartJanitor(5 * time.Millisecond)
	defer stop()

	deadline := time.Now().Add(2 * time.Second)
	for {
		l.mu.Lock()
		n := len(l.entries)
		l.mu.Unlock()
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("janitor did not clean up the expired window")
		}
		time.Sleep(5 * time.Millisecond)
	}
	stop()
	stop() // stopping twice must be safe
}

// TestRateLimiterConcurrentAccess exercises the limiter in parallel;
// run with -race.
func TestRateLimiterConcurrentAccess(t *testing.T) {
	l := NewRateLimiter()
	stop := l.StartJanitor(time.Millisecond)
	defer stop()

	var wg sync.WaitGroup
	ips := []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				ip := ips[(n+j)%len(ips)]
				l.Allow(ip)
				if j%5 == 0 {
					l.Reset(ip)
				}
				l.Cleanup()
			}
		}(i)
	}
	wg.Wait()
}
