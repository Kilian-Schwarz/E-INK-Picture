package hass

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sentinelToken is a fake token used across the security tests. It never
// contains a real value and is asserted absent from logs.
const sentinelToken = "SENTINEL_TOKEN_DO_NOT_LOG"

const testBaseURL = "http://10.0.0.5:8123"

// captureLogs redirects the default slog logger into a buffer so a test can
// assert the token never appears in any log line.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(orig) })
	return &buf
}

func TestSetConfigPersistsAndReloads(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := m.SetConfig(testBaseURL, sentinelToken); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	base, token, ok := m.Config()
	if !ok || base != testBaseURL || token != sentinelToken {
		t.Fatalf("Config() = (%q, <token>, %v), want (%q, <token>, true)", base, ok, testBaseURL)
	}

	// A fresh manager over the same dir must reload the persisted config.
	m2, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager reload: %v", err)
	}
	base2, token2, ok2 := m2.Config()
	if !ok2 || base2 != testBaseURL || token2 != sentinelToken {
		t.Fatalf("reloaded Config() = (%q, <token>, %v), want persisted values", base2, ok2)
	}
}

// TestConfigFileMode0600 is AC-SEC9: the token file is 0600 after SetConfig.
func TestConfigFileMode0600(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := m.SetConfig(testBaseURL, sentinelToken); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, hassFileName))
	if err != nil {
		t.Fatalf("stat hass.json: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("hass.json mode = %o, want 0600", perm)
	}
}

// TestSetConfigRejectsBadScheme is AC-SEC7(a): non-http(s)/empty base URLs are
// rejected and nothing is persisted.
func TestSetConfigRejectsBadScheme(t *testing.T) {
	cases := []string{
		"",
		"file:///etc/passwd",
		"gopher://10.0.0.5/",
		"ftp://10.0.0.5/",
		"://nohost",
	}
	for _, base := range cases {
		dir := t.TempDir()
		m, err := NewManager(dir)
		if err != nil {
			t.Fatalf("NewManager: %v", err)
		}
		if err := m.SetConfig(base, sentinelToken); err == nil {
			t.Errorf("SetConfig(%q) = nil error, want rejection", base)
		}
		if _, err := os.Stat(filepath.Join(dir, hassFileName)); !os.IsNotExist(err) {
			t.Errorf("SetConfig(%q) persisted a file, want nothing written", base)
		}
		if _, _, ok := m.Config(); ok {
			t.Errorf("SetConfig(%q) left manager configured, want not configured", base)
		}
	}
}

// TestStatusAndTokenSetNeverReturnToken asserts the UI-facing accessors expose
// presence flags and the base URL but never the token value.
func TestStatusAndTokenSetNeverReturnToken(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := m.SetConfig(testBaseURL, sentinelToken); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	base, configured := m.Status()
	if !configured || base != testBaseURL {
		t.Fatalf("Status() = (%q, %v), want (%q, true)", base, configured, testBaseURL)
	}
	if base == sentinelToken {
		t.Fatal("Status() base URL must never equal the token")
	}
	if !m.TokenSet() {
		t.Error("TokenSet() = false, want true after SetConfig")
	}
}

// TestConfigOkRequiresBoth: ok is true only when base URL AND token are set.
func TestConfigOkRequiresBoth(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	// URL set, token empty → not configured, but TokenSet false.
	if err := m.SetConfig(testBaseURL, ""); err != nil {
		t.Fatalf("SetConfig with empty token: %v", err)
	}
	if _, _, ok := m.Config(); ok {
		t.Error("Config() ok = true with empty token, want false")
	}
	if _, configured := m.Status(); configured {
		t.Error("Status() configured = true with empty token, want false")
	}
	if m.TokenSet() {
		t.Error("TokenSet() = true with empty token, want false")
	}
}

// TestSetConfigNeverLogsToken is AC-SEC2 (runtime, config path): the token
// never appears in any log line emitted while setting/bootstrapping config.
func TestSetConfigNeverLogsToken(t *testing.T) {
	buf := captureLogs(t)

	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := m.SetConfig(testBaseURL, sentinelToken); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	// A second manager over a fresh dir with an env bootstrap also logs.
	m2, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := m2.Bootstrap(testBaseURL, sentinelToken); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	// A second bootstrap on m2 hits the "already present" warning path.
	if err := m2.Bootstrap(testBaseURL, sentinelToken); err != nil {
		t.Fatalf("Bootstrap (second): %v", err)
	}

	if strings.Contains(buf.String(), sentinelToken) {
		t.Fatalf("token sentinel leaked into logs:\n%s", buf.String())
	}
}

// TestBootstrapExistingWins mirrors auth.Bootstrap: an existing config wins and
// the env vars are ignored; a token-only bootstrap is stored but not
// configured until a URL is set.
func TestBootstrapExistingWins(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := m.SetConfig(testBaseURL, sentinelToken); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	// Env values must be ignored because a config already exists.
	if err := m.Bootstrap("http://other.example:8123", "OTHER_TOKEN"); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	base, token, _ := m.Config()
	if base != testBaseURL || token != sentinelToken {
		t.Errorf("existing config overwritten by bootstrap: base=%q", base)
	}

	// Token-only bootstrap: stored but not configured (no URL yet).
	m2, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := m2.Bootstrap("", sentinelToken); err != nil {
		t.Fatalf("Bootstrap token-only: %v", err)
	}
	if _, _, ok := m2.Config(); ok {
		t.Error("token-only bootstrap left ok=true, want false until URL set")
	}
	if !m2.TokenSet() {
		t.Error("token-only bootstrap did not store the token")
	}
}

// TestBootstrapEmptyNoop: empty env values do not write a file.
func TestBootstrapEmptyNoop(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := m.Bootstrap("", ""); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, hassFileName)); !os.IsNotExist(err) {
		t.Error("empty bootstrap wrote a file, want no-op")
	}
}
