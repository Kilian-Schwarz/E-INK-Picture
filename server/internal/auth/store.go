package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

const (
	// SessionTTL is the sliding session lifetime.
	SessionTTL = 7 * 24 * time.Hour
	// sessionCapacity bounds the store; the oldest session is evicted when
	// exceeded (single-admin device, bounded memory).
	sessionCapacity = 32
	// tokenBytes is the raw session token length (43 chars base64url).
	tokenBytes = 32
)

type session struct {
	expiresAt time.Time
	createdAt time.Time
}

// Store is a bounded in-memory session store. Tokens are opaque 32-byte
// crypto/rand values; sessions expire after a sliding TTL and the store is
// capped at 32 entries. Restart means re-login (accepted, spec decision 2).
type Store struct {
	mu       sync.Mutex
	sessions map[string]session
	ttl      time.Duration
	capacity int
	now      func() time.Time
}

// NewStore creates a session store with the spec defaults
// (TTL 7 days sliding, capacity 32).
func NewStore() *Store {
	return &Store{
		sessions: make(map[string]session),
		ttl:      SessionTTL,
		capacity: sessionCapacity,
		now:      time.Now,
	}
}

// SetClock injects a time source for tests.
func (s *Store) SetClock(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}

// Create issues a fresh session token (a new token per login — no session
// fixation). When the store is full, the oldest session is evicted.
func (s *Store) Create() (string, error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(buf)

	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if len(s.sessions) >= s.capacity {
		s.evictOldestLocked()
	}
	s.sessions[token] = session{expiresAt: now.Add(s.ttl), createdAt: now}
	return token, nil
}

// Validate reports whether the token belongs to a live session and renews
// the sliding TTL on success. Expired sessions are removed.
func (s *Store) Validate(token string) bool {
	if token == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[token]
	if !ok {
		return false
	}
	now := s.now()
	if now.After(sess.expiresAt) {
		delete(s.sessions, token)
		return false
	}
	sess.expiresAt = now.Add(s.ttl)
	s.sessions[token] = sess
	return true
}

// Delete removes a session (logout).
func (s *Store) Delete(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

// Len returns the number of stored sessions.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

// Cleanup removes all expired sessions and returns how many were dropped.
func (s *Store) Cleanup() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	removed := 0
	for token, sess := range s.sessions {
		if now.After(sess.expiresAt) {
			delete(s.sessions, token)
			removed++
		}
	}
	return removed
}

// StartJanitor runs Cleanup on the given interval until the returned stop
// function is called.
func (s *Store) StartJanitor(interval time.Duration) (stop func()) {
	done := make(chan struct{})
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.Cleanup()
			case <-done:
				return
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(done) }) }
}

// evictOldestLocked drops the session with the earliest creation time.
// Callers must hold s.mu.
func (s *Store) evictOldestLocked() {
	var oldestToken string
	var oldest time.Time
	for token, sess := range s.sessions {
		if oldestToken == "" || sess.createdAt.Before(oldest) {
			oldestToken = token
			oldest = sess.createdAt
		}
	}
	if oldestToken != "" {
		delete(s.sessions, oldestToken)
	}
}
