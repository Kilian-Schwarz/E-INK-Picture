package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"e-ink-picture/server/internal/models"
	"e-ink-picture/server/internal/services"
)

// SettingsHandler handles display settings endpoints.
type SettingsHandler struct {
	settings *services.SettingsService
}

// NewSettingsHandler creates a new SettingsHandler.
func NewSettingsHandler(s *services.SettingsService) *SettingsHandler {
	return &SettingsHandler{settings: s}
}

// GetSettings returns the current settings with resolved display config.
func (h *SettingsHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	resp, err := h.settings.GetSettingsResponse()
	if err != nil {
		slog.Error("failed to load settings", "error", err)
		jsonError(w, "failed to load settings", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, resp)
}

// UpdateSettings saves new settings and returns the updated state.
func (h *SettingsHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DisplayType     models.DisplayType `json:"display_type"`
		RefreshInterval *int               `json:"refresh_interval,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	current, err := h.settings.GetSettings()
	if err != nil {
		slog.Error("failed to load settings", "error", err)
		jsonError(w, "failed to load settings", http.StatusInternalServerError)
		return
	}

	if req.DisplayType != "" {
		if _, ok := models.DisplayProfiles[req.DisplayType]; !ok {
			jsonError(w, "unknown display type: "+string(req.DisplayType), http.StatusBadRequest)
			return
		}
		current.DisplayType = req.DisplayType
	}

	if req.RefreshInterval != nil && *req.RefreshInterval > 0 {
		current.RefreshInterval = *req.RefreshInterval
	}

	if err := h.settings.SaveSettings(current); err != nil {
		slog.Error("failed to save settings", "error", err)
		jsonError(w, "failed to save settings", http.StatusInternalServerError)
		return
	}

	resp, err := h.settings.GetSettingsResponse()
	if err != nil {
		slog.Error("failed to load settings after save", "error", err)
		jsonError(w, "failed to load settings", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, resp)
}

// TriggerRefresh sets a refresh trigger for the client to pick up.
func (h *SettingsHandler) TriggerRefresh(w http.ResponseWriter, r *http.Request) {
	ts, err := h.settings.TriggerRefresh()
	if err != nil {
		slog.Error("failed to trigger refresh", "error", err)
		jsonError(w, "failed to trigger refresh", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{
		"status":    "refresh_triggered",
		"timestamp": ts,
	})
}

// RefreshStatus returns whether the client should refresh.
func (h *SettingsHandler) RefreshStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.settings.GetRefreshStatus()
	if err != nil {
		slog.Error("failed to get refresh status", "error", err)
		jsonError(w, "failed to get refresh status", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, status)
}

// ClientHeartbeat records a client refresh and returns ok.
func (h *SettingsHandler) ClientHeartbeat(w http.ResponseWriter, r *http.Request) {
	// Accept body but don't require timestamp - server uses its own UTC time
	var req struct {
		Status string `json:"status"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if err := h.settings.RecordClientRefresh(""); err != nil {
		slog.Error("failed to record client heartbeat", "error", err)
		jsonError(w, "failed to record heartbeat", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

// ListDisplayProfiles returns all available display profiles.
func (h *SettingsHandler) ListDisplayProfiles(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, models.DisplayProfileList())
}
