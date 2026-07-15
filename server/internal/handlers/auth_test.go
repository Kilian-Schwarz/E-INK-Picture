package handlers

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"e-ink-picture/server/internal/auth"
	"e-ink-picture/server/internal/middleware"
)

type authEnv struct {
	handler *AuthHandler
	manager *auth.Manager
	store   *auth.Store
	limiter *auth.RateLimiter
	dataDir string
	clock   *testClock
}

type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newAuthEnv(t *testing.T, cookieSecure bool) *authEnv {
	t.Helper()
	dir := t.TempDir()
	manager, err := auth.NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	store := auth.NewStore()
	limiter := auth.NewRateLimiter()
	clock := &testClock{t: time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)}
	limiter.SetClock(clock.Now)
	return &authEnv{
		handler: NewAuthHandler(manager, store, limiter, cookieSecure),
		manager: manager,
		store:   store,
		limiter: limiter,
		dataDir: dir,
		clock:   clock,
	}
}

func postJSON(handlerFunc http.HandlerFunc, path, body, remoteAddr string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	}
	w := httptest.NewRecorder()
	handlerFunc(w, req)
	return w
}

func captureAuthLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func sessionCookie(w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == middleware.SessionCookieName {
			return c
		}
	}
	return nil
}

// TestSetupBcryptRoundtrip covers AC3: file mode 0600, bcrypt hash, no
// plaintext in files or logs, second setup 403 with unchanged hash.
func TestSetupBcryptRoundtrip(t *testing.T) {
	logs := captureAuthLogs(t)
	env := newAuthEnv(t, false)
	const plaintext = "super-secret-pw"

	w := postJSON(env.handler.Setup, "/api/auth/setup", `{"password":"`+plaintext+`"}`, "")
	if w.Code != http.StatusOK {
		t.Fatalf("setup = %d, want 200", w.Code)
	}

	authPath := filepath.Join(env.dataDir, "auth.json")
	info, err := os.Stat(authPath)
	if err != nil {
		t.Fatalf("auth.json missing: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("auth.json mode = %o, want 0600", perm)
	}
	hashBefore, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("read auth.json: %v", err)
	}
	if !strings.Contains(string(hashBefore), "$2a$") && !strings.Contains(string(hashBefore), "$2b$") {
		t.Errorf("auth.json has no bcrypt hash: %s", hashBefore)
	}

	// Plaintext appears in no file under the data dir and not in the logs.
	err = filepath.WalkDir(env.dataDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), plaintext) {
			t.Errorf("plaintext password found in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk data dir: %v", err)
	}

	// Login (right and wrong) to also exercise the logging paths.
	postJSON(env.handler.Login, "/api/auth/login", `{"password":"wrong"}`, "")
	postJSON(env.handler.Login, "/api/auth/login", `{"password":"`+plaintext+`"}`, "")
	if strings.Contains(logs.String(), plaintext) {
		t.Error("plaintext password leaked into logs")
	}

	// Second setup call: 403, hash unchanged.
	w = postJSON(env.handler.Setup, "/api/auth/setup", `{"password":"other"}`, "")
	if w.Code != http.StatusForbidden {
		t.Errorf("second setup = %d, want 403", w.Code)
	}
	hashAfter, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("read auth.json: %v", err)
	}
	if !bytes.Equal(hashBefore, hashAfter) {
		t.Error("second setup modified the stored hash")
	}
}

func TestSetupRejectsEmptyPassword(t *testing.T) {
	env := newAuthEnv(t, false)
	if w := postJSON(env.handler.Setup, "/api/auth/setup", `{"password":""}`, ""); w.Code != http.StatusBadRequest {
		t.Errorf("empty password setup = %d, want 400", w.Code)
	}
	if env.manager.PasswordSet() {
		t.Error("empty password must not activate auth")
	}
}

// TestLoginCookieAttributes covers the AC3 cookie assertions.
func TestLoginCookieAttributes(t *testing.T) {
	env := newAuthEnv(t, false)
	if err := env.manager.SetPassword("pw"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	w := postJSON(env.handler.Login, "/api/auth/login", `{"password":"pw"}`, "")
	if w.Code != http.StatusOK {
		t.Fatalf("login = %d, want 200", w.Code)
	}
	c := sessionCookie(w)
	if c == nil {
		t.Fatal("no eink_session cookie set")
	}
	if !c.HttpOnly {
		t.Error("cookie not HttpOnly")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie SameSite = %v, want Lax", c.SameSite)
	}
	if c.Path != "/" {
		t.Errorf("cookie Path = %q, want /", c.Path)
	}
	if c.Secure {
		t.Error("cookie Secure set without TLS and without EINK_COOKIE_SECURE")
	}
	if c.MaxAge <= 0 {
		t.Errorf("cookie MaxAge = %d, want positive TTL", c.MaxAge)
	}
	if !env.store.Validate(c.Value) {
		t.Error("issued session token not valid in store")
	}

	// EINK_COOKIE_SECURE=true forces the Secure attribute.
	envSecure := newAuthEnv(t, true)
	if err := envSecure.manager.SetPassword("pw"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	w = postJSON(envSecure.handler.Login, "/api/auth/login", `{"password":"pw"}`, "")
	if c := sessionCookie(w); c == nil || !c.Secure {
		t.Error("EINK_COOKIE_SECURE=true must set the Secure attribute")
	}
}

func TestLoginWrongPassword(t *testing.T) {
	env := newAuthEnv(t, false)
	if err := env.manager.SetPassword("pw"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	w := postJSON(env.handler.Login, "/api/auth/login", `{"password":"nope"}`, "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("wrong password = %d, want 401", w.Code)
	}
	if sessionCookie(w) != nil {
		t.Error("failed login must not set a session cookie")
	}
}

func TestLoginWithoutPasswordSet(t *testing.T) {
	env := newAuthEnv(t, false)
	w := postJSON(env.handler.Login, "/api/auth/login", `{"password":"x"}`, "")
	if w.Code != http.StatusBadRequest {
		t.Errorf("login without password set = %d, want 400", w.Code)
	}
}

// TestLoginRateLimit covers AC4 for /api/auth/login.
func TestLoginRateLimit(t *testing.T) {
	env := newAuthEnv(t, false)
	if err := env.manager.SetPassword("pw"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	ipA := "10.1.1.1:40000"

	for i := 1; i <= 5; i++ {
		w := postJSON(env.handler.Login, "/api/auth/login", `{"password":"wrong"}`, ipA)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401", i, w.Code)
		}
	}
	// 6th attempt: 429 even with the CORRECT password.
	w := postJSON(env.handler.Login, "/api/auth/login", `{"password":"pw"}`, ipA)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("6th attempt = %d, want 429", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("429 without Retry-After header")
	}

	// Another IP is unaffected.
	w = postJSON(env.handler.Login, "/api/auth/login", `{"password":"pw"}`, "10.2.2.2:40000")
	if w.Code != http.StatusOK {
		t.Errorf("other IP = %d, want 200", w.Code)
	}

	// After the window expires (injected clock) attempts work again.
	env.clock.Advance(auth.RateLimitWindow)
	w = postJSON(env.handler.Login, "/api/auth/login", `{"password":"pw"}`, ipA)
	if w.Code != http.StatusOK {
		t.Errorf("after window expiry = %d, want 200", w.Code)
	}
}

// TestSetupRateLimit covers AC4 for /api/auth/setup.
func TestSetupRateLimit(t *testing.T) {
	env := newAuthEnv(t, false)
	ipA := "10.1.1.1:40000"

	// Keep the password unset by sending invalid bodies (400) — the limiter
	// counts attempts regardless of outcome.
	for i := 1; i <= 5; i++ {
		w := postJSON(env.handler.Setup, "/api/auth/setup", `{"password":""}`, ipA)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("attempt %d = %d, want 400", i, w.Code)
		}
	}
	w := postJSON(env.handler.Setup, "/api/auth/setup", `{"password":"valid"}`, ipA)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("6th setup attempt = %d, want 429", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("429 without Retry-After header")
	}

	// Another IP can still set up.
	w = postJSON(env.handler.Setup, "/api/auth/setup", `{"password":"valid"}`, "10.2.2.2:40000")
	if w.Code != http.StatusOK {
		t.Errorf("other IP setup = %d, want 200", w.Code)
	}

	env.clock.Advance(auth.RateLimitWindow)
	w = postJSON(env.handler.Setup, "/api/auth/setup", `{"password":"valid"}`, ipA)
	if w.Code != http.StatusForbidden {
		t.Errorf("setup after window with password set = %d, want 403", w.Code)
	}
}

// TestSetupConcurrentRequests: two parallel setup requests (distinct IPs,
// below the rate limit) → exactly one 200, the other the same 403 as a late
// sequential call. Run with -race.
func TestSetupConcurrentRequests(t *testing.T) {
	env := newAuthEnv(t, false)

	ips := []string{"10.1.1.1:40000", "10.2.2.2:40000"}
	codes := make([]int, len(ips))
	var wg sync.WaitGroup
	for i, ip := range ips {
		wg.Add(1)
		go func(i int, ip string) {
			defer wg.Done()
			w := postJSON(env.handler.Setup, "/api/auth/setup",
				`{"password":"pw-`+ip+`"}`, ip)
			codes[i] = w.Code
		}(i, ip)
	}
	wg.Wait()

	ok, forbidden := 0, 0
	for _, c := range codes {
		switch c {
		case http.StatusOK:
			ok++
		case http.StatusForbidden:
			forbidden++
		default:
			t.Errorf("unexpected status %d", c)
		}
	}
	if ok != 1 || forbidden != 1 {
		t.Errorf("got %d× 200 and %d× 403, want exactly 1 and 1", ok, forbidden)
	}
	if !env.manager.PasswordSet() {
		t.Error("no password set after concurrent setups")
	}
}

// TestAuthLogout covers the AC6 logout assertions at handler level.
func TestAuthLogout(t *testing.T) {
	env := newAuthEnv(t, false)
	if err := env.manager.SetPassword("pw"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	w := postJSON(env.handler.Login, "/api/auth/login", `{"password":"pw"}`, "")
	c := sessionCookie(w)
	if c == nil {
		t.Fatal("login did not set a cookie")
	}

	req := httptest.NewRequest("POST", "/api/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: middleware.SessionCookieName, Value: c.Value})
	rec := httptest.NewRecorder()
	env.handler.Logout(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout = %d, want 200", rec.Code)
	}
	cleared := sessionCookie(rec)
	if cleared == nil {
		t.Fatal("logout did not set a clearing cookie")
	}
	if cleared.MaxAge >= 0 {
		t.Errorf("clearing cookie MaxAge = %d, want < 0 (Max-Age=0 on the wire)", cleared.MaxAge)
	}
	if env.store.Validate(c.Value) {
		t.Error("session still valid after logout")
	}
}

func TestAuthStatus(t *testing.T) {
	env := newAuthEnv(t, false)

	get := func(cookie *http.Cookie) map[string]bool {
		req := httptest.NewRequest("GET", "/api/auth/status", nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		env.handler.Status(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		var resp map[string]bool
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode status: %v", err)
		}
		return resp
	}

	resp := get(nil)
	if resp["password_set"] || resp["authenticated"] {
		t.Errorf("fresh instance status = %v, want both false", resp)
	}

	if err := env.manager.SetPassword("pw"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	token, err := env.store.Create()
	if err != nil {
		t.Fatalf("store.Create: %v", err)
	}
	resp = get(&http.Cookie{Name: middleware.SessionCookieName, Value: token})
	if !resp["password_set"] || !resp["authenticated"] {
		t.Errorf("authenticated status = %v, want both true", resp)
	}
}
