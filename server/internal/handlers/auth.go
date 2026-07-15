package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"

	"e-ink-picture/server/internal/auth"
	"e-ink-picture/server/internal/middleware"
)

// AuthHandler serves /api/auth/* (login, logout, setup, status).
type AuthHandler struct {
	manager      *auth.Manager
	sessions     *auth.Store
	limiter      *auth.RateLimiter
	cookieSecure bool
}

// NewAuthHandler creates an AuthHandler. cookieSecure forces the Secure
// cookie attribute (EINK_COOKIE_SECURE, for TLS-proxy setups); without it
// the attribute is set only for TLS requests.
func NewAuthHandler(manager *auth.Manager, sessions *auth.Store, limiter *auth.RateLimiter, cookieSecure bool) *AuthHandler {
	return &AuthHandler{
		manager:      manager,
		sessions:     sessions,
		limiter:      limiter,
		cookieSecure: cookieSecure,
	}
}

type passwordRequest struct {
	Password string `json:"password"`
}

// Login handles POST /api/auth/login: rate-limited bcrypt verify, issues a
// fresh session token (no fixation) and sets the session cookie.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !h.allowAttempt(w, ip) {
		return
	}

	var req passwordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if !h.manager.PasswordSet() {
		jsonError(w, "no password set", http.StatusBadRequest)
		return
	}
	if !h.manager.Verify(req.Password) {
		slog.Warn("failed login attempt", "ip", ip)
		jsonError(w, "invalid password", http.StatusUnauthorized)
		return
	}

	token, err := h.sessions.Create()
	if err != nil {
		slog.Error("failed to create session", "error", err)
		jsonError(w, "failed to create session", http.StatusInternalServerError)
		return
	}
	h.limiter.Reset(ip)
	h.setSessionCookie(w, r, token, int(auth.SessionTTL.Seconds()))
	jsonResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

// Logout handles POST /api/auth/logout: deletes the session server-side and
// expires the cookie. The route itself is session-protected by the guard.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(middleware.SessionCookieName); err == nil {
		h.sessions.Delete(cookie.Value)
	}
	h.setSessionCookie(w, r, "", -1)
	jsonResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

// Setup handles POST /api/auth/setup: sets the initial admin password.
// Once a password exists the endpoint is dead (403) — changing the password
// requires deleting data/auth.json (documented recovery path).
func (h *AuthHandler) Setup(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !h.allowAttempt(w, ip) {
		return
	}

	if h.manager.PasswordSet() {
		jsonError(w, "password already set", http.StatusForbidden)
		return
	}
	var req passwordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Password == "" {
		jsonError(w, "password must not be empty", http.StatusBadRequest)
		return
	}
	// Atomic check-and-set: concurrent setup requests race here, exactly one
	// wins — the loser gets the same 403 as a late sequential call.
	if err := h.manager.SetPasswordIfUnset(req.Password); err != nil {
		if errors.Is(err, auth.ErrPasswordAlreadySet) {
			jsonError(w, "password already set", http.StatusForbidden)
			return
		}
		slog.Error("failed to set admin password", "error", err)
		jsonError(w, "failed to set password", http.StatusInternalServerError)
		return
	}
	slog.Info("admin password set; authentication enabled", "ip", ip)
	jsonResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

// Status handles GET /api/auth/status (public): the login page and the
// designer banner need it before authentication.
func (h *AuthHandler) Status(w http.ResponseWriter, r *http.Request) {
	authenticated := false
	if cookie, err := r.Cookie(middleware.SessionCookieName); err == nil {
		authenticated = h.sessions.Validate(cookie.Value)
	}
	jsonResponse(w, http.StatusOK, map[string]bool{
		"password_set":  h.manager.PasswordSet(),
		"authenticated": authenticated,
	})
}

// allowAttempt applies the per-IP rate limit and writes the 429 response
// (with Retry-After) when exceeded — regardless of password correctness.
func (h *AuthHandler) allowAttempt(w http.ResponseWriter, ip string) bool {
	ok, retryAfter := h.limiter.Allow(ip)
	if ok {
		return true
	}
	slog.Warn("rate limit exceeded for auth endpoint", "ip", ip)
	w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retryAfter.Seconds()))))
	jsonError(w, "too many attempts, retry later", http.StatusTooManyRequests)
	return false
}

// setSessionCookie writes the eink_session cookie (HttpOnly, SameSite=Lax).
func (h *AuthHandler) setSessionCookie(w http.ResponseWriter, r *http.Request, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || h.cookieSecure,
	})
}

// clientIP extracts the host part of RemoteAddr. X-Forwarded-For is
// deliberately not parsed (spoofable without a trust concept, spec non-goal).
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
