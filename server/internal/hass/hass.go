// Package hass persists the single Home-Assistant admin connection
// (base URL + long-lived token) to data/hass.json (mode 0600, atomic
// tmp+rename writes) and exposes it to the read-only HA fetch path
// (specs/B5-home-assistant.md, sub-task B5a). The token is a secret: it is
// stored only in data/hass.json, returned only to the fetch path via Config,
// and NEVER logged, echoed to the UI (Status/TokenSet omit it) or placed in a
// URL. This package mirrors the auth.Manager storage pattern byte-for-byte.
package hass

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"sync"
)

// hassFileName is the connection file inside the data directory. Like
// auth.json it is a separate file (never settings.json) because settings.json
// is served to the client via GET /settings and must never carry the token.
const hassFileName = "hass.json"

// hassFile is the on-disk JSON format of data/hass.json.
type hassFile struct {
	BaseURL string `json:"base_url"`
	Token   string `json:"token"`
}

// Manager holds the HA base URL and long-lived token and persists them to
// data/hass.json (mode 0600, atomic tmp+rename writes). An empty base URL or
// token means "not configured" (the fetch path stays offline, no lockout).
type Manager struct {
	path    string
	mu      sync.RWMutex
	baseURL string
	token   string
}

// NewManager creates a Manager backed by <dataDir>/hass.json and loads an
// existing configuration if the file is present. A missing file is "not
// configured", not an error.
func NewManager(dataDir string) (*Manager, error) {
	m := &Manager{path: filepath.Join(dataDir, hassFileName)}

	data, err := os.ReadFile(m.path)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", m.path, err)
	}

	var f hassFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", m.path, err)
	}
	m.baseURL = f.BaseURL
	m.token = f.Token
	return m, nil
}

// SetConfig validates baseURL (parseable, scheme http/https, non-empty host)
// and atomically persists the configuration with mode 0600. The token is
// stored verbatim and never validated by content. A validation error never
// contains the token value.
func (m *Manager) SetConfig(baseURL, token string) error {
	if err := validateBaseURL(baseURL); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.writeLocked(baseURL, token); err != nil {
		return err
	}
	m.baseURL = baseURL
	m.token = token
	return nil
}

// Config returns the base URL and token for the fetch path. ok is false when
// either the base URL or the token is empty (nothing to fetch with).
func (m *Manager) Config() (baseURL, token string, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.baseURL, m.token, m.baseURL != "" && m.token != ""
}

// Status returns the base URL and whether the connection is fully configured
// (base URL AND token present) for the admin UI. It NEVER returns the token.
func (m *Manager) Status() (baseURL string, configured bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.baseURL, m.baseURL != "" && m.token != ""
}

// TokenSet reports whether a token is stored, independent of the base URL
// (a bootstrap token without a URL is stored but not yet "configured"). It
// NEVER returns the token value.
func (m *Manager) TokenSet() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.token != ""
}

// Bootstrap applies the EINK_HASS_URL / EINK_HASS_TOKEN startup semantics,
// mirroring auth.Bootstrap: an existing data/hass.json always wins and the env
// vars are ignored with a warning; otherwise non-empty env values are
// persisted. A token without a URL is stored but stays not-configured
// (Config().ok == false) until a URL is set. The token value is never logged.
func (m *Manager) Bootstrap(envURL, envToken string) error {
	if envURL == "" && envToken == "" {
		return nil
	}
	if m.baseURLOrTokenSet() {
		slog.Warn("hass env bootstrap ignored, config already present — delete data/hass.json to reset")
		return nil
	}
	// A bare token without a URL is still persisted so a later SetConfig only
	// needs to supply the URL; validateBaseURL is skipped (no URL to validate).
	if envURL == "" {
		m.mu.Lock()
		defer m.mu.Unlock()
		if err := m.writeLocked("", envToken); err != nil {
			return fmt.Errorf("bootstrap hass config from environment: %w", err)
		}
		m.token = envToken
		slog.Info("hass credential set from environment; base URL still required")
		return nil
	}
	if err := m.SetConfig(envURL, envToken); err != nil {
		return fmt.Errorf("bootstrap hass config from environment: %w", err)
	}
	slog.Info("hass config set from environment")
	return nil
}

// baseURLOrTokenSet reports whether any part of the config is already present.
func (m *Manager) baseURLOrTokenSet() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.baseURL != "" || m.token != ""
}

// validateBaseURL enforces the scheme allowlist (http/https) and a non-empty
// host. An empty URL is rejected. The returned error never includes the token.
func validateBaseURL(baseURL string) error {
	if baseURL == "" {
		return errors.New("base_url must not be empty")
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return errors.New("base_url is not a valid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("base_url scheme must be http or https")
	}
	if u.Hostname() == "" {
		return errors.New("base_url host must not be empty")
	}
	return nil
}

// writeLocked atomically persists the configuration (tmp file + rename, mode
// 0600), byte-structurally identical to auth.Manager.writeLocked. Callers must
// hold m.mu.
func (m *Manager) writeLocked(baseURL, token string) error {
	data, err := json.Marshal(hassFile{BaseURL: baseURL, Token: token})
	if err != nil {
		return fmt.Errorf("marshal hass file: %w", err)
	}

	dir := filepath.Dir(m.path)
	tmp, err := os.CreateTemp(dir, hassFileName+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp hass file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp hass file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp hass file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp hass file: %w", err)
	}
	if err := os.Rename(tmpName, m.path); err != nil {
		return fmt.Errorf("rename hass file: %w", err)
	}
	return nil
}
