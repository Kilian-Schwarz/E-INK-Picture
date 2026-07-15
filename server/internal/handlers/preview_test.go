package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
	settingsSvc := services.NewSettingsService(tmpDir, models.DisplayWaveshare75V2)
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

// setupPreviewSemaphoreTest builds the real handler stack with render_quality
// pinned to high (longest render path) and a deterministic, font-free design
// saved as "sem-test".
func setupPreviewSemaphoreTest(t *testing.T) (*PreviewHandler, *services.PreviewService) {
	t.Helper()
	tmpDir := t.TempDir()

	for _, sub := range []string{"designs", "uploaded_images", "fonts", "weather_styles"} {
		if err := os.MkdirAll(filepath.Join(tmpDir, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}

	settingsData := `{"display_type":"waveshare_7in5_v2","refresh_interval":3600,"render_quality":"high","dither_algorithm":"floyd_steinberg","calibration":"default"}`
	if err := os.WriteFile(filepath.Join(tmpDir, "settings.json"), []byte(settingsData), 0644); err != nil {
		t.Fatal(err)
	}

	designSvc := services.NewDesignService(tmpDir)
	imageSvc := services.NewImageService(tmpDir)
	weatherSvc := services.NewWeatherService("", "", tmpDir)
	settingsSvc := services.NewSettingsService(tmpDir, models.DisplayWaveshare75V2)
	previewSvc := services.NewPreviewService(designSvc, weatherSvc, imageSvc, settingsSvc, tmpDir)

	vis := true
	design := &models.DesignV2{
		Name:    "sem-test",
		Version: 2,
		Canvas:  models.CanvasConfig{Width: 800, Height: 480, Background: "#FFFFFF"},
		Elements: []models.Element{
			{
				ID: "s1", Type: "shape",
				X: 100, Y: 80, Width: 300, Height: 200,
				ZIndex: 0, Visible: &vis,
				Properties: map[string]any{"fill": "#000000"},
			},
		},
	}
	if err := designSvc.Save("sem-test", design); err != nil {
		t.Fatal(err)
	}

	return NewPreviewHandler(previewSvc, designSvc), previewSvc
}

// TestPreviewConcurrentRequestsSerialized proves AC1 of
// specs/E5.6-render-memory.md end-to-end: with the default semaphore
// capacity of 1, four parallel GET /preview requests against the wired
// handler stack all answer 200 with a valid PNG while the observed maximum
// of the active-render counter is exactly 1. Runs clean under -race.
func TestPreviewConcurrentRequestsSerialized(t *testing.T) {
	handler, previewSvc := setupPreviewSemaphoreTest(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /preview", handler.Preview)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Sampler goroutine: records the maximum observed active-render count.
	var maxActive atomic.Int32
	stop := make(chan struct{})
	samplerDone := make(chan struct{})
	go func() {
		defer close(samplerDone)
		for {
			select {
			case <-stop:
				return
			default:
			}
			if a := previewSvc.ActiveRenders(); a > maxActive.Load() {
				maxActive.Store(a)
			}
			time.Sleep(50 * time.Microsecond)
		}
	}()

	const parallel = 4
	var wg sync.WaitGroup
	errs := make(chan error, parallel)
	for i := 0; i < parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(srv.URL + "/preview?name=sem-test")
			if err != nil {
				errs <- err
				return
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				errs <- err
				return
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("status = %d, want 200 (body: %s)", resp.StatusCode, body)
				return
			}
			if _, err := png.Decode(bytes.NewReader(body)); err != nil {
				t.Errorf("response is not a valid PNG: %v", err)
			}
		}()
	}
	wg.Wait()
	close(stop)
	<-samplerDone
	close(errs)
	for err := range errs {
		t.Errorf("request failed: %v", err)
	}

	if got := maxActive.Load(); got != 1 {
		t.Errorf("observed max active renders = %d, want exactly 1 (semaphore must serialize)", got)
	}
}

// TestPreviewCanceledContext503 proves the AC1 queue-abort mapping: a request
// whose context is already canceled leaves the render queue without
// rendering and the handler answers 503 via jsonError.
func TestPreviewCanceledContext503(t *testing.T) {
	handler, previewSvc := setupPreviewSemaphoreTest(t)

	t.Run("preview", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/preview?name=sem-test", nil)
		ctx, cancel := context.WithCancel(req.Context())
		cancel()
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		handler.Preview(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want 503", rec.Code)
		}
		if got := previewSvc.ActiveRenders(); got != 0 {
			t.Errorf("ActiveRenders = %d, want 0 — canceled request must not render", got)
		}
	})

	t.Run("preview_live", func(t *testing.T) {
		vis := true
		design := models.DesignV2{
			Name: "live", Version: 2,
			Canvas: models.CanvasConfig{Width: 800, Height: 480, Background: "#FFFFFF"},
			Elements: []models.Element{{
				ID: "s1", Type: "shape", X: 0, Y: 0, Width: 100, Height: 100,
				ZIndex: 0, Visible: &vis, Properties: map[string]any{"fill": "#000000"},
			}},
		}
		body, err := json.Marshal(design)
		if err != nil {
			t.Fatal(err)
		}

		req := httptest.NewRequest("POST", "/api/preview_live", bytes.NewReader(body))
		ctx, cancel := context.WithCancel(req.Context())
		cancel()
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		handler.PreviewLive(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want 503", rec.Code)
		}
	})
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
