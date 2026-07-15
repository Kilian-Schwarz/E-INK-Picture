package services

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"sync"
	"testing"
	"time"

	"e-ink-picture/server/internal/models"
)

// buildShapeDesign returns a deterministic, font-free design: two flat
// rectangles on a white canvas of the given size.
func buildShapeDesign(canvasW, canvasH int) *models.DesignV2 {
	vis := true
	return &models.DesignV2{
		Name:    "concurrency-test",
		Version: 2,
		Canvas:  models.CanvasConfig{Width: canvasW, Height: canvasH, Background: "#FFFFFF"},
		Elements: []models.Element{
			{
				ID: "s1", Type: "shape",
				X: 20, Y: 16, Width: float64(canvasW) / 2, Height: float64(canvasH) / 2,
				ZIndex: 0, Visible: &vis,
				Properties: map[string]any{"fill": "#000000"},
			},
			{
				ID: "s2", Type: "shape",
				X: float64(canvasW) / 3, Y: float64(canvasH) / 3, Width: float64(canvasW) / 3, Height: float64(canvasH) / 3,
				ZIndex: 1, Visible: &vis,
				Properties: map[string]any{"fill": "#FF0000"},
			},
		},
	}
}

// TestRenderContextCanceled proves the AC1 queue-abort behavior at the
// service level: a canceled context leaves the render queue with ctx.Err()
// and never enters the render body (ActiveRenders stays 0).
func TestRenderContextCanceled(t *testing.T) {
	t.Run("already_canceled_before_acquire", func(t *testing.T) {
		previewSvc, _ := setupGoldenServices(t, models.DisplayWaveshare75V2, models.RenderQualityFast)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		out, err := previewSvc.Render(ctx, buildShapeDesign(800, 480), false)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
		if out != nil {
			t.Error("expected nil output for canceled context")
		}
		if got := previewSvc.ActiveRenders(); got != 0 {
			t.Errorf("ActiveRenders = %d, want 0", got)
		}
	})

	t.Run("canceled_while_queued", func(t *testing.T) {
		previewSvc, _ := setupGoldenServices(t, models.DisplayWaveshare75V2, models.RenderQualityFast)

		// Occupy the semaphore so the render below has to queue.
		previewSvc.renderSem <- struct{}{}
		defer func() { <-previewSvc.renderSem }()

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			_, err := previewSvc.Render(ctx, buildShapeDesign(800, 480), false)
			done <- err
		}()

		time.Sleep(20 * time.Millisecond) // let the goroutine block in the queue
		cancel()

		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("err = %v, want context.Canceled", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("queued render did not abort after context cancel")
		}
		if got := previewSvc.ActiveRenders(); got != 0 {
			t.Errorf("ActiveRenders = %d, want 0 — queued request must not enter the render body", got)
		}
	})
}

// TestParallelTextRendersNoFontRace is the regression test for the shared
// font.Face data race: opentype.Face is explicitly NOT safe for concurrent
// use (lazy metrics, sfnt.Buffer, rasterizer and alpha mask mutate per
// glyph), so the font cache must hand out a fresh Face per call and only
// cache the parsed *opentype.Font. With a Face instance shared between
// renders this test is red under -race at semaphore capacity 2 (and
// panic-prone: "index out of range" in drawGlyphOver).
//
// The warmup render populates the font cache so both parallel renders hit
// the same cached entry.
func TestParallelTextRendersNoFontRace(t *testing.T) {
	previewSvc, _ := setupGoldenServices(t, models.DisplayWaveshare75V2, models.RenderQualityFast)
	previewSvc.SetMaxConcurrentRenders(2)
	design := loadTestDesign(t, "basic") // text elements with pinned testfont.ttf

	if _, err := previewSvc.Render(context.Background(), design, false); err != nil {
		t.Fatalf("warmup render failed: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 2; j++ {
				if _, err := previewSvc.Render(context.Background(), design, false); err != nil {
					t.Errorf("parallel text render failed: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

// TestSetMaxConcurrentRenders covers the semaphore sizing (values < 1 clamp
// to 1, configured values resize the buffered channel).
func TestSetMaxConcurrentRenders(t *testing.T) {
	previewSvc, _ := setupGoldenServices(t, models.DisplayWaveshare75V2, models.RenderQualityFast)
	if got := cap(previewSvc.renderSem); got != 1 {
		t.Errorf("default semaphore capacity = %d, want 1", got)
	}
	previewSvc.SetMaxConcurrentRenders(4)
	if got := cap(previewSvc.renderSem); got != 4 {
		t.Errorf("semaphore capacity after SetMaxConcurrentRenders(4) = %d, want 4", got)
	}
	previewSvc.SetMaxConcurrentRenders(0)
	if got := cap(previewSvc.renderSem); got != 1 {
		t.Errorf("semaphore capacity after SetMaxConcurrentRenders(0) = %d, want 1 (clamped)", got)
	}
}

// TestScalerCacheBounded proves AC3b: the downscale scaler cache holds at
// most maxScalerCacheEntries geometries; unknown geometries beyond that get
// nil (callers fall back to the unpooled xdraw.CatmullRom.Scale), while
// cached geometries keep being served.
func TestScalerCacheBounded(t *testing.T) {
	previewSvc, _ := setupGoldenServices(t, models.DisplayWaveshare75V2, models.RenderQualityHigh)

	keys := []scalerKey{
		{dw: 800, dh: 480, sw: 1600, sh: 960},
		{dw: 800, dh: 480, sw: 1200, sh: 720},
		{dw: 640, dh: 384, sw: 1280, sh: 768},
	}
	for _, k := range keys {
		if sc := previewSvc.downscaleScaler(k.dw, k.dh, k.sw, k.sh); sc == nil {
			t.Fatalf("expected cached scaler for %+v, got nil", k)
		}
	}

	// Cache is full: a fourth geometry must NOT be cached.
	if sc := previewSvc.downscaleScaler(400, 300, 800, 600); sc != nil {
		t.Error("expected nil (fallback) for a fourth geometry, cache must stay bounded")
	}
	if got := len(previewSvc.scalerCache); got != maxScalerCacheEntries {
		t.Errorf("scaler cache holds %d entries, want %d", got, maxScalerCacheEntries)
	}

	// Known geometries keep hitting the cache.
	for _, k := range keys {
		if sc := previewSvc.downscaleScaler(k.dw, k.dh, k.sw, k.sh); sc == nil {
			t.Errorf("cached geometry %+v no longer served", k)
		}
	}
}

// TestRenderUnusualCanvasFallback proves AC3b end-to-end: a design-driven,
// non-standard canvas size renders correctly, and the fallback path (cache
// full -> xdraw.CatmullRom.Scale) is byte-identical to the pooled scaler
// path (Fakt 2 in specs/E5.6-render-memory.md).
func TestRenderUnusualCanvasFallback(t *testing.T) {
	design := buildShapeDesign(636, 380)

	// Path A: fresh service, geometry gets a pooled scaler.
	svcA, tmpDir := setupGoldenServices(t, models.DisplayWaveshare73E, models.RenderQualityHigh)
	pooled, err := svcA.Render(context.Background(), design, false)
	if err != nil {
		t.Fatalf("pooled-path render failed: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(pooled))
	if err != nil {
		t.Fatalf("decode rendered PNG: %v", err)
	}
	if img.Bounds().Dx() != 636 || img.Bounds().Dy() != 380 {
		t.Fatalf("rendered size %dx%d, want 636x380", img.Bounds().Dx(), img.Bounds().Dy())
	}

	// Path B: same data dir, but the scaler cache is pre-filled with three
	// foreign geometries, forcing the unpooled Kernel.Scale fallback.
	svcB := newGoldenPreviewService(tmpDir)
	svcB.downscaleScaler(10, 10, 20, 20)
	svcB.downscaleScaler(11, 11, 22, 22)
	svcB.downscaleScaler(12, 12, 24, 24)
	if sc := svcB.downscaleScaler(636, 380, 1272, 760); sc != nil {
		t.Fatal("precondition failed: expected full cache to force the fallback path")
	}

	fallback, err := svcB.Render(context.Background(), design, false)
	if err != nil {
		t.Fatalf("fallback-path render failed: %v", err)
	}
	if !bytes.Equal(pooled, fallback) {
		t.Error("pooled scaler and CatmullRom.Scale fallback produced different bytes — B2 must be byte-identical")
	}
}
