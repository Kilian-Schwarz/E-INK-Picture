package auth

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// captureLogs redirects slog's default logger into a buffer for the duration
// of the test and returns it.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func TestNewManagerNoFile(t *testing.T) {
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if m.PasswordSet() {
		t.Error("expected PasswordSet=false without auth.json")
	}
	if m.Verify("anything") {
		t.Error("Verify must fail when no password is set")
	}
}

func TestSetPasswordRoundtrip(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := m.SetPassword("correct horse"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if !m.PasswordSet() {
		t.Error("expected PasswordSet=true")
	}
	if !m.Verify("correct horse") {
		t.Error("correct password rejected")
	}
	if m.Verify("wrong") {
		t.Error("wrong password accepted")
	}

	// Reload from disk: hash persists.
	m2, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager reload: %v", err)
	}
	if !m2.PasswordSet() || !m2.Verify("correct horse") {
		t.Error("password hash not persisted across reload")
	}
}

func TestSetPasswordEmptyRejected(t *testing.T) {
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := m.SetPassword(""); err == nil {
		t.Error("expected error for empty password")
	}
}

func TestAuthFileModeAndContent(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	const plaintext = "s3cret-plaintext"
	if err := m.SetPassword(plaintext); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	path := filepath.Join(dir, "auth.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat auth.json: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("auth.json mode = %o, want 0600", perm)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read auth.json: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "$2a$") && !strings.Contains(content, "$2b$") {
		t.Errorf("auth.json does not contain a bcrypt hash: %s", content)
	}
	if strings.Contains(content, plaintext) {
		t.Error("auth.json contains the plaintext password")
	}

	// No stray temp files left behind.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

func TestBootstrapEnvPassword(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := m.Bootstrap("env-password"); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if !m.PasswordSet() || !m.Verify("env-password") {
		t.Error("Bootstrap did not activate the env password")
	}
	if _, err := os.Stat(filepath.Join(dir, "auth.json")); err != nil {
		t.Errorf("auth.json not persisted: %v", err)
	}
}

func TestBootstrapIgnoresEnvWhenPasswordSet(t *testing.T) {
	logs := captureLogs(t)
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := m.SetPassword("original"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if err := m.Bootstrap("env-password"); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if !m.Verify("original") {
		t.Error("existing password was overwritten by env password")
	}
	if m.Verify("env-password") {
		t.Error("env password must not be active when auth.json exists")
	}
	if !strings.Contains(logs.String(), "EINK_ADMIN_PASSWORD ignored") {
		t.Error("expected warning that EINK_ADMIN_PASSWORD is ignored")
	}
}

func TestSetPasswordIfUnset(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := m.SetPasswordIfUnset(""); err == nil {
		t.Error("empty password must be rejected")
	}
	if err := m.SetPasswordIfUnset("first"); err != nil {
		t.Fatalf("first SetPasswordIfUnset: %v", err)
	}
	if err := m.SetPasswordIfUnset("second"); !errors.Is(err, ErrPasswordAlreadySet) {
		t.Errorf("second SetPasswordIfUnset error = %v, want ErrPasswordAlreadySet", err)
	}
	if !m.Verify("first") || m.Verify("second") {
		t.Error("losing call must not change the stored password")
	}
}

// TestSetPasswordIfUnsetConcurrent proves the check-and-set is atomic:
// N parallel setups → exactly one winner. Run with -race.
func TestSetPasswordIfUnsetConcurrent(t *testing.T) {
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	const n = 8
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = m.SetPasswordIfUnset(fmt.Sprintf("pw-%d", i))
		}(i)
	}
	wg.Wait()

	winner := -1
	for i, err := range errs {
		switch {
		case err == nil:
			if winner != -1 {
				t.Fatalf("two winners: #%d and #%d", winner, i)
			}
			winner = i
		case !errors.Is(err, ErrPasswordAlreadySet):
			t.Errorf("loser #%d: unexpected error %v", i, err)
		}
	}
	if winner == -1 {
		t.Fatal("no setup call succeeded")
	}
	if !m.Verify(fmt.Sprintf("pw-%d", winner)) {
		t.Error("winning password does not verify")
	}
}

func TestBootstrapEmptyEnvIsNoop(t *testing.T) {
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := m.Bootstrap(""); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if m.PasswordSet() {
		t.Error("empty env password must not activate auth")
	}
}
