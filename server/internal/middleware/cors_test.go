package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func TestResolveCORSOrigins(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		mode     string
		want     []string
		wantWarn string
	}{
		{"local empty", "", "local", nil, ""},
		{"local configured is ignored", "http://app.example", "local", nil, "ignored in local mode"},
		{"cloud empty", "", "cloud", nil, ""},
		{"cloud single", "http://app.example", "cloud", []string{"http://app.example"}, ""},
		{"cloud list with spaces", " http://a.example , https://b.example ", "cloud",
			[]string{"http://a.example", "https://b.example"}, ""},
		{"cloud wildcard downgrades", "*", "cloud", nil, "not allowed with credentials"},
		{"cloud wildcard in list downgrades", "http://a.example,*", "cloud", nil, "not allowed with credentials"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			logs := captureLogs(t)
			got := ResolveCORSOrigins(c.raw, c.mode)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("ResolveCORSOrigins(%q, %q) = %v, want %v", c.raw, c.mode, got, c.want)
			}
			if c.wantWarn != "" && !strings.Contains(logs.String(), c.wantWarn) {
				t.Errorf("expected warning containing %q, got logs: %s", c.wantWarn, logs.String())
			}
			if c.wantWarn == "" && strings.Contains(logs.String(), "level=WARN") {
				t.Errorf("unexpected warning: %s", logs.String())
			}
		})
	}
}

func corsHandler(origins []string) http.Handler {
	return CORS(origins)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}

func assertNoCORSHeaders(t *testing.T, h http.Header, context string) {
	t.Helper()
	for name := range h {
		if strings.HasPrefix(name, "Access-Control-") {
			t.Errorf("%s: unexpected CORS header %s: %q", context, name, h.Get(name))
		}
	}
}

func TestCORSLocalModeNoHeaders(t *testing.T) {
	// AC8: local mode (empty origin list) never emits Access-Control-*.
	h := corsHandler(nil)

	req := httptest.NewRequest("GET", "/designs", nil)
	req.Header.Set("Origin", "http://pi.local:5000")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	assertNoCORSHeaders(t, w.Header(), "GET local")

	// Preflight: 204 without Allow-Origin.
	req = httptest.NewRequest("OPTIONS", "/update_settings", nil)
	req.Header.Set("Origin", "http://pi.local:5000")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("OPTIONS = %d, want 204", w.Code)
	}
	assertNoCORSHeaders(t, w.Header(), "OPTIONS local")
}

func TestCORSCloudModeEcho(t *testing.T) {
	h := corsHandler([]string{"http://app.example"})

	req := httptest.NewRequest("GET", "/designs", nil)
	req.Header.Set("Origin", "http://app.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://app.example" {
		t.Errorf("Allow-Origin = %q, want exact echo", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Allow-Credentials = %q, want true", got)
	}
	if got := w.Header().Get("Vary"); !strings.Contains(got, "Origin") {
		t.Errorf("Vary = %q, want Origin", got)
	}
}

func TestCORSCloudModeUnlistedOrigin(t *testing.T) {
	h := corsHandler([]string{"http://app.example"})

	req := httptest.NewRequest("GET", "/designs", nil)
	req.Header.Set("Origin", "http://other.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	assertNoCORSHeaders(t, w.Header(), "unlisted origin")
}

func TestCORSCloudPreflight(t *testing.T) {
	h := corsHandler([]string{"http://app.example"})

	req := httptest.NewRequest("OPTIONS", "/api/designs/1", nil)
	req.Header.Set("Origin", "http://app.example")
	req.Header.Set("Access-Control-Request-Method", "DELETE")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("preflight status = %d, want 204", w.Code)
	}
	methods := w.Header().Get("Access-Control-Allow-Methods")
	for _, m := range []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"} {
		if !strings.Contains(methods, m) {
			t.Errorf("Allow-Methods %q missing %s", methods, m)
		}
	}
	headers := w.Header().Get("Access-Control-Allow-Headers")
	if !strings.Contains(headers, "Content-Type") || !strings.Contains(headers, ClientTokenHeader) {
		t.Errorf("Allow-Headers = %q, want Content-Type and %s", headers, ClientTokenHeader)
	}
}

func TestCORSNeverWildcardWithCredentials(t *testing.T) {
	// Defense in depth: even a wildcard that slipped past ResolveCORSOrigins
	// must never be echoed for an arbitrary origin.
	for _, origins := range [][]string{nil, {"http://app.example"}, ResolveCORSOrigins("*", "cloud")} {
		h := corsHandler(origins)
		req := httptest.NewRequest("POST", "/update_settings", nil)
		req.Header.Set("Origin", "http://attacker.example")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		allowOrigin := w.Header().Get("Access-Control-Allow-Origin")
		credentials := w.Header().Get("Access-Control-Allow-Credentials")
		if allowOrigin == "*" && credentials == "true" {
			t.Fatalf("Allow-Origin * combined with credentials (origins=%v)", origins)
		}
		if allowOrigin != "" {
			t.Errorf("unexpected Allow-Origin %q for unlisted origin (origins=%v)", allowOrigin, origins)
		}
	}
}
