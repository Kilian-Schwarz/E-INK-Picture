package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"e-ink-picture/server/internal/models"
	"e-ink-picture/server/internal/services"
)

func newTestSettingsHandler(t *testing.T) *SettingsHandler {
	t.Helper()
	svc := services.NewSettingsService(t.TempDir(), models.DisplayWaveshare75V2)
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

func TestClientHeartbeat_NoTimestampRequired(t *testing.T) {
	h := newTestSettingsHandler(t)
	body := `{"status":"refreshed"}`
	req := httptest.NewRequest("POST", "/api/client_heartbeat", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ClientHeartbeat(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
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

// getSettingsMap fetches GET /settings and decodes the JSON body.
func getSettingsMap(t *testing.T, h *SettingsHandler) map[string]any {
	t.Helper()
	req := httptest.NewRequest("GET", "/settings", nil)
	w := httptest.NewRecorder()
	h.GetSettings(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /settings: expected 200, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode settings response: %v", err)
	}
	return resp
}

// postSettings sends a POST /update_settings with the given JSON body.
func postSettings(t *testing.T, h *SettingsHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/update_settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.UpdateSettings(w, req)
	return w
}

// E1.6 AC1: GET /settings resolves the new fields to their defaults when no
// settings.json exists.
func TestGetSettings_DitherCalibrationDefaults(t *testing.T) {
	h := newTestSettingsHandler(t)
	resp := getSettingsMap(t, h)

	if resp["dither_algorithm"] != "floyd_steinberg" {
		t.Errorf("expected dither_algorithm=floyd_steinberg, got %v", resp["dither_algorithm"])
	}
	if resp["calibration"] != "default" {
		t.Errorf("expected calibration=default, got %v", resp["calibration"])
	}
}

// E1.6 AC1: valid values persist; fields that are not sent stay unchanged.
func TestUpdateSettings_DitherAlgorithmAndCalibration(t *testing.T) {
	h := newTestSettingsHandler(t)

	w := postSettings(t, h, `{"dither_algorithm":"atkinson"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["dither_algorithm"] != "atkinson" {
		t.Errorf("expected dither_algorithm=atkinson, got %v", resp["dither_algorithm"])
	}
	if resp["calibration"] != "default" {
		t.Errorf("calibration was not sent and must stay default, got %v", resp["calibration"])
	}

	w = postSettings(t, h, `{"calibration":"off"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp = getSettingsMap(t, h)
	if resp["calibration"] != "off" {
		t.Errorf("expected calibration=off, got %v", resp["calibration"])
	}
	if resp["dither_algorithm"] != "atkinson" {
		t.Errorf("dither_algorithm was not sent and must stay atkinson, got %v", resp["dither_algorithm"])
	}
}

// E1.6 AC1: unknown values answer 400 and must not have persistence side
// effects — the previously stored values stay intact.
func TestUpdateSettings_InvalidDitherAlgorithmAndCalibration(t *testing.T) {
	h := newTestSettingsHandler(t)

	if w := postSettings(t, h, `{"dither_algorithm":"atkinson","calibration":"off"}`); w.Code != http.StatusOK {
		t.Fatalf("seed update: expected 200, got %d", w.Code)
	}

	if w := postSettings(t, h, `{"dither_algorithm":"bayer"}`); w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for dither_algorithm=bayer, got %d", w.Code)
	}
	if w := postSettings(t, h, `{"calibration":"vivid"}`); w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for calibration=vivid, got %d", w.Code)
	}
	// Invalid value alongside a valid field: the whole request must fail
	// without persisting anything.
	if w := postSettings(t, h, `{"refresh_interval":1234,"dither_algorithm":"bayer"}`); w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for mixed valid/invalid update, got %d", w.Code)
	}

	resp := getSettingsMap(t, h)
	if resp["dither_algorithm"] != "atkinson" {
		t.Errorf("dither_algorithm changed by rejected update: %v", resp["dither_algorithm"])
	}
	if resp["calibration"] != "off" {
		t.Errorf("calibration changed by rejected update: %v", resp["calibration"])
	}
	if resp["refresh_interval"].(float64) == 1234 {
		t.Error("refresh_interval was persisted by a rejected update")
	}
}
