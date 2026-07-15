package handlers

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"e-ink-picture/server/internal/auth"
	"e-ink-picture/server/internal/services"
)

// SetupHandler serves GET /api/setup/status — the public probe the setup
// wizard uses to decide whether it should appear
// (specs/E2.3-setup-wizard.md, Architektur-Richtung 1).
type SetupHandler struct {
	manager  *auth.Manager
	settings *services.SettingsService
	designs  *services.DesignService
	images   *services.ImageService
}

// NewSetupHandler creates a SetupHandler.
func NewSetupHandler(manager *auth.Manager, settings *services.SettingsService, designs *services.DesignService, images *services.ImageService) *SetupHandler {
	return &SetupHandler{
		manager:  manager,
		settings: settings,
		designs:  designs,
		images:   images,
	}
}

// setupStatusResponse is the exact (deliberately minimal) public schema:
// password_set is already public via GET /api/auth/status, wizard and
// setup_completed carry no secrets, and server_time/server_timezone exist
// only for the wizard's sanity display (timezone is a non-goal).
type setupStatusResponse struct {
	Wizard         bool   `json:"wizard"`
	PasswordSet    bool   `json:"password_set"`
	SetupCompleted bool   `json:"setup_completed"`
	ServerTime     string `json:"server_time"`
	ServerTimezone string `json:"server_timezone"`
}

// Status handles GET /api/setup/status (public). wizard is true only when
// the installation is factory-fresh: no password, no setup_completed latch,
// no user-set settings keys (heartbeat/trigger traces do NOT count), at most
// one untouched default design, no history snapshots and an empty media
// library. Any usage trace means wizard=false — in doubt, no wizard.
func (h *SetupHandler) Status(w http.ResponseWriter, r *http.Request) {
	settings, err := h.settings.GetSettings()
	if err != nil {
		slog.Error("setup status: failed to load settings", "error", err)
		jsonError(w, "failed to determine setup status", http.StatusInternalServerError)
		return
	}
	settingsTouched, err := h.settings.HasUserSettings()
	if err != nil {
		slog.Error("setup status: failed to inspect settings.json", "error", err)
		jsonError(w, "failed to determine setup status", http.StatusInternalServerError)
		return
	}
	designsPristine, err := h.designs.IsPristine()
	if err != nil {
		slog.Error("setup status: failed to inspect designs", "error", err)
		jsonError(w, "failed to determine setup status", http.StatusInternalServerError)
		return
	}
	images, err := h.images.ListImages()
	if err != nil {
		slog.Error("setup status: failed to list images", "error", err)
		jsonError(w, "failed to determine setup status", http.StatusInternalServerError)
		return
	}
	fonts, err := h.images.ListFonts()
	if err != nil {
		slog.Error("setup status: failed to list fonts", "error", err)
		jsonError(w, "failed to determine setup status", http.StatusInternalServerError)
		return
	}

	passwordSet := h.manager.PasswordSet()
	wizard := !passwordSet &&
		!settings.SetupCompleted &&
		!settingsTouched &&
		designsPristine &&
		len(images) == 0 &&
		len(fonts) == 0

	jsonResponse(w, http.StatusOK, setupStatusResponse{
		Wizard:         wizard,
		PasswordSet:    passwordSet,
		SetupCompleted: settings.SetupCompleted,
		ServerTime:     time.Now().Format(time.RFC3339),
		ServerTimezone: os.Getenv("TZ"),
	})
}
