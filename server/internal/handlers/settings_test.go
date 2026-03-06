package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"e-ink-picture/server/internal/services"
)

func newTestSettingsHandler(t *testing.T) *SettingsHandler {
	t.Helper()
	svc := services.NewSettingsService(t.TempDir())
	return NewSettingsHandler(svc)
}

func TestGetSettings(t *testing.T) {
	h := newTestSettingsHandler(t)
	req := httptest.NewRequest("GET", "/settings", nil)
	w := httptest.NewRecorder()

	h.GetSettings(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["refresh_interval"] == nil {
		t.Error("expected refresh_interval in response")
	}
}

func TestTriggerRefresh(t *testing.T) {
	h := newTestSettingsHandler(t)
	req := httptest.NewRequest("POST", "/api/trigger_refresh", nil)
	w := httptest.NewRecorder()

	h.TriggerRefresh(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "refresh_triggered" {
		t.Errorf("expected status=refresh_triggered, got %q", resp["status"])
	}
	if resp["timestamp"] == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestRefreshStatus(t *testing.T) {
	h := newTestSettingsHandler(t)
	req := httptest.NewRequest("GET", "/api/refresh_status", nil)
	w := httptest.NewRecorder()

	h.RefreshStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if _, ok := resp["should_refresh"]; !ok {
		t.Error("expected should_refresh in response")
	}
	if _, ok := resp["refresh_interval"]; !ok {
		t.Error("expected refresh_interval in response")
	}
}

func TestClientHeartbeat(t *testing.T) {
	h := newTestSettingsHandler(t)
	body := `{"status":"refreshed","timestamp":"2026-03-06T14:30:00Z"}`
	req := httptest.NewRequest("POST", "/api/client_heartbeat", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ClientHeartbeat(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]bool
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp["ok"] {
		t.Error("expected ok=true")
	}
}

func TestClientHeartbeat_MissingTimestamp(t *testing.T) {
	h := newTestSettingsHandler(t)
	body := `{"status":"refreshed"}`
	req := httptest.NewRequest("POST", "/api/client_heartbeat", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ClientHeartbeat(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestUpdateSettings_RefreshInterval(t *testing.T) {
	h := newTestSettingsHandler(t)
	body := `{"refresh_interval": 900}`
	req := httptest.NewRequest("POST", "/update_settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.UpdateSettings(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["refresh_interval"].(float64) != 900 {
		t.Errorf("expected refresh_interval=900, got %v", resp["refresh_interval"])
	}
}
