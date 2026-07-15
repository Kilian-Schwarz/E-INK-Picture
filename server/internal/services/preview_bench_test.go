package services

import (
	"context"
	"runtime"
	"testing"

	"e-ink-picture/server/internal/models"
)

// renderAllocDelta renders once and returns the runtime.MemStats TotalAlloc
// delta. TotalAlloc is monotonic and independent of GC timing, so the value
// is deterministic up to small map/slice-growth noise.
func renderAllocDelta(t *testing.T, svc *PreviewService, design *models.DesignV2) uint64 {
	t.Helper()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	if _, err := svc.Render(context.Background(), design, false); err != nil {
		t.Fatalf("measured render failed: %v", err)
	}
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// TestRenderAllocBudget (AC4, specs/E5.6-render-memory.md): a warm
// high-quality render of the basic design (floyd_steinberg, calibration
// default) must allocate < 20 MiB. Derivation: after the buffer diet B1+B2
// roughly canvas 6.14 + downscale dst 1.54 + paletted 0.41 + text buffers
// (~3-6) + PNG/misc (~0.5) ~= 12-15 MiB remain; 20 MiB leaves margin but
// reliably catches any reintroduction of the 24.6 MiB unpooled scaler temp
// buffer (36+ MiB -> red).
//
// The minimum over five measured renders is compared: the pooled scaler
// buffer lives in a sync.Pool, which the GC may empty between two renders
// (victim cache: gone after two GC cycles). A regression to the unpooled
// path makes EVERY render exceed the budget, so taking the minimum is safe
// against false greens and immune to GC-timing flakes.
func TestRenderAllocBudget(t *testing.T) {
	previewSvc, _ := setupGoldenServices(t, models.DisplayWaveshare73E, models.RenderQualityHigh)
	design := loadTestDesign(t, "basic")

	// Two warmup renders: font cache and scaler cache warm.
	for i := 0; i < 2; i++ {
		if _, err := previewSvc.Render(context.Background(), design, false); err != nil {
			t.Fatalf("warmup render failed: %v", err)
		}
	}

	const budget = uint64(20 << 20) // 20 MiB = 20971520 B
	minDelta := renderAllocDelta(t, previewSvc, design)
	for i := 0; i < 4; i++ {
		if d := renderAllocDelta(t, previewSvc, design); d < minDelta {
			minDelta = d
		}
	}

	t.Logf("TotalAlloc delta per warm high-quality basic render: %d B (%.2f MiB), budget %d B (20 MiB)",
		minDelta, float64(minDelta)/(1<<20), budget)
	if minDelta >= budget {
		t.Errorf("render alloc budget exceeded: %d B (%.2f MiB) >= %d B (20 MiB)",
			minDelta, float64(minDelta)/(1<<20), budget)
	}
}

// BenchmarkRenderHighQuality measures a full high-quality render through the
// public API with the golden setup mechanics (floyd_steinberg, calibration
// default, waveshare_7in3_e). The gradient sub-benchmark covers the photo
// path (image decode + resizeImage), which intentionally stays expensive —
// see the buffer inventory #5/#6 in specs/E5.6-render-memory.md.
func BenchmarkRenderHighQuality(b *testing.B) {
	for _, designName := range []string{"basic", "gradient"} {
		b.Run(designName, func(b *testing.B) {
			previewSvc, _ := setupGoldenServices(b, models.DisplayWaveshare73E, models.RenderQualityHigh)
			design := loadTestDesign(b, designName)

			if _, err := previewSvc.Render(context.Background(), design, false); err != nil {
				b.Fatalf("warmup render failed: %v", err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := previewSvc.Render(context.Background(), design, false); err != nil {
					b.Fatalf("render failed: %v", err)
				}
			}
		})
	}
}
