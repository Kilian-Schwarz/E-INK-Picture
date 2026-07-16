package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"e-ink-picture/server/internal/hass"
	"e-ink-picture/server/internal/services"
)

// hassHandlerSentinelToken is a fake token; the GET response must never carry it.
const hassHandlerSentinelToken = "SENTINEL_TOKEN_DO_NOT_LOG"

func newHassTestHandler(t *testing.T) (*HassHandler, string) {
	t.Helper()
	dir := t.TempDir()
	mgr, err := hass.NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	svc := services.NewHassService(mgr)
	return NewHassHandler(svc, mgr), dir
}

func postHassConfig(t *testing.T, h *HassHandler, baseURL, token string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{"base_url": baseURL, "token": token})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/hass/config", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.SetConfig(rec, req)
	return rec
}

// TestHassGetConfigNeverReturnsToken is AC-SEC9 (response part): GET exposes
// presence flags and the base URL but never the token value.
func TestHassGetConfigNeverReturnsToken(t *testing.T) {
	h, _ := newHassTestHandler(t)
	if rec := postHassConfig(t, h, "http://10.0.0.5:8123", hassHandlerSentinelToken); rec.Code != http.StatusOK {
		t.Fatalf("POST config = %d, want 200", rec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/hass/config", nil)
	rec := httptest.NewRecorder()
	h.GetConfig(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET config = %d, want 200", rec.Code)
	}

	raw := rec.Body.String()
	if strings.Contains(raw, hassHandlerSentinelToken) {
		t.Fatalf("GET response leaks the token:\n%s", raw)
	}

	var resp struct {
		Configured bool   `json:"configured"`
		BaseURL    string `json:"base_url"`
		TokenSet   bool   `json:"token_set"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Configured || !resp.TokenSet || resp.BaseURL != "http://10.0.0.5:8123" {
		t.Errorf("resp = %+v, want configured/token_set true and the base URL", resp)
	}
	// Defensive: there is no token field of any name in the JSON.
	var loose map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &loose); err != nil {
		t.Fatalf("decode loose: %v", err)
	}
	if _, ok := loose["token"]; ok {
		t.Error("GET response contains a \"token\" field, want none")
	}
}

// TestHassSetConfigRejectsBadURL is AC-SEC7(a): non-http(s)/empty base URLs are
// rejected with 400 and nothing is persisted.
func TestHassSetConfigRejectsBadURL(t *testing.T) {
	for _, base := range []string{"", "file:///etc/passwd", "gopher://10.0.0.5/", "ftp://x/"} {
		h, dir := newHassTestHandler(t)
		rec := postHassConfig(t, h, base, hassHandlerSentinelToken)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("POST base_url=%q = %d, want 400", base, rec.Code)
		}
		if strings.Contains(rec.Body.String(), hassHandlerSentinelToken) {
			t.Errorf("POST base_url=%q error response leaks the token", base)
		}
		if _, err := os.Stat(filepath.Join(dir, "hass.json")); !os.IsNotExist(err) {
			t.Errorf("POST base_url=%q persisted a file, want nothing written", base)
		}
	}
}

// TestHassSetConfigMalformedBody: a non-JSON body is a 400.
func TestHassSetConfigMalformedBody(t *testing.T) {
	h, _ := newHassTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/hass/config", strings.NewReader("not json"))
	rec := httptest.NewRecorder()
	h.SetConfig(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("malformed body = %d, want 400", rec.Code)
	}
}

// TestHassSetConfigOK: a valid config returns 200 with presence flags and no
// token, and persists the file.
func TestHassSetConfigOK(t *testing.T) {
	h, dir := newHassTestHandler(t)
	rec := postHassConfig(t, h, "https://ha.example:8123", hassHandlerSentinelToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST config = %d, want 200", rec.Code)
	}
	if strings.Contains(rec.Body.String(), hassHandlerSentinelToken) {
		t.Fatalf("POST response leaks the token:\n%s", rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "hass.json")); err != nil {
		t.Errorf("hass.json not persisted: %v", err)
	}
}
