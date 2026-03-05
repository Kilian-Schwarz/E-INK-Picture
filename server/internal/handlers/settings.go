package handlers

import (
	"encoding/json"
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
		http.Error(w, "failed to load settings", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// UpdateSettings saves new settings and returns the updated state.
func (h *SettingsHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DisplayType models.DisplayType `json:"display_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate display type
	if _, ok := models.DisplayProfiles[req.DisplayType]; !ok {
		http.Error(w, "unknown display type", http.StatusBadRequest)
		return
	}

	settings := &models.Settings{DisplayType: req.DisplayType}
	if err := h.settings.SaveSettings(settings); err != nil {
		http.Error(w, "failed to save settings", http.StatusInternalServerError)
		return
	}

	resp, err := h.settings.GetSettingsResponse()
	if err != nil {
		http.Error(w, "failed to load settings", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ListDisplayProfiles returns all available display profiles.
func (h *SettingsHandler) ListDisplayProfiles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.DisplayProfileList())
}
