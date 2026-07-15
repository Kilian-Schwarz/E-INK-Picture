package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"e-ink-picture/server/internal/config"
)

const (
	testPassword    = "full-stack-test-pw"
	testClientToken = "e2e-client-token"
	testHost        = "pi.local:5000"
)

// newTestConfig returns a config like config.Load() would produce on a fresh
// local-mode install, with an isolated data dir.
func newTestConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		Port:                 "5000",
		DataDir:              t.TempDir(),
		DeploymentMode:       "local",
		MaxConcurrentRenders: 1,
	}
}

// newTestApp builds the fully wired production stack (router + middleware
// chain from newApplication) for full-stack request tests.
func newTestApp(t *testing.T, mutate func(*config.Config)) *application {
	t.Helper()
	cfg := newTestConfig(t)
	if mutate != nil {
		mutate(cfg)
	}
	app, err := newApplication(cfg)
	if err != nil {
		t.Fatalf("newApplication: %v", err)
	}
	return app
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// do sends a request through the full middleware chain.
func do(app *application, method, path string, body io.Reader, mutate func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	req.Host = testHost
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if mutate != nil {
		mutate(req)
	}
	w := httptest.NewRecorder()
	app.handler.ServeHTTP(w, req)
	return w
}

// login performs a real login and returns the session cookie.
func login(t *testing.T, app *application, password string) *http.Cookie {
	t.Helper()
	w := do(app, "POST", "/api/auth/login", strings.NewReader(`{"password":"`+password+`"}`), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("login = %d, want 200", w.Code)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "eink_session" {
			return c
		}
	}
	t.Fatal("login did not set eink_session cookie")
	return nil
}

func withCookie(c *http.Cookie) func(*http.Request) {
	return func(r *http.Request) { r.AddCookie(c) }
}

func assertJSONMessage(t *testing.T, w *httptest.ResponseRecorder, context string) {
	t.Helper()
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("%s: Content-Type = %q, want application/json", context, ct)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil || body["message"] == "" {
		t.Errorf("%s: body is not a {\"message\":...} JSON error", context)
	}
}

// TestGuardFullStackClassification covers AC1: deny-by-default over the
// fully wired handler stack with a password set.
func TestGuardFullStackClassification(t *testing.T) {
	app := newTestApp(t, nil)
	if err := app.authMgr.SetPassword(testPassword); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	// (a) One route from each session group without session → 401 JSON.
	sessionRoutes := []struct{ method, path string }{
		{"GET", "/designs"},
		{"POST", "/update_settings"},
		{"DELETE", "/api/media/images/x.png"},
		{"GET", "/api/widgets/system"},
	}
	for _, c := range sessionRoutes {
		w := do(app, c.method, c.path, nil, nil)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("(a) %s %s = %d, want 401", c.method, c.path, w.Code)
			continue
		}
		assertJSONMessage(t, w, c.method+" "+c.path)
	}

	// (b) Page route without session → 302 to /login.
	w := do(app, "GET", "/designer", nil, nil)
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/login" {
		t.Errorf("(b) GET /designer = %d Location=%q, want 302 /login", w.Code, w.Header().Get("Location"))
	}

	// (c) Same routes with a valid session → business-logic status.
	cookie := login(t, app, testPassword)
	expected := []struct {
		method, path string
		body         string
		want         int
	}{
		{"GET", "/designs", "", http.StatusOK},
		{"POST", "/update_settings", `{"refresh_interval":1800}`, http.StatusOK},
		{"DELETE", "/api/media/images/x.png", "", http.StatusNotFound},
		{"GET", "/api/widgets/system", "", http.StatusOK},
		{"GET", "/designer", "", http.StatusOK},
	}
	for _, c := range expected {
		var body io.Reader
		if c.body != "" {
			body = strings.NewReader(c.body)
		}
		w := do(app, c.method, c.path, body, withCookie(cookie))
		if w.Code != c.want {
			t.Errorf("(c) %s %s with session = %d, want %d", c.method, c.path, w.Code, c.want)
		}
	}

	// (d) Unregistered path without session → 401, not 404: the guard sits
	// before the router match (deny by default).
	w = do(app, "GET", "/api/does_not_exist", nil, nil)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("(d) GET /api/does_not_exist = %d, want 401 (guard before router)", w.Code)
	}

	// (e) Public routes without session → 200.
	publicRoutes := []string{"/health", "/login", "/api/auth/status", "/api/setup/status", "/static/js/designer.js"}
	for _, path := range publicRoutes {
		w := do(app, "GET", path, nil, nil)
		if w.Code != http.StatusOK {
			t.Errorf("(e) GET %s = %d, want 200 (public)", path, w.Code)
		}
	}
}

// TestClientTokenFullStack covers AC2 over the fully wired stack.
func TestClientTokenFullStack(t *testing.T) {
	app := newTestApp(t, func(cfg *config.Config) { cfg.ClientToken = testClientToken })
	if err := app.authMgr.SetPassword(testPassword); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	clientRoutes := []struct{ method, path string }{
		{"GET", "/settings"},
		{"GET", "/preview"},
		{"GET", "/api/refresh_status"},
		{"POST", "/api/client_heartbeat"},
	}
	for _, c := range clientRoutes {
		w := do(app, c.method, c.path, nil, func(r *http.Request) {
			r.Header.Set("X-Client-Token", testClientToken)
		})
		if w.Code != http.StatusOK {
			t.Errorf("%s %s with token = %d, want 200", c.method, c.path, w.Code)
		}

		if w := do(app, c.method, c.path, nil, nil); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without token/session = %d, want 401", c.method, c.path, w.Code)
		}

		w = do(app, c.method, c.path, nil, func(r *http.Request) {
			r.Header.Set("X-Client-Token", "wrong-token")
		})
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with wrong token = %d, want 401", c.method, c.path, w.Code)
		}
	}

	// Session instead of token also works (the designer itself uses
	// /settings and /preview).
	cookie := login(t, app, testPassword)
	for _, c := range clientRoutes {
		w := do(app, c.method, c.path, nil, withCookie(cookie))
		if w.Code != http.StatusOK {
			t.Errorf("%s %s with session = %d, want 200", c.method, c.path, w.Code)
		}
	}

	// The client token is NOT a general key: session-only route stays 401.
	w := do(app, "POST", "/update_settings", strings.NewReader(`{"refresh_interval":900}`), func(r *http.Request) {
		r.Header.Set("X-Client-Token", testClientToken)
	})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("POST /update_settings with client token = %d, want 401", w.Code)
	}
}

// TestCSRFFullStack covers AC5: origin check for cookie-authenticated
// mutating requests, client-token requests exempt.
func TestCSRFFullStack(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.ClientToken = testClientToken
	app, err := newApplication(cfg)
	if err != nil {
		t.Fatalf("newApplication: %v", err)
	}
	if err := app.authMgr.SetPassword(testPassword); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	cookie := login(t, app, testPassword)

	// Legit update with own-host Origin → 200 (also creates settings.json).
	w := do(app, "POST", "/update_settings", strings.NewReader(`{"refresh_interval":1111}`), func(r *http.Request) {
		r.AddCookie(cookie)
		r.Header.Set("Origin", "http://"+testHost)
	})
	if w.Code != http.StatusOK {
		t.Fatalf("own-host origin update = %d, want 200", w.Code)
	}

	settingsPath := filepath.Join(cfg.DataDir, "settings.json")
	before, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}

	// Foreign Origin with valid cookie → 403, settings.json unchanged.
	w = do(app, "POST", "/update_settings", strings.NewReader(`{"refresh_interval":2222}`), func(r *http.Request) {
		r.AddCookie(cookie)
		r.Header.Set("Origin", "http://evil.example")
	})
	if w.Code != http.StatusForbidden {
		t.Errorf("evil origin = %d, want 403", w.Code)
	}
	after, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("settings.json changed despite 403 CSRF rejection")
	}

	// No Origin, Sec-Fetch-Site: cross-site → 403.
	w = do(app, "POST", "/update_settings", strings.NewReader(`{"refresh_interval":3333}`), func(r *http.Request) {
		r.AddCookie(cookie)
		r.Header.Set("Sec-Fetch-Site", "cross-site")
	})
	if w.Code != http.StatusForbidden {
		t.Errorf("Sec-Fetch-Site cross-site = %d, want 403", w.Code)
	}

	// Neither header (old client) → 200, SameSite=Lax carries alone.
	w = do(app, "POST", "/update_settings", strings.NewReader(`{"refresh_interval":4444}`), withCookie(cookie))
	if w.Code != http.StatusOK {
		t.Errorf("no origin headers = %d, want 200", w.Code)
	}

	// Client-token request (no cookie) with foreign Origin → 200 (exempt).
	w = do(app, "POST", "/api/client_heartbeat", nil, func(r *http.Request) {
		r.Header.Set("X-Client-Token", testClientToken)
		r.Header.Set("Origin", "http://evil.example")
	})
	if w.Code != http.StatusOK {
		t.Errorf("client-token request with foreign origin = %d, want 200", w.Code)
	}
}

// TestLogoutFullStack covers the AC6 logout flow over the wired stack.
func TestLogoutFullStack(t *testing.T) {
	app := newTestApp(t, nil)
	if err := app.authMgr.SetPassword(testPassword); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	cookie := login(t, app, testPassword)

	if w := do(app, "GET", "/designs", nil, withCookie(cookie)); w.Code != http.StatusOK {
		t.Fatalf("session request before logout = %d, want 200", w.Code)
	}

	w := do(app, "POST", "/api/auth/logout", nil, withCookie(cookie))
	if w.Code != http.StatusOK {
		t.Fatalf("logout = %d, want 200", w.Code)
	}
	var cleared *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "eink_session" {
			cleared = c
		}
	}
	if cleared == nil || cleared.MaxAge >= 0 {
		t.Error("logout did not clear the cookie with Max-Age=0")
	}

	if w := do(app, "GET", "/designs", nil, withCookie(cookie)); w.Code != http.StatusUnauthorized {
		t.Errorf("old cookie after logout = %d, want 401", w.Code)
	}
}

// TestUpgradePathNoPassword covers AC7: without a password nothing is
// blocked and today's status codes are preserved.
func TestUpgradePathNoPassword(t *testing.T) {
	logs := captureLogs(t)
	app := newTestApp(t, nil)

	if !strings.Contains(logs.String(), "authentication disabled") {
		t.Error("startup warning about disabled authentication not logged")
	}

	routes := []struct {
		method, path string
		body         string
		want         int
	}{
		{"GET", "/", "", http.StatusFound},      // → /designer
		{"GET", "/designer", "", http.StatusOK}, // page served
		{"GET", "/media", "", http.StatusFound}, // → /designer#media-images
		{"GET", "/designs", "", http.StatusOK},
		{"POST", "/update_settings", `{"refresh_interval":1800}`, http.StatusOK},
		{"GET", "/settings", "", http.StatusOK},
		{"GET", "/api/refresh_status", "", http.StatusOK},
		{"POST", "/api/client_heartbeat", "", http.StatusOK},
		{"DELETE", "/api/media/images/x.png", "", http.StatusNotFound},
		{"GET", "/api/does_not_exist", "", http.StatusNotFound}, // router 404, no guard 401
		{"GET", "/health", "", http.StatusOK},
		{"GET", "/static/js/designer.js", "", http.StatusOK},
	}
	for _, c := range routes {
		var body io.Reader
		if c.body != "" {
			body = strings.NewReader(c.body)
		}
		w := do(app, c.method, c.path, body, nil)
		if w.Code != c.want {
			t.Errorf("%s %s without password = %d, want %d (today's behavior)", c.method, c.path, w.Code, c.want)
		}
	}

	w := do(app, "GET", "/api/auth/status", nil, nil)
	var status map[string]bool
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status["password_set"] {
		t.Error("password_set = true on fresh instance, want false")
	}
}

// TestUpgradePathEnvPassword covers AC7: EINK_ADMIN_PASSWORD bootstraps the
// hash; an existing auth.json wins over the env var.
func TestUpgradePathEnvPassword(t *testing.T) {
	dataDir := t.TempDir()

	// No auth.json + env password → hash persisted, auth active.
	cfg := newTestConfig(t)
	cfg.DataDir = dataDir
	cfg.AdminPassword = "env-secret"
	app, err := newApplication(cfg)
	if err != nil {
		t.Fatalf("newApplication: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "auth.json")); err != nil {
		t.Fatalf("auth.json not created from EINK_ADMIN_PASSWORD: %v", err)
	}
	login(t, app, "env-secret")
	if w := do(app, "GET", "/designs", nil, nil); w.Code != http.StatusUnauthorized {
		t.Errorf("protected route without session = %d, want 401 (auth active)", w.Code)
	}

	// Existing auth.json + different env password → env ignored with warning.
	logs := captureLogs(t)
	cfg2 := newTestConfig(t)
	cfg2.DataDir = dataDir
	cfg2.AdminPassword = "other-password"
	app2, err := newApplication(cfg2)
	if err != nil {
		t.Fatalf("newApplication: %v", err)
	}
	if !strings.Contains(logs.String(), "EINK_ADMIN_PASSWORD ignored") {
		t.Error("expected warning that EINK_ADMIN_PASSWORD is ignored")
	}
	login(t, app2, "env-secret") // old password still valid
	w := do(app2, "POST", "/api/auth/login", strings.NewReader(`{"password":"other-password"}`), nil)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("ignored env password logged in = %d, want 401", w.Code)
	}
}

// TestSetupThenLoginFullStack wires AC3 end-to-end: setup over the public
// route, then login and access.
func TestSetupThenLoginFullStack(t *testing.T) {
	app := newTestApp(t, nil)

	w := do(app, "POST", "/api/auth/setup", strings.NewReader(`{"password":"first-pw"}`), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("setup = %d, want 200", w.Code)
	}
	if w := do(app, "GET", "/designs", nil, nil); w.Code != http.StatusUnauthorized {
		t.Errorf("protected route after setup without session = %d, want 401", w.Code)
	}
	cookie := login(t, app, "first-pw")
	if w := do(app, "GET", "/designs", nil, withCookie(cookie)); w.Code != http.StatusOK {
		t.Errorf("protected route with session = %d, want 200", w.Code)
	}

	// Setup endpoint is dead once a password exists.
	w = do(app, "POST", "/api/auth/setup", strings.NewReader(`{"password":"second-pw"}`), nil)
	if w.Code != http.StatusForbidden {
		t.Errorf("second setup = %d, want 403", w.Code)
	}
}

// TestSetupStatusPublicFullStack (E2.3 AC2): GET /api/setup/status answers
// through the fully wired stack without a session even when a password is
// set, and carries exactly the five spec'd fields — nothing sensitive.
func TestSetupStatusPublicFullStack(t *testing.T) {
	app := newTestApp(t, nil)

	// Fresh install (newApplication ran EnsureDesignExists): wizard on.
	w := do(app, "GET", "/api/setup/status", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("fresh: GET /api/setup/status = %d, want 200", w.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["wizard"] != true || resp["password_set"] != false {
		t.Errorf("fresh: wizard=%v password_set=%v, want true/false", resp["wizard"], resp["password_set"])
	}

	// With a password and WITHOUT a session the route stays reachable
	// (public allowlist) and reports wizard=false.
	if err := app.authMgr.SetPassword(testPassword); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	w = do(app, "GET", "/api/setup/status", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("with password, no session: GET /api/setup/status = %d, want 200", w.Code)
	}
	resp = map[string]any{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["wizard"] != false || resp["password_set"] != true {
		t.Errorf("with password: wizard=%v password_set=%v, want false/true", resp["wizard"], resp["password_set"])
	}
	want := []string{"wizard", "password_set", "setup_completed", "server_time", "server_timezone"}
	for _, key := range want {
		if _, ok := resp[key]; !ok {
			t.Errorf("response is missing field %q", key)
		}
	}
	if len(resp) != len(want) {
		t.Errorf("response must contain exactly %d fields, got %d: %v", len(want), len(resp), resp)
	}
}

// TestCORSFullStackLocalMode covers AC8 for the default local deployment:
// no Access-Control-* headers anywhere, preflight answers 204 bare.
func TestCORSFullStackLocalMode(t *testing.T) {
	app := newTestApp(t, func(cfg *config.Config) {
		cfg.CORSAllowedOrigins = "" // default
	})

	w := do(app, "GET", "/health", nil, func(r *http.Request) {
		r.Header.Set("Origin", "http://"+testHost)
	})
	for name := range w.Header() {
		if strings.HasPrefix(name, "Access-Control-") {
			t.Errorf("local mode: unexpected CORS header %s", name)
		}
	}

	w = do(app, "OPTIONS", "/update_settings", nil, func(r *http.Request) {
		r.Header.Set("Origin", "http://"+testHost)
		r.Header.Set("Access-Control-Request-Method", "POST")
	})
	if w.Code != http.StatusNoContent {
		t.Errorf("preflight = %d, want 204", w.Code)
	}
	for name := range w.Header() {
		if strings.HasPrefix(name, "Access-Control-") {
			t.Errorf("local preflight: unexpected CORS header %s", name)
		}
	}
}

// TestCORSFullStackCloudMode covers AC8 for cloud mode with an explicit
// origin list wired through config → middleware.
func TestCORSFullStackCloudMode(t *testing.T) {
	app := newTestApp(t, func(cfg *config.Config) {
		cfg.DeploymentMode = "cloud"
		cfg.CORSAllowedOrigins = "http://app.example"
	})

	w := do(app, "GET", "/health", nil, func(r *http.Request) {
		r.Header.Set("Origin", "http://app.example")
	})
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://app.example" {
		t.Errorf("Allow-Origin = %q, want echo of configured origin", got)
	}
	if w.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Error("Allow-Credentials missing")
	}
	if !strings.Contains(w.Header().Get("Vary"), "Origin") {
		t.Error("Vary: Origin missing")
	}

	w = do(app, "GET", "/health", nil, func(r *http.Request) {
		r.Header.Set("Origin", "http://other.example")
	})
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("unlisted origin must not get CORS headers")
	}

	// "*" is rejected and downgrades to same-origin only.
	logs := captureLogs(t)
	appStar := newTestApp(t, func(cfg *config.Config) {
		cfg.DeploymentMode = "cloud"
		cfg.CORSAllowedOrigins = "*"
	})
	if !strings.Contains(logs.String(), "not allowed with credentials") {
		t.Error("expected startup warning for CORS_ALLOWED_ORIGINS=*")
	}
	w = do(appStar, "GET", "/health", nil, func(r *http.Request) {
		r.Header.Set("Origin", "http://app.example")
	})
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("wildcard config must behave as unconfigured")
	}
}

// TestHealthVersionDefault proves /health served through the full production
// stack reports the unstamped default version "dev"
// (specs/E6.2-release-workflow.md AC1).
func TestHealthVersionDefault(t *testing.T) {
	app := newTestApp(t, nil)
	w := do(app, "GET", "/health", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /health = %d, want 200", w.Code)
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status = %q, want %q", resp["status"], "ok")
	}
	if resp["version"] != "dev" {
		t.Errorf("version = %q, want %q", resp["version"], "dev")
	}
}

// TestVersionLdflagsStamp proves the version variable is stampable at build
// time: it builds the server with -ldflags "-X main.version=vTEST" and
// verifies the stamp via `go version -m` on the resulting binary — buildinfo
// survives even -s -w stripping (specs/E6.2-release-workflow.md AC1, Fakt 4).
func TestVersionLdflagsStamp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping build test in short mode")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not in PATH")
	}

	binPath := filepath.Join(t.TempDir(), "server-stamped")
	build := exec.Command(goBin, "build", "-ldflags", "-s -w -X main.version=vTEST", "-o", binPath, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build with ldflags stamp failed: %v\n%s", err, out)
	}

	out, err := exec.Command(goBin, "version", "-m", binPath).CombinedOutput()
	if err != nil {
		t.Fatalf("go version -m failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "main.version=vTEST") {
		t.Errorf("go version -m output missing ldflags stamp main.version=vTEST:\n%s", out)
	}
}
