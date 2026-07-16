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

	"e-ink-picture/server/internal/auth"
	"e-ink-picture/server/internal/middleware"
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

// TestPreviewEndpointAuthCSRFStatusCodes locks the B7/AC3 endpoint
// status-code contract (specs/B7-repro-protocol.md §1, specs/B7-designer-preview.md
// AC3). The status codes are produced by the auth+CSRF Guard sitting in front of
// the router, so the regression test wires the REAL PreviewHandler behind the
// REAL Guard — exactly the composition from main.go (Guard(mux)).
//
// The distinction the frontend relies on:
//   - a cross-origin mutating request must be 403 (NOT 401), because auth.js
//     only redirects on 401; a 403 has to reach toolbar.js's cross-origin
//     branch instead of being funnelled into the saved-design fallback.
//   - a missing/expired session must be 401 so the client redirect fires.
//   - the happy path must still return a decodable image/png PNG.
//
// Hermetic: httptest only, no network/panel; deterministic font-free shapes.
func TestPreviewEndpointAuthCSRFStatusCodes(t *testing.T) {
	handler, _ := setupPreviewTestServices(t)

	vis := true

	// A saved, font-free design for the GET /preview happy path.
	savedDesign := &models.DesignV2{
		Name:    "ac3-preview",
		Version: 2,
		Canvas:  models.CanvasConfig{Width: 800, Height: 480, Background: "#FFFFFF"},
		Elements: []models.Element{{
			ID: "s1", Type: "shape", X: 40, Y: 40, Width: 200, Height: 120,
			ZIndex: 0, Visible: &vis, Properties: map[string]any{"fill": "#000000"},
		}},
	}
	if err := handler.designSvc.Save("ac3-preview", savedDesign); err != nil {
		t.Fatalf("save design: %v", err)
	}

	// A font-free live design for POST /api/preview_live.
	liveBody, err := json.Marshal(models.DesignV2{
		Name:    "ac3-live",
		Version: 2,
		Canvas:  models.CanvasConfig{Width: 800, Height: 480, Background: "#FFFFFF"},
		Elements: []models.Element{{
			ID: "s1", Type: "shape", X: 10, Y: 10, Width: 100, Height: 100,
			ZIndex: 0, Visible: &vis, Properties: map[string]any{"fill": "#000000"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Auth active (password set) so the guard actually enforces.
	mgr, err := auth.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := mgr.SetPassword("test-password"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	const host = "pi.local:5000"

	// newGuard wraps the real preview routes with the real Guard for a given
	// session store and client-token config — mirrors main.go's Guard(mux).
	newGuard := func(store *auth.Store, clientToken string) http.Handler {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /preview", handler.Preview)
		mux.HandleFunc("POST /api/preview_live", handler.PreviewLive)
		return middleware.Guard(middleware.GuardConfig{
			Manager:     mgr,
			Sessions:    store,
			ClientToken: clientToken,
		})(mux)
	}

	do := func(h http.Handler, method, target string, body []byte, mutate func(*http.Request)) *httptest.ResponseRecorder {
		var rdr io.Reader
		if body != nil {
			rdr = bytes.NewReader(body)
		}
		req := httptest.NewRequest(method, target, rdr)
		req.Host = host
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if mutate != nil {
			mutate(req)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	assertPNG := func(t *testing.T, rec *httptest.ResponseRecorder) {
		t.Helper()
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
			t.Errorf("Content-Type = %q, want image/png", ct)
		}
		if _, err := png.Decode(bytes.NewReader(rec.Body.Bytes())); err != nil {
			t.Errorf("body is not a valid PNG: %v", err)
		}
	}

	store := auth.NewStore()
	token, err := store.Create()
	if err != nil {
		t.Fatalf("store.Create: %v", err)
	}
	cookie := &http.Cookie{Name: middleware.SessionCookieName, Value: token}
	srv := newGuard(store, "")

	t.Run("preview_live valid session same-origin -> 200 PNG", func(t *testing.T) {
		rec := do(srv, "POST", "/api/preview_live", liveBody, func(r *http.Request) {
			r.AddCookie(cookie)
			r.Header.Set("Origin", "http://"+host)
		})
		assertPNG(t, rec)
	})

	t.Run("preview_live valid session FOREIGN origin -> 403 not 401", func(t *testing.T) {
		rec := do(srv, "POST", "/api/preview_live", liveBody, func(r *http.Request) {
			r.AddCookie(cookie)
			r.Header.Set("Origin", "http://evil.example")
		})
		if rec.Code == http.StatusUnauthorized {
			t.Fatalf("status = 401, want 403 — a cross-origin rejection MUST be 403 so it reaches toolbar.js's cross-origin branch, not auth.js's 401 redirect")
		}
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rec.Code)
		}
	})

	t.Run("preview_live no session -> 401", func(t *testing.T) {
		rec := do(srv, "POST", "/api/preview_live", liveBody, func(r *http.Request) {
			r.Header.Set("Origin", "http://"+host)
		})
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401 (missing session)", rec.Code)
		}
	})

	t.Run("preview_live expired session -> 401", func(t *testing.T) {
		expStore := auth.NewStore()
		var mu sync.Mutex
		now := time.Now()
		expStore.SetClock(func() time.Time {
			mu.Lock()
			defer mu.Unlock()
			return now
		})
		expTok, err := expStore.Create()
		if err != nil {
			t.Fatalf("store.Create: %v", err)
		}
		expCookie := &http.Cookie{Name: middleware.SessionCookieName, Value: expTok}
		mu.Lock()
		now = now.Add(auth.SessionTTL + time.Minute)
		mu.Unlock()

		expSrv := newGuard(expStore, "")
		rec := do(expSrv, "POST", "/api/preview_live", liveBody, func(r *http.Request) {
			r.AddCookie(expCookie)
			r.Header.Set("Origin", "http://"+host)
		})
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401 (expired session)", rec.Code)
		}
	})

	t.Run("GET /preview valid session -> 200 PNG", func(t *testing.T) {
		rec := do(srv, "GET", "/preview?name=ac3-preview", nil, func(r *http.Request) {
			r.AddCookie(cookie)
		})
		assertPNG(t, rec)
	})

	t.Run("GET /preview no session/token -> 401", func(t *testing.T) {
		rec := do(srv, "GET", "/preview?name=ac3-preview", nil, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401 (no session, no token)", rec.Code)
		}
	})

	t.Run("GET /preview X-Client-Token -> 200 PNG", func(t *testing.T) {
		const clientTok = "ac3-client-token"
		tokSrv := newGuard(auth.NewStore(), clientTok)
		rec := do(tokSrv, "GET", "/preview?name=ac3-preview", nil, func(r *http.Request) {
			r.Header.Set(middleware.ClientTokenHeader, clientTok)
		})
		assertPNG(t, rec)
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
