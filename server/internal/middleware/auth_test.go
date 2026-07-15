package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"e-ink-picture/server/internal/auth"
)

const testClientToken = "test-client-token-hex"

// newGuardEnv returns a guard-wrapped OK handler plus manager and store.
// withPassword controls whether auth is active.
func newGuardEnv(t *testing.T, withPassword bool) (http.Handler, *auth.Manager, *auth.Store) {
	t.Helper()
	mgr, err := auth.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if withPassword {
		if err := mgr.SetPassword("test-password"); err != nil {
			t.Fatalf("SetPassword: %v", err)
		}
	}
	store := auth.NewStore()
	guard := Guard(GuardConfig{
		Manager:     mgr,
		Sessions:    store,
		ClientToken: testClientToken,
	})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	return guard(next), mgr, store
}

func newSession(t *testing.T, store *auth.Store) *http.Cookie {
	t.Helper()
	token, err := store.Create()
	if err != nil {
		t.Fatalf("store.Create: %v", err)
	}
	return &http.Cookie{Name: SessionCookieName, Value: token}
}

func doGuard(h http.Handler, method, path string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Host = "pi.local:5000"
	if mutate != nil {
		mutate(req)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestGuardPublicRoutesWithoutSession(t *testing.T) {
	h, _, _ := newGuardEnv(t, true)
	cases := []struct{ method, path string }{
		{"GET", "/health"},
		{"GET", "/login"},
		{"POST", "/api/auth/login"},
		{"POST", "/api/auth/setup"},
		{"GET", "/api/auth/status"},
		{"GET", "/static/js/designer.js"},
		{"GET", "/static/css/designer.css"},
	}
	for _, c := range cases {
		if w := doGuard(h, c.method, c.path, nil); w.Code != http.StatusOK {
			t.Errorf("%s %s = %d, want 200 (public)", c.method, c.path, w.Code)
		}
	}
}

func TestGuardDenyByDefault(t *testing.T) {
	h, _, _ := newGuardEnv(t, true)
	cases := []struct{ method, path string }{
		{"GET", "/designs"},
		{"POST", "/update_settings"},
		{"DELETE", "/api/media/images/x.png"},
		{"GET", "/api/widgets/system"},
		{"POST", "/api/auth/logout"},
		{"GET", "/api/does_not_exist"}, // unregistered: guard rejects before any router match
		{"POST", "/upload_image"},
	}
	for _, c := range cases {
		w := doGuard(h, c.method, c.path, nil)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401", c.method, c.path, w.Code)
			continue
		}
		if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Errorf("%s %s Content-Type = %q, want application/json", c.method, c.path, ct)
		}
		var body map[string]string
		if err := json.NewDecoder(w.Body).Decode(&body); err != nil || body["message"] == "" {
			t.Errorf("%s %s: body is not a {\"message\":...} JSON error", c.method, c.path)
		}
	}
}

func TestGuardPageRoutesRedirectToLogin(t *testing.T) {
	h, _, _ := newGuardEnv(t, true)
	for _, path := range []string{"/", "/designer", "/media"} {
		w := doGuard(h, "GET", path, nil)
		if w.Code != http.StatusFound {
			t.Errorf("GET %s = %d, want 302", path, w.Code)
		}
		if loc := w.Header().Get("Location"); loc != "/login" {
			t.Errorf("GET %s Location = %q, want /login", path, loc)
		}
	}
}

func TestGuardValidSessionPasses(t *testing.T) {
	h, _, store := newGuardEnv(t, true)
	cookie := newSession(t, store)
	cases := []struct{ method, path string }{
		{"GET", "/designs"},
		{"GET", "/designer"},
		{"GET", "/settings"},
		{"GET", "/preview"},
	}
	for _, c := range cases {
		w := doGuard(h, c.method, c.path, func(r *http.Request) { r.AddCookie(cookie) })
		if w.Code != http.StatusOK {
			t.Errorf("%s %s with session = %d, want 200", c.method, c.path, w.Code)
		}
	}
}

func TestGuardSessionExpiryOnNextRequest(t *testing.T) {
	h, _, store := newGuardEnv(t, true)
	clock := struct {
		mu sync.Mutex
		t  time.Time
	}{t: time.Now()}
	store.SetClock(func() time.Time {
		clock.mu.Lock()
		defer clock.mu.Unlock()
		return clock.t
	})
	cookie := newSession(t, store)

	if w := doGuard(h, "GET", "/designs", func(r *http.Request) { r.AddCookie(cookie) }); w.Code != http.StatusOK {
		t.Fatalf("fresh session rejected: %d", w.Code)
	}
	clock.mu.Lock()
	clock.t = clock.t.Add(auth.SessionTTL + time.Minute)
	clock.mu.Unlock()
	if w := doGuard(h, "GET", "/designs", func(r *http.Request) { r.AddCookie(cookie) }); w.Code != http.StatusUnauthorized {
		t.Errorf("expired session = %d, want 401", w.Code)
	}
	if store.Len() != 0 {
		t.Errorf("expired session not removed, store len = %d", store.Len())
	}
}

func TestGuardClientToken(t *testing.T) {
	h, _, store := newGuardEnv(t, true)
	clientRoutes := []struct{ method, path string }{
		{"GET", "/settings"},
		{"GET", "/preview"},
		{"GET", "/api/refresh_status"},
		{"POST", "/api/client_heartbeat"},
	}
	for _, c := range clientRoutes {
		w := doGuard(h, c.method, c.path, func(r *http.Request) {
			r.Header.Set(ClientTokenHeader, testClientToken)
		})
		if w.Code != http.StatusOK {
			t.Errorf("%s %s with token = %d, want 200", c.method, c.path, w.Code)
		}
		if w := doGuard(h, c.method, c.path, nil); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without token/session = %d, want 401", c.method, c.path, w.Code)
		}
		w = doGuard(h, c.method, c.path, func(r *http.Request) {
			r.Header.Set(ClientTokenHeader, "wrong-token")
		})
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with wrong token = %d, want 401", c.method, c.path, w.Code)
		}
	}

	// Session instead of token also works on client routes.
	cookie := newSession(t, store)
	for _, c := range clientRoutes {
		w := doGuard(h, c.method, c.path, func(r *http.Request) { r.AddCookie(cookie) })
		if w.Code != http.StatusOK {
			t.Errorf("%s %s with session = %d, want 200", c.method, c.path, w.Code)
		}
	}
}

func TestGuardClientTokenIsNoGeneralKey(t *testing.T) {
	h, _, _ := newGuardEnv(t, true)
	// AC2: a valid client token must NOT open session-only routes.
	w := doGuard(h, "POST", "/update_settings", func(r *http.Request) {
		r.Header.Set(ClientTokenHeader, testClientToken)
	})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("POST /update_settings with client token = %d, want 401", w.Code)
	}
}

func TestGuardEmptyClientTokenFailsClosed(t *testing.T) {
	// Auth active but EINK_CLIENT_TOKEN unset: client endpoints require a
	// session — no silent bypass via empty header comparison.
	mgr, err := auth.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := mgr.SetPassword("pw"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	h := Guard(GuardConfig{Manager: mgr, Sessions: auth.NewStore(), ClientToken: ""})(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))

	w := doGuard(h, "GET", "/settings", func(r *http.Request) { r.Header.Set(ClientTokenHeader, "") })
	if w.Code != http.StatusUnauthorized {
		t.Errorf("GET /settings with empty configured token = %d, want 401", w.Code)
	}
}

func TestGuardCSRFOriginCheck(t *testing.T) {
	h, _, store := newGuardEnv(t, true)
	cookie := newSession(t, store)

	cases := []struct {
		name   string
		mutate func(*http.Request)
		want   int
	}{
		{"evil origin", func(r *http.Request) {
			r.AddCookie(cookie)
			r.Header.Set("Origin", "http://evil.example")
		}, http.StatusForbidden},
		{"own host origin", func(r *http.Request) {
			r.AddCookie(cookie)
			r.Header.Set("Origin", "http://pi.local:5000")
		}, http.StatusOK},
		{"own host https origin", func(r *http.Request) {
			r.AddCookie(cookie)
			r.Header.Set("Origin", "https://pi.local:5000")
		}, http.StatusOK},
		{"no origin, sec-fetch-site cross-site", func(r *http.Request) {
			r.AddCookie(cookie)
			r.Header.Set("Sec-Fetch-Site", "cross-site")
		}, http.StatusForbidden},
		{"no origin, sec-fetch-site same-origin", func(r *http.Request) {
			r.AddCookie(cookie)
			r.Header.Set("Sec-Fetch-Site", "same-origin")
		}, http.StatusOK},
		{"no origin, sec-fetch-site none", func(r *http.Request) {
			r.AddCookie(cookie)
			r.Header.Set("Sec-Fetch-Site", "none")
		}, http.StatusOK},
		{"neither header (old client)", func(r *http.Request) {
			r.AddCookie(cookie)
		}, http.StatusOK},
	}
	for _, c := range cases {
		w := doGuard(h, "POST", "/update_settings", c.mutate)
		if w.Code != c.want {
			t.Errorf("%s: status = %d, want %d", c.name, w.Code, c.want)
		}
	}

	// GET with foreign origin is not a CSRF vector — no origin check.
	w := doGuard(h, "GET", "/designs", func(r *http.Request) {
		r.AddCookie(cookie)
		r.Header.Set("Origin", "http://evil.example")
	})
	if w.Code != http.StatusOK {
		t.Errorf("GET with foreign origin = %d, want 200 (origin check only for mutating)", w.Code)
	}
}

func TestGuardCSRFCloudOriginList(t *testing.T) {
	mgr, err := auth.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := mgr.SetPassword("pw"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	store := auth.NewStore()
	h := Guard(GuardConfig{
		Manager:        mgr,
		Sessions:       store,
		AllowedOrigins: []string{"http://app.example"},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	cookie := newSession(t, store)

	w := doGuard(h, "POST", "/update_settings", func(r *http.Request) {
		r.AddCookie(cookie)
		r.Header.Set("Origin", "http://app.example")
	})
	if w.Code != http.StatusOK {
		t.Errorf("configured cloud origin = %d, want 200", w.Code)
	}
	w = doGuard(h, "POST", "/update_settings", func(r *http.Request) {
		r.AddCookie(cookie)
		r.Header.Set("Origin", "http://other.example")
	})
	if w.Code != http.StatusForbidden {
		t.Errorf("unlisted cloud origin = %d, want 403", w.Code)
	}
}

func TestGuardClientTokenExemptFromOriginCheck(t *testing.T) {
	h, _, _ := newGuardEnv(t, true)
	// AC5: token-authenticated request (no cookie) with foreign origin → 200.
	w := doGuard(h, "POST", "/api/client_heartbeat", func(r *http.Request) {
		r.Header.Set(ClientTokenHeader, testClientToken)
		r.Header.Set("Origin", "http://evil.example")
	})
	if w.Code != http.StatusOK {
		t.Errorf("client-token request with foreign origin = %d, want 200 (exempt)", w.Code)
	}
}

func TestGuardDisabledWithoutPassword(t *testing.T) {
	h, _, _ := newGuardEnv(t, false)
	// AC7: no password set — everything behaves like today.
	cases := []struct{ method, path string }{
		{"GET", "/designs"},
		{"GET", "/designer"},
		{"POST", "/update_settings"},
		{"DELETE", "/api/media/images/x.png"},
		{"GET", "/settings"},
		{"POST", "/api/client_heartbeat"},
		{"GET", "/api/does_not_exist"},
	}
	for _, c := range cases {
		if w := doGuard(h, c.method, c.path, nil); w.Code != http.StatusOK {
			t.Errorf("%s %s without password = %d, want 200 (auth disabled)", c.method, c.path, w.Code)
		}
	}
}

func TestGuardOptionsPassThrough(t *testing.T) {
	h, _, _ := newGuardEnv(t, true)
	if w := doGuard(h, "OPTIONS", "/update_settings", nil); w.Code != http.StatusOK {
		t.Errorf("OPTIONS = %d, want pass-through (preflights carry no cookies)", w.Code)
	}
}
