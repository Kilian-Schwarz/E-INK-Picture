package middleware

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"e-ink-picture/server/internal/auth"
)

// SessionCookieName is the session cookie issued on login.
const SessionCookieName = "eink_session"

// ClientTokenHeader authenticates the headless e-ink client on the four
// client endpoints.
const ClientTokenHeader = "X-Client-Token"

// GuardConfig wires the deny-by-default guard middleware.
type GuardConfig struct {
	// Manager decides whether auth is active (password set) and is never nil.
	Manager *auth.Manager
	// Sessions validates and renews session cookies.
	Sessions *auth.Store
	// ClientToken is EINK_CLIENT_TOKEN; empty means the client endpoints
	// require a session (fail closed, spec decision 1).
	ClientToken string
	// AllowedOrigins are the exact cloud-mode origins (resolved via
	// ResolveCORSOrigins) additionally accepted by the CSRF origin check.
	AllowedOrigins []string
}

// publicRoutes is the exact-match public allowlist (method + path). Everything
// not listed here (or under /static/) requires authentication once a password
// is set — deny by default, the guard runs before any router match.
var publicRoutes = map[string]bool{
	"GET /health":           true,
	"GET /login":            true,
	"POST /api/auth/login":  true,
	"POST /api/auth/setup":  true,
	"GET /api/auth/status":  true,
	"GET /api/setup/status": true,
}

// staticPrefix serves embedded CSS/JS (no user data); the login page needs it.
const staticPrefix = "/static/"

// clientRoutes are exactly the four endpoints the Python e-ink client polls
// (spec Fakt 3); they accept the client token OR a session.
var clientRoutes = map[string]bool{
	"GET /settings":              true,
	"GET /preview":               true,
	"GET /api/refresh_status":    true,
	"POST /api/client_heartbeat": true,
}

// pageRoutes redirect to /login instead of returning 401 JSON.
var pageRoutes = map[string]bool{
	"GET /":         true,
	"GET /designer": true,
	"GET /media":    true,
}

// Guard is the single auth+CSRF middleware (spec Architektur-Richtung 10).
// Check order: OPTIONS pass-through (CORS answered), public allowlist,
// auth-disabled pass-through, client token for the four client routes,
// session cookie, then origin check for cookie-authenticated mutating
// requests.
func Guard(cfg GuardConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// (1) Preflights carry no cookies; CORS middleware already
			// answered them. Defensive pass-through if reached.
			if r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			key := r.Method + " " + r.URL.Path

			// (2) Public allowlist.
			if publicRoutes[key] {
				next.ServeHTTP(w, r)
				return
			}
			if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, staticPrefix) {
				next.ServeHTTP(w, r)
				return
			}

			// (3) No password set: auth disabled, behave exactly like today
			// (upgrade path without lockout, spec decision 4).
			if !cfg.Manager.PasswordSet() {
				next.ServeHTTP(w, r)
				return
			}

			// (4) Client token for the four client endpoints. Not a general
			// key: only these routes, and only when a token is configured.
			if clientRoutes[key] && cfg.ClientToken != "" {
				token := r.Header.Get(ClientTokenHeader)
				if token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(cfg.ClientToken)) == 1 {
					// Token-authenticated requests carry no cookie — no CSRF
					// vector, origin check exempt.
					next.ServeHTTP(w, r)
					return
				}
			}

			// (5) Session cookie (lookup + sliding renewal).
			if !hasValidSession(cfg.Sessions, r) {
				if pageRoutes[key] {
					http.Redirect(w, r, "/login", http.StatusFound)
					return
				}
				writeJSONError(w, http.StatusUnauthorized, "authentication required")
				return
			}

			// (6) CSRF origin check for cookie-authenticated mutating
			// requests (SameSite=Lax is the first layer, this the second).
			if isMutating(r.Method) && !originAllowed(r, cfg.AllowedOrigins) {
				writeJSONError(w, http.StatusForbidden, "cross-origin request rejected")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// hasValidSession validates (and slides) the session cookie.
func hasValidSession(store *auth.Store, r *http.Request) bool {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return false
	}
	return store.Validate(cookie.Value)
}

// isMutating reports whether the method requires the CSRF origin check.
func isMutating(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}

// originAllowed implements the browser-independent CSRF layer: a present
// Origin header must exactly match the request host (either scheme) or the
// configured cloud origins; without Origin, a present Sec-Fetch-Site must be
// same-origin/none; with neither header (very old clients) the request
// passes — SameSite=Lax carries alone.
func originAllowed(r *http.Request, allowedOrigins []string) bool {
	origin := r.Header.Get("Origin")
	if origin != "" {
		if origin == "http://"+r.Host || origin == "https://"+r.Host {
			return true
		}
		for _, allowed := range allowedOrigins {
			if origin == allowed {
				return true
			}
		}
		return false
	}
	switch r.Header.Get("Sec-Fetch-Site") {
	case "", "same-origin", "none":
		return true
	}
	return false
}

// writeJSONError mirrors the handlers' jsonError format ({"message":...}).
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"message": message})
}
