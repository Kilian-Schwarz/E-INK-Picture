package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"e-ink-picture/server/internal/auth"
	"e-ink-picture/server/internal/models"
	"e-ink-picture/server/internal/services"
)

// setupFixture wires a SetupHandler over a real temp DATA_DIR that mirrors
// the startup state of newApplication (directory layout + EnsureDesignExists).
type setupFixture struct {
	dir      string
	handler  *SetupHandler
	authMgr  *auth.Manager
	settings *services.SettingsService
	designs  *services.DesignService
}

func newSetupFixture(t *testing.T) *setupFixture {
	t.Helper()
	dir := t.TempDir()
	for _, sub := range []string{
		filepath.Join("designs", "history"),
		filepath.Join("uploaded_images", "thumbs"),
		"fonts",
	} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}

	authMgr, err := auth.NewManager(dir)
	if err != nil {
		t.Fatalf("auth.NewManager: %v", err)
	}
	settingsSvc := services.NewSettingsService(dir, models.DisplayWaveshare73E)
	designSvc := services.NewDesignService(dir)
	imageSvc := services.NewImageService(dir)

	// Startup behavior: the untouched "Default Design" always exists.
	if err := designSvc.EnsureDesignExists(); err != nil {
		t.Fatalf("EnsureDesignExists: %v", err)
	}

	return &setupFixture{
		dir:      dir,
		handler:  NewSetupHandler(authMgr, settingsSvc, designSvc, imageSvc),
		authMgr:  authMgr,
		settings: settingsSvc,
		designs:  designSvc,
	}
}

// getSetupStatus performs GET /api/setup/status and decodes the response.
func getSetupStatus(t *testing.T, h *SetupHandler) map[string]any {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/setup/status", nil)
	w := httptest.NewRecorder()
	h.Status(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/setup/status: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode setup status: %v", err)
	}
	return resp
}

// writeRawSettings writes a raw settings.json (bypassing the service) so
// tests control exactly which keys exist on disk.
func writeRawSettings(t *testing.T, dir, raw string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(raw), 0644); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}
}

// E2.3 AC1: the appearance criterion. Fresh installs (including
// heartbeat-only traces) show the wizard; every real usage trace blocks it.
func TestSetupStatus_FreshnessTable(t *testing.T) {
	cases := []struct {
		name    string
		prepare func(t *testing.T, f *setupFixture)
		want    bool
	}{
		{
			name:    "fresh install (AC1a)",
			prepare: func(t *testing.T, f *setupFixture) {},
			want:    true,
		},
		{
			name: "password set (AC1b)",
			prepare: func(t *testing.T, f *setupFixture) {
				if err := f.authMgr.SetPassword("secret"); err != nil {
					t.Fatalf("SetPassword: %v", err)
				}
			},
			want: false,
		},
		{
			name: "settings trace: render_quality key (AC1c)",
			prepare: func(t *testing.T, f *setupFixture) {
				writeRawSettings(t, f.dir, `{"display_type":"waveshare_7in3_e","refresh_interval":3600,"render_quality":"high"}`)
			},
			want: false,
		},
		{
			name: "settings trace: dither_algorithm key",
			prepare: func(t *testing.T, f *setupFixture) {
				writeRawSettings(t, f.dir, `{"display_type":"waveshare_7in3_e","refresh_interval":3600,"dither_algorithm":"floyd_steinberg"}`)
			},
			want: false,
		},
		{
			name: "settings trace: calibration key",
			prepare: func(t *testing.T, f *setupFixture) {
				writeRawSettings(t, f.dir, `{"display_type":"waveshare_7in3_e","refresh_interval":3600,"calibration":"default"}`)
			},
			want: false,
		},
		{
			name: "settings trace: sleep window keys",
			prepare: func(t *testing.T, f *setupFixture) {
				writeRawSettings(t, f.dir, `{"display_type":"waveshare_7in3_e","refresh_interval":3600,"sleep_start":"23:00","sleep_end":"06:00"}`)
			},
			want: false,
		},
		{
			// The critical case: RecordClientRefresh materializes
			// settings.json with display_type/refresh_interval/
			// last_client_refresh — that alone must NOT block the wizard.
			name: "heartbeat-only settings.json (AC1d)",
			prepare: func(t *testing.T, f *setupFixture) {
				if err := f.settings.RecordClientRefresh(""); err != nil {
					t.Fatalf("RecordClientRefresh: %v", err)
				}
			},
			want: true,
		},
		{
			name: "trigger-only settings.json",
			prepare: func(t *testing.T, f *setupFixture) {
				if _, err := f.settings.TriggerRefresh(); err != nil {
					t.Fatalf("TriggerRefresh: %v", err)
				}
			},
			want: true,
		},
		{
			name: "setup_completed latch set",
			prepare: func(t *testing.T, f *setupFixture) {
				writeRawSettings(t, f.dir, `{"display_type":"waveshare_7in3_e","refresh_interval":3600,"setup_completed":true}`)
			},
			want: false,
		},
		{
			name: "second design exists (AC1c)",
			prepare: func(t *testing.T, f *setupFixture) {
				if _, err := f.designs.CreateDesign("Second", nil, models.CanvasConfig{Width: 800, Height: 480, Background: "#FFFFFF"}); err != nil {
					t.Fatalf("CreateDesign: %v", err)
				}
			},
			want: false,
		},
		{
			name: "default design has an element (AC1c)",
			prepare: func(t *testing.T, f *setupFixture) {
				design := `{"name":"Default Design","version":2,"canvas":{"width":800,"height":480,"background":"#FFFFFF"},"elements":[{"id":"e1","type":"text","x":0,"y":0,"width":100,"height":40,"properties":{"text":"hi"}}],"active":true}`
				if err := os.WriteFile(filepath.Join(f.dir, "designs", "design_default.json"), []byte(design), 0644); err != nil {
					t.Fatalf("write design: %v", err)
				}
			},
			want: false,
		},
		{
			name: "history snapshot exists (AC1c)",
			prepare: func(t *testing.T, f *setupFixture) {
				histDir := filepath.Join(f.dir, "designs", "history", "design_default")
				if err := os.MkdirAll(histDir, 0755); err != nil {
					t.Fatalf("mkdir history: %v", err)
				}
				if err := os.WriteFile(filepath.Join(histDir, "2026-01-01T00-00-00.json"), []byte(`{}`), 0644); err != nil {
					t.Fatalf("write snapshot: %v", err)
				}
			},
			want: false,
		},
		{
			name: "one uploaded image (AC1c)",
			prepare: func(t *testing.T, f *setupFixture) {
				if err := os.WriteFile(filepath.Join(f.dir, "uploaded_images", "photo.png"), []byte("x"), 0644); err != nil {
					t.Fatalf("write image: %v", err)
				}
			},
			want: false,
		},
		{
			name: "one uploaded font",
			prepare: func(t *testing.T, f *setupFixture) {
				if err := os.WriteFile(filepath.Join(f.dir, "fonts", "custom.ttf"), []byte("x"), 0644); err != nil {
					t.Fatalf("write font: %v", err)
				}
			},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSetupFixture(t)
			tc.prepare(t, f)
			resp := getSetupStatus(t, f.handler)
			if resp["wizard"] != tc.want {
				t.Errorf("wizard = %v, want %v (response: %v)", resp["wizard"], tc.want, resp)
			}
		})
	}
}

// E2.3 Richtung 1: the response carries exactly the five spec'd fields —
// the public probe stays minimal and leaks nothing else.
func TestSetupStatus_ResponseSchema(t *testing.T) {
	t.Setenv("TZ", "Europe/Berlin")
	f := newSetupFixture(t)
	resp := getSetupStatus(t, f.handler)

	for _, key := range []string{"wizard", "password_set", "setup_completed", "server_time", "server_timezone"} {
		if _, ok := resp[key]; !ok {
			t.Errorf("response is missing field %q", key)
		}
	}
	if len(resp) != 5 {
		t.Errorf("response must contain exactly 5 fields, got %d: %v", len(resp), resp)
	}
	if resp["password_set"] != false || resp["setup_completed"] != false {
		t.Errorf("fresh install: password_set/setup_completed must be false, got %v/%v",
			resp["password_set"], resp["setup_completed"])
	}
	st, _ := resp["server_time"].(string)
	if _, err := time.Parse(time.RFC3339, st); err != nil {
		t.Errorf("server_time %q is not RFC3339: %v", st, err)
	}
	if resp["server_timezone"] != "Europe/Berlin" {
		t.Errorf("server_timezone = %v, want Europe/Berlin (TZ env)", resp["server_timezone"])
	}
}

// E2.3 AC2/Richtung 2: setup_completed roundtrips through POST
// /update_settings, is a one-way latch, and survives the settings.json
// rewrites of TriggerRefresh and RecordClientRefresh.
func TestUpdateSettings_SetupCompletedFlag(t *testing.T) {
	f := newSetupFixture(t)
	settingsH := NewSettingsHandler(f.settings)

	// Roundtrip: POST {"setup_completed": true} persists the flag.
	if w := postSettings(t, settingsH, `{"setup_completed":true}`); w.Code != http.StatusOK {
		t.Fatalf("set setup_completed: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	s, err := f.settings.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if !s.SetupCompleted {
		t.Fatal("setup_completed was not persisted")
	}

	// One-way latch: sending false must not reset the flag.
	if w := postSettings(t, settingsH, `{"setup_completed":false}`); w.Code != http.StatusOK {
		t.Fatalf("send false: expected 200, got %d", w.Code)
	}
	if s, _ = f.settings.GetSettings(); !s.SetupCompleted {
		t.Error("setup_completed was reset to false; it must be a one-way latch")
	}

	// The flag survives the full-struct roundtrips of trigger + heartbeat.
	if _, err := f.settings.TriggerRefresh(); err != nil {
		t.Fatalf("TriggerRefresh: %v", err)
	}
	if err := f.settings.RecordClientRefresh(""); err != nil {
		t.Fatalf("RecordClientRefresh: %v", err)
	}
	if s, _ = f.settings.GetSettings(); !s.SetupCompleted {
		t.Error("setup_completed was lost by TriggerRefresh/RecordClientRefresh roundtrip")
	}
	raw, err := os.ReadFile(filepath.Join(f.dir, "settings.json"))
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	var onDisk map[string]any
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("parse settings.json: %v", err)
	}
	if onDisk["setup_completed"] != true {
		t.Errorf("settings.json setup_completed = %v, want true", onDisk["setup_completed"])
	}

	// And the status endpoint reflects it: wizard stays off for good.
	resp := getSetupStatus(t, f.handler)
	if resp["setup_completed"] != true {
		t.Errorf("status setup_completed = %v, want true", resp["setup_completed"])
	}
	if resp["wizard"] != false {
		t.Errorf("wizard = %v, want false after completion", resp["wizard"])
	}
}

// An unrelated settings update must not touch the flag (pointer semantics).
func TestUpdateSettings_SetupCompletedUntouchedByOtherUpdates(t *testing.T) {
	f := newSetupFixture(t)
	settingsH := NewSettingsHandler(f.settings)

	if w := postSettings(t, settingsH, `{"setup_completed":true}`); w.Code != http.StatusOK {
		t.Fatalf("set setup_completed: expected 200, got %d", w.Code)
	}
	if w := postSettings(t, settingsH, `{"refresh_interval":900}`); w.Code != http.StatusOK {
		t.Fatalf("unrelated update: expected 200, got %d", w.Code)
	}
	if s, _ := f.settings.GetSettings(); !s.SetupCompleted {
		t.Error("setup_completed was lost by an unrelated update")
	}
}
