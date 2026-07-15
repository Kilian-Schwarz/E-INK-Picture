package auth

import (
	"regexp"
	"sync"
	"testing"
	"time"
)

// fakeClock is a mutex-protected movable time source for store/limiter tests.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func TestStoreCreateTokenFormat(t *testing.T) {
	s := NewStore()
	token, err := s.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// 32 bytes base64url without padding = 43 chars.
	if len(token) != 43 {
		t.Errorf("token length = %d, want 43", len(token))
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(token) {
		t.Errorf("token is not base64url: %q", token)
	}

	other, err := s.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if token == other {
		t.Error("two logins produced the same token")
	}
}

func TestStoreValidateUnknownToken(t *testing.T) {
	s := NewStore()
	if s.Validate("") {
		t.Error("empty token validated")
	}
	if s.Validate("nonexistent") {
		t.Error("unknown token validated")
	}
}

func TestStoreExpiry(t *testing.T) {
	clock := newFakeClock()
	s := NewStore()
	s.SetClock(clock.Now)

	token, err := s.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	clock.Advance(SessionTTL + time.Minute)
	if s.Validate(token) {
		t.Error("expired session validated")
	}
	if s.Len() != 0 {
		t.Errorf("expired session not removed from store, len=%d", s.Len())
	}
}

func TestStoreSlidingRenewal(t *testing.T) {
	clock := newFakeClock()
	s := NewStore()
	s.SetClock(clock.Now)

	token, err := s.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Access shortly before expiry renews the TTL...
	clock.Advance(SessionTTL - time.Hour)
	if !s.Validate(token) {
		t.Fatal("session expired too early")
	}
	// ...so it survives past the original expiry.
	clock.Advance(SessionTTL - time.Hour)
	if !s.Validate(token) {
		t.Error("sliding renewal did not extend the session")
	}
	// Without further access it expires TTL after the last renewal.
	clock.Advance(SessionTTL + time.Minute)
	if s.Validate(token) {
		t.Error("session did not expire TTL after last access")
	}
}

func TestStoreDelete(t *testing.T) {
	s := NewStore()
	token, err := s.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	s.Delete(token)
	if s.Validate(token) {
		t.Error("deleted session still validates")
	}
}

func TestStoreCapacityEvictsOldest(t *testing.T) {
	clock := newFakeClock()
	s := NewStore()
	s.SetClock(clock.Now)

	// AC6: 33 logins — the first (oldest) session is evicted.
	first, err := s.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	tokens := make([]string, 0, 32)
	for i := 0; i < 32; i++ {
		clock.Advance(time.Second)
		tok, err := s.Create()
		if err != nil {
			t.Fatalf("Create #%d: %v", i, err)
		}
		tokens = append(tokens, tok)
	}
	if s.Len() != 32 {
		t.Errorf("store len = %d, want capacity 32", s.Len())
	}
	if s.Validate(first) {
		t.Error("oldest session survived eviction")
	}
	for i, tok := range tokens {
		if !s.Validate(tok) {
			t.Errorf("newer session #%d was evicted", i)
		}
	}
}

func TestStoreCleanup(t *testing.T) {
	clock := newFakeClock()
	s := NewStore()
	s.SetClock(clock.Now)

	old, err := s.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	clock.Advance(SessionTTL + time.Minute)
	fresh, err := s.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if removed := s.Cleanup(); removed != 1 {
		t.Errorf("Cleanup removed %d sessions, want 1", removed)
	}
	if s.Validate(old) {
		t.Error("expired session survived cleanup")
	}
	if !s.Validate(fresh) {
		t.Error("live session removed by cleanup")
	}
}

func TestStoreJanitor(t *testing.T) {
	clock := newFakeClock()
	s := NewStore()
	s.SetClock(clock.Now)

	if _, err := s.Create(); err != nil {
		t.Fatalf("Create: %v", err)
	}
	clock.Advance(SessionTTL + time.Minute)

	stop := s.StartJanitor(5 * time.Millisecond)
	defer stop()

	deadline := time.Now().Add(2 * time.Second)
	for s.Len() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("janitor did not clean up the expired session")
		}
		time.Sleep(5 * time.Millisecond)
	}
	stop()
	stop() // stopping twice must be safe
}

// TestStoreConcurrentAccess exercises create/validate/delete/cleanup in
// parallel; run with -race (AC6).
func TestStoreConcurrentAccess(t *testing.T) {
	s := NewStore()
	stop := s.StartJanitor(time.Millisecond)
	defer stop()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				token, err := s.Create()
				if err != nil {
					t.Errorf("Create: %v", err)
					return
				}
				s.Validate(token)
				if j%3 == 0 {
					s.Delete(token)
				}
				s.Cleanup()
				s.Len()
			}
		}()
	}
	wg.Wait()
}
