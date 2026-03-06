package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"e-ink-picture/server/internal/models"
	"e-ink-picture/server/internal/services"
)

func setupPreviewTestServices(t *testing.T) (*PreviewHandler, string) {
	t.Helper()
	tmpDir := t.TempDir()

	for _, sub := range []string{"designs", "uploaded_images", "fonts", "weather_styles"} {
		if err := os.MkdirAll(filepath.Join(tmpDir, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}

	settingsData := `{"display_type":"waveshare_7in5_V2","refresh_interval":3600}`
	os.WriteFile(filepath.Join(tmpDir, "settings.json"), []byte(settingsData), 0644)

	designSvc := services.NewDesignService(tmpDir)
	imageSvc := services.NewImageService(tmpDir)
	weatherSvc := services.NewWeatherService("", "", tmpDir)
	settingsSvc := services.NewSettingsService(tmpDir)
	previewSvc := services.NewPreviewService(designSvc, weatherSvc, imageSvc, settingsSvc, tmpDir)

	handler := NewPreviewHandler(previewSvc, designSvc)
	return handler, tmpDir
}

func TestPreviewLiveEndpoint(t *testing.T) {
	handler, _ := setupPreviewTestServices(t)

	vis := true
	design := models.DesignV2{
		Name:    "live-test",
		Version: 2,
		Canvas:  models.CanvasConfig{Width: 800, Height: 480, Background: "#FFFFFF"},
		Elements: []models.Element{
			{
				ID:      "t1",
				Type:    "text",
				X:       100,
				Y:       50,
				Width:   200,
				Height:  60,
				ZIndex:  0,
				Visible: &vis,
				Properties: map[string]any{
					"text":      "Live Preview Test",
					"fontSize":  float64(24),
					"color":     "#000000",
					"textAlign": "left",
				},
			},
		},
	}

	body, err := json.Marshal(design)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("POST", "/api/preview_live", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.PreviewLive(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Expected Content-Type image/png, got %s", ct)
	}
	if rec.Body.Len() < 100 {
		t.Error("Response body too small for a valid PNG")
	}
}

func TestPreviewLiveInvalidBody(t *testing.T) {
	handler, _ := setupPreviewTestServices(t)

	req := httptest.NewRequest("POST", "/api/preview_live", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.PreviewLive(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid JSON, got %d", rec.Code)
	}
}
