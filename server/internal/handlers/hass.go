package handlers

// HassHandler serves the Home-Assistant admin config endpoints
// (specs/B5-home-assistant.md, sub-task B5a). GET returns only presence flags
// and the base URL — NEVER the token. POST sets the connection after
// validating the base URL. Both routes are wired outside publicRoutes and
// clientRoutes in main.go, so they require a session and (for POST) pass the
// CSRF/same-origin check exactly like POST /api/preview_live. The token is
// never logged or echoed back.

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"e-ink-picture/server/internal/hass"
	"e-ink-picture/server/internal/services"
)

// HassHandler wires the HA config manager and (read-only) fetch service.
type HassHandler struct {
	svc *services.HassService
	mgr *hass.Manager
}

// NewHassHandler creates a HassHandler.
func NewHassHandler(svc *services.HassService, mgr *hass.Manager) *HassHandler {
	return &HassHandler{svc: svc, mgr: mgr}
}

// hassConfigRequest is the POST /api/hass/config body.
type hassConfigRequest struct {
	BaseURL string `json:"base_url"`
	Token   string `json:"token"`
}

// hassConfigResponse is the GET/POST response shape. It deliberately has NO
// token field — the token value never leaves the server.
type hassConfigResponse struct {
	Configured bool   `json:"configured"`
	BaseURL    string `json:"base_url"`
	TokenSet   bool   `json:"token_set"`
}

// GetConfig handles GET /api/hass/config. It returns only {configured,
// base_url, token_set} — never the token value.
func (h *HassHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	baseURL, configured := h.mgr.Status()
	jsonResponse(w, http.StatusOK, hassConfigResponse{
		Configured: configured,
		BaseURL:    baseURL,
		TokenSet:   h.mgr.TokenSet(),
	})
}

// SetConfig handles POST /api/hass/config. It validates the base URL (scheme
// http/https, non-empty host; empty/file://gopher:// rejected with 400) and
// persists the connection atomically. The response never contains the token,
// and no token value is logged.
func (h *HassHandler) SetConfig(w http.ResponseWriter, r *http.Request) {
	var req hassConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.mgr.SetConfig(req.BaseURL, req.Token); err != nil {
		// Generic client message; the validation error names only the base URL
		// (never the token) but is not echoed to keep the surface minimal.
		slog.Warn("rejected hass config: invalid base URL")
		jsonError(w, "invalid Home Assistant configuration", http.StatusBadRequest)
		return
	}

	baseURL, configured := h.mgr.Status()
	jsonResponse(w, http.StatusOK, hassConfigResponse{
		Configured: configured,
		BaseURL:    baseURL,
		TokenSet:   h.mgr.TokenSet(),
	})
}
