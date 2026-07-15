// Package auth implements the single-admin password authentication for the
// server: bcrypt password hashing persisted in data/auth.json, an in-memory
// session store and a per-IP rate limiter for the login/setup endpoints
// (specs/E5.1-authentication.md).
package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

// authFileName is the password hash file inside the data directory. It is a
// separate file (never models.Settings/settings.json) because settings.json
// is served to the client via GET /settings.
const authFileName = "auth.json"

// ErrPasswordAlreadySet is returned by SetPasswordIfUnset when an admin
// password hash already exists.
var ErrPasswordAlreadySet = errors.New("password already set")

// authFile is the on-disk JSON format of data/auth.json.
type authFile struct {
	PasswordHash string `json:"password_hash"`
}

// Manager holds the admin password hash and persists it to data/auth.json
// (mode 0600, atomic tmp+rename writes). A Manager without a hash means
// authentication is disabled (upgrade path without lockout).
type Manager struct {
	path string
	mu   sync.RWMutex
	hash []byte
}

// NewManager creates a Manager backed by <dataDir>/auth.json and loads an
// existing hash if the file is present.
func NewManager(dataDir string) (*Manager, error) {
	m := &Manager{path: filepath.Join(dataDir, authFileName)}

	data, err := os.ReadFile(m.path)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", m.path, err)
	}

	var f authFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", m.path, err)
	}
	if f.PasswordHash != "" {
		m.hash = []byte(f.PasswordHash)
	}
	return m, nil
}

// PasswordSet reports whether an admin password hash exists, i.e. whether
// authentication is active.
func (m *Manager) PasswordSet() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.hash) > 0
}

// SetPassword hashes the password with bcrypt (DefaultCost, 10) and persists
// it atomically to data/auth.json with mode 0600.
func (m *Manager) SetPassword(password string) error {
	if password == "" {
		return errors.New("password must not be empty")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.writeLocked(hash); err != nil {
		return err
	}
	m.hash = hash
	return nil
}

// SetPasswordIfUnset atomically sets the INITIAL admin password: the
// unset-check and the write happen under the same lock, so concurrent
// /api/auth/setup requests cannot both win (no TOCTOU). Exactly one caller
// succeeds; the others get ErrPasswordAlreadySet. The bcrypt hashing runs
// outside the lock (it is slow by design) — the race loser just wastes one
// hash computation.
func (m *Manager) SetPasswordIfUnset(password string) error {
	if password == "" {
		return errors.New("password must not be empty")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.hash) > 0 {
		return ErrPasswordAlreadySet
	}
	if err := m.writeLocked(hash); err != nil {
		return err
	}
	m.hash = hash
	return nil
}

// Verify reports whether the password matches the stored hash. It returns
// false when no password is set.
func (m *Manager) Verify(password string) bool {
	m.mu.RLock()
	hash := m.hash
	m.mu.RUnlock()
	if len(hash) == 0 {
		return false
	}
	return bcrypt.CompareHashAndPassword(hash, []byte(password)) == nil
}

// Bootstrap applies the EINK_ADMIN_PASSWORD startup semantics
// (spec, Architektur-Richtung 4): an existing auth.json always wins and the
// env var is ignored with a warning; without auth.json a non-empty env
// password is hashed and persisted, activating authentication.
func (m *Manager) Bootstrap(envPassword string) error {
	if envPassword == "" {
		return nil
	}
	if m.PasswordSet() {
		slog.Warn("EINK_ADMIN_PASSWORD ignored, password already set — delete data/auth.json to reset")
		return nil
	}
	if err := m.SetPassword(envPassword); err != nil {
		return fmt.Errorf("bootstrap admin password from EINK_ADMIN_PASSWORD: %w", err)
	}
	slog.Info("admin password set from EINK_ADMIN_PASSWORD; authentication enabled")
	return nil
}

// writeLocked atomically persists the hash (tmp file + rename, mode 0600).
// Callers must hold m.mu.
func (m *Manager) writeLocked(hash []byte) error {
	data, err := json.Marshal(authFile{PasswordHash: string(hash)})
	if err != nil {
		return fmt.Errorf("marshal auth file: %w", err)
	}

	dir := filepath.Dir(m.path)
	tmp, err := os.CreateTemp(dir, authFileName+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp auth file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp auth file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp auth file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp auth file: %w", err)
	}
	if err := os.Rename(tmpName, m.path); err != nil {
		return fmt.Errorf("rename auth file: %w", err)
	}
	return nil
}
