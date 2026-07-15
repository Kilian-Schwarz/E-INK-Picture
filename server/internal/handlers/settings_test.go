package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// --- E5.2 Phase 1: sleep window (panel care) ---

// newTestSettingsHandlerWithDir returns a handler plus its data dir so tests
// can seed settings.json directly.
func newTestSettingsHandlerWithDir(t *testing.T) (*SettingsHandler, string) {
	t.Helper()
	dir := t.TempDir()
	svc := services.NewSettingsService(dir, models.DisplayWaveshare75V2)
	return NewSettingsHandler(svc), dir
}

// writeHandlerSettings seeds a settings.json into the handler's data dir.
func writeHandlerSettings(t *testing.T, dir string, s models.Settings) {
	t.Helper()
	if s.DisplayType == "" {
		s.DisplayType = models.DisplayWaveshare75V2
	}
	data, err := json.MarshalIndent(&s, "", "  ")
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), data, 0644); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}
}

// getRefreshStatusRaw fetches GET /api/refresh_status and returns the decoded
// map plus the raw body (for field-absence assertions).
func getRefreshStatusRaw(t *testing.T, h *SettingsHandler) (map[string]any, string) {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/refresh_status", nil)
	w := httptest.NewRecorder()
	h.RefreshStatus(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/refresh_status: expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	var resp map[string]any
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode refresh_status: %v", err)
	}
	return resp, body
}

// AC1: sleep window roundtrip — set, read back, clear.
func TestUpdateSettings_SleepWindowRoundtrip(t *testing.T) {
	h := newTestSettingsHandler(t)

	w := postSettings(t, h, `{"sleep_start":"23:00","sleep_end":"06:00"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	resp := getSettingsMap(t, h)
	if resp["sleep_start"] != "23:00" {
		t.Errorf("expected sleep_start=23:00, got %v", resp["sleep_start"])
	}
	if resp["sleep_end"] != "06:00" {
		t.Errorf("expected sleep_end=06:00, got %v", resp["sleep_end"])
	}

	// Clearing both fields disables the window; the keys stay present.
	w = postSettings(t, h, `{"sleep_start":"","sleep_end":""}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when clearing, got %d: %s", w.Code, w.Body.String())
	}
	resp = getSettingsMap(t, h)
	if resp["sleep_start"] != "" {
		t.Errorf("expected sleep_start cleared to \"\", got %v", resp["sleep_start"])
	}
	if resp["sleep_end"] != "" {
		t.Errorf("expected sleep_end cleared to \"\", got %v", resp["sleep_end"])
	}
}

// AC1: invalid values and half-set windows answer 400 without persisting.
func TestUpdateSettings_SleepWindowValidation(t *testing.T) {
	h := newTestSettingsHandler(t)

	badBodies := []string{
		`{"sleep_start":"23:0","sleep_end":"06:00"}`,  // minutes not zero-padded
		`{"sleep_start":"24:00","sleep_end":"06:00"}`, // hour out of range
		`{"sleep_start":"aa:bb","sleep_end":"06:00"}`, // garbage
		`{"sleep_start":"23:00"}`,                     // only one field resulting set
		`{"sleep_end":"06:00"}`,                       // only one field resulting set
		`{"sleep_start":"12:00","sleep_end":"12:00"}`, // start == end
	}
	for _, body := range badBodies {
		if w := postSettings(t, h, body); w.Code != http.StatusBadRequest {
			t.Errorf("body %s: expected 400, got %d", body, w.Code)
		}
	}

	resp := getSettingsMap(t, h)
	if resp["sleep_start"] != "" || resp["sleep_end"] != "" {
		t.Errorf("rejected updates must not persist, got %v/%v", resp["sleep_start"], resp["sleep_end"])
	}

	// Clearing only one field of an existing window is rejected too.
	if w := postSettings(t, h, `{"sleep_start":"23:00","sleep_end":"06:00"}`); w.Code != http.StatusOK {
		t.Fatalf("seed window: expected 200, got %d", w.Code)
	}
	if w := postSettings(t, h, `{"sleep_end":""}`); w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when clearing only sleep_end, got %d", w.Code)
	}
	resp = getSettingsMap(t, h)
	if resp["sleep_start"] != "23:00" || resp["sleep_end"] != "06:00" {
		t.Errorf("window changed by rejected update: %v/%v", resp["sleep_start"], resp["sleep_end"])
	}
}

// AC1: pointer semantics — fields not sent stay unchanged; sending one field
// against an existing window is a valid partial update.
func TestUpdateSettings_SleepWindowPointerSemantics(t *testing.T) {
	h := newTestSettingsHandler(t)

	if w := postSettings(t, h, `{"sleep_start":"23:00","sleep_end":"06:00"}`); w.Code != http.StatusOK {
		t.Fatalf("seed window: expected 200, got %d", w.Code)
	}

	// An unrelated update leaves the window untouched.
	if w := postSettings(t, h, `{"refresh_interval":900}`); w.Code != http.StatusOK {
		t.Fatalf("refresh_interval update: expected 200, got %d", w.Code)
	}
	resp := getSettingsMap(t, h)
	if resp["sleep_start"] != "23:00" || resp["sleep_end"] != "06:00" {
		t.Errorf("window changed by unrelated update: %v/%v", resp["sleep_start"], resp["sleep_end"])
	}

	// A partial update of one bound keeps the other.
	if w := postSettings(t, h, `{"sleep_start":"22:00"}`); w.Code != http.StatusOK {
		t.Fatalf("partial update: expected 200, got %d", w.Code)
	}
	resp = getSettingsMap(t, h)
	if resp["sleep_start"] != "22:00" {
		t.Errorf("expected sleep_start=22:00, got %v", resp["sleep_start"])
	}
	if resp["sleep_end"] != "06:00" {
		t.Errorf("sleep_end must stay 06:00, got %v", resp["sleep_end"])
	}
}

// AC4: reason is "interval" when the interval elapsed, "manual" after a
// trigger, and absent from the raw JSON when should_refresh is false.
func TestRefreshStatus_ReasonField(t *testing.T) {
	t.Run("interval", func(t *testing.T) {
		h, dir := newTestSettingsHandlerWithDir(t)
		writeHandlerSettings(t, dir, models.Settings{
			RefreshInterval:   1,
			LastClientRefresh: time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
		})

		resp, _ := getRefreshStatusRaw(t, h)
		if resp["should_refresh"] != true {
			t.Fatalf("expected should_refresh=true, got %v", resp["should_refresh"])
		}
		if resp["reason"] != "interval" {
			t.Errorf("expected reason=interval, got %v", resp["reason"])
		}
	})

	t.Run("manual", func(t *testing.T) {
		h, dir := newTestSettingsHandlerWithDir(t)
		writeHandlerSettings(t, dir, models.Settings{
			RefreshInterval:   3600,
			LastClientRefresh: time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
		})

		req := httptest.NewRequest("POST", "/api/trigger_refresh", nil)
		w := httptest.NewRecorder()
		h.TriggerRefresh(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("trigger: expected 200, got %d", w.Code)
		}

		resp, _ := getRefreshStatusRaw(t, h)
		if resp["should_refresh"] != true {
			t.Fatalf("expected should_refresh=true, got %v", resp["should_refresh"])
		}
		if resp["reason"] != "manual" {
			t.Errorf("expected reason=manual, got %v", resp["reason"])
		}
	})

	t.Run("absent when no refresh", func(t *testing.T) {
		h, dir := newTestSettingsHandlerWithDir(t)
		writeHandlerSettings(t, dir, models.Settings{
			RefreshInterval:   3600,
			LastClientRefresh: time.Now().UTC().Format(time.RFC3339),
		})

		resp, body := getRefreshStatusRaw(t, h)
		if resp["should_refresh"] != false {
			t.Fatalf("expected should_refresh=false, got %v", resp["should_refresh"])
		}
		if strings.Contains(body, `"reason"`) {
			t.Errorf("reason must be omitted when should_refresh=false, body: %s", body)
		}
	})
}

// AC3 (httptest): an active sleep window suppresses the elapsed interval;
// POST /api/trigger_refresh breaks through with reason=manual.
func TestRefreshStatus_ManualTriggerBreaksSleepWindow(t *testing.T) {
	h, dir := newTestSettingsHandlerWithDir(t)
	now := time.Now()
	writeHandlerSettings(t, dir, models.Settings{
		RefreshInterval:   1,
		LastClientRefresh: now.Add(-time.Hour).UTC().Format(time.RFC3339),
		SleepStart:        now.Add(-time.Hour).Format("15:04"), // window around "now"
		SleepEnd:          now.Add(time.Hour).Format("15:04"),
	})

	// Interval elapsed, but inside the window: suppressed, no reason field.
	resp, body := getRefreshStatusRaw(t, h)
	if resp["should_refresh"] != false {
		t.Fatalf("expected should_refresh=false inside sleep window, body: %s", body)
	}
	if strings.Contains(body, `"reason"`) {
		t.Errorf("reason must be omitted when suppressed, body: %s", body)
	}

	// Manual trigger breaks through the window.
	req := httptest.NewRequest("POST", "/api/trigger_refresh", nil)
	w := httptest.NewRecorder()
	h.TriggerRefresh(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("trigger: expected 200, got %d", w.Code)
	}

	resp, _ = getRefreshStatusRaw(t, h)
	if resp["should_refresh"] != true {
		t.Error("expected should_refresh=true after manual trigger inside sleep window")
	}
	if resp["reason"] != "manual" {
		t.Errorf("expected reason=manual, got %v", resp["reason"])
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
