package services

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"math"
	"testing"

	"e-ink-picture/server/internal/models"
)

// B2 geometry verification (specs/B2-rounding-parity.md, AC1–AC3). All
// measurements run against the RAW (unquantized) high-quality render of
// rounding.json so corner geometry is read straight off the anti-aliased
// 800x480 panel raster, browser-free and deterministic.

// renderRoundingRaw renders rounding.json raw at render_quality=high.
func renderRoundingRaw(t *testing.T) image.Image {
	t.Helper()
	svc, _ := setupGoldenServices(t, models.DisplayWaveshare73E, models.RenderQualityHigh)
	design := loadTestDesign(t, "rounding")
	raw, err := svc.Render(context.Background(), design, true)
	if err != nil {
		t.Fatalf("raw render: %v", err)
	}
	im, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return im
}

// fillFrac returns 0 for the white background and 1 for a fully saturated
// (non-white) pixel, using the largest per-channel drop from 255.
func fillFrac(im image.Image, x, y int) float64 {
	r, g, b, _ := im.At(x, y).RGBA()
	R, G, B := float64(r>>8), float64(g>>8), float64(b>>8)
	return math.Max(255-R, math.Max(255-G, 255-B)) / 255
}

// strongFill reports whether the pixel is a saturated (near-solid) color, used
// to pinpoint the tangent point where a straight edge begins.
func strongFill(im image.Image, x, y int) bool {
	r, g, b, _ := im.At(x, y).RGBA()
	R, G, B := int(r>>8), int(g>>8), int(b>>8)
	return (255-R)+(255-G)+(255-B) > 150
}

// cornerRadiusDiagonal measures the corner radius of a shape whose top-left
// box corner is at (x, y). Along the 45° diagonal from the box corner the
// rounded boundary is steep (crisp AA), crossing at distance R·(1-1/√2) from
// the corner; the sub-pixel 50%-coverage crossing recovers R analytically.
func cornerRadiusDiagonal(im image.Image, x, y int) float64 {
	const k = 1.0 - math.Sqrt2/2 // 1 - 1/√2 ≈ 0.29289
	prev := fillFrac(im, x, y)
	for t := 1; t <= 80; t++ {
		cur := fillFrac(im, x+t, y+t)
		if prev < 0.5 && cur >= 0.5 {
			frac := (0.5 - prev) / (cur - prev)
			return (float64(t-1) + frac) / k
		}
		prev = cur
	}
	return -1
}

// flatTopRadius returns where the straight top edge begins (≈ rx): the first
// column, scanning right from the box's left edge, whose top-edge pixel is a
// solid fill.
func flatTopRadius(im image.Image, x, y, w int) int {
	for px := x; px < x+w/2; px++ {
		if strongFill(im, px, y) {
			return px - x
		}
	}
	return -1
}

// flatLeftRadius returns where the straight left edge begins (≈ ry).
func flatLeftRadius(im image.Image, x, y, h int) int {
	for py := y; py < y+h/2; py++ {
		if strongFill(im, x, py) {
			return py - y
		}
	}
	return -1
}

// TestShapeRoundingGeometry proves the B2 corner/stroke geometry on the panel
// raster: the radius scales with the design radius (and is never the old
// halved value, AC1), wide-flat boxes get elliptical (per-axis clamped)
// corners rather than a single circle (AC2), and the stroke is a centered ring
// with a transparent-fill interior left as background (AC3).
func TestShapeRoundingGeometry(t *testing.T) {
	im := renderRoundingRaw(t)

	// AC1 — radius scaling, never halved. The diagonal measurement carries a
	// small (~2px) inward bias from AA and the pixel-center convention, so the
	// tolerance is ±3px; the not-halved guard (R > wantR·0.6) is what catches a
	// D0 regression (which would render R/2 ≈ measured wantR/2 - 2).
	t.Run("AC1_radius_not_halved", func(t *testing.T) {
		cases := []struct {
			name    string
			x, y, R int
		}{
			{"rx8", 155, 20, 8},
			{"rx16", 290, 20, 16},
			{"rx24", 425, 20, 24},
			{"rx40", 560, 20, 40},
		}
		for _, c := range cases {
			got := cornerRadiusDiagonal(im, c.x, c.y)
			want := float64(c.R)
			if math.Abs(got-want) > 3.0 {
				t.Errorf("%s: measured corner radius %.1f, want %d (±3px)", c.name, got, c.R)
			}
			if got <= want*0.6 {
				t.Errorf("%s: measured radius %.1f ≈ halved (want ~%d); D0 scale fix missing", c.name, got, c.R)
			}
		}
	})

	// AC2 — elliptical corners with independent per-axis clamp. The pill
	// (w=200,h=60,rx=100) must clamp rx→100 (w/2) and ry→30 (h/2): a wide
	// ellipse, NOT a circle of radius 30. The rx≠ry box (rx=40, ry=15) must
	// keep a horizontal radius clearly larger than the vertical one.
	t.Run("AC2_elliptical_corners", func(t *testing.T) {
		// pill: x=20,y=140,w=200,h=60
		ph := flatTopRadius(im, 20, 140, 200)
		pv := flatLeftRadius(im, 20, 140, 60)
		if ph < 78 || ph > 102 {
			t.Errorf("pill horizontal radius %d, want ~100 (clamped to w/2); a circle would give ~30", ph)
		}
		if pv < 24 || pv > 33 {
			t.Errorf("pill vertical radius %d, want ~30 (clamped to h/2)", pv)
		}
		if float64(ph) < float64(pv)*2.5 {
			t.Errorf("pill horizontal %d not >> vertical %d — corners are a circle, not an ellipse", ph, pv)
		}
		// ellipse: x=250,y=140,w=140,h=70, rx=40, ry=15
		eh := flatTopRadius(im, 250, 140, 140)
		ev := flatLeftRadius(im, 250, 140, 70)
		if eh < 28 || eh > 44 {
			t.Errorf("ellipse horizontal radius %d, want ~40", eh)
		}
		if ev < 9 || ev > 19 {
			t.Errorf("ellipse vertical radius %d, want ~15", ev)
		}
		if eh <= ev+8 {
			t.Errorf("ellipse rx≠ry not honored: horizontal %d, vertical %d", eh, ev)
		}
	})

	// AC3 — centered stroke ring. r_stroke_geom: x=530,y=240,w=180,h=90,
	// fill green, stroke black, strokeWidth=12 → at high (scale 2, /2 back)
	// half-width 6: outer edge at x-6=524, fill boundary at x+6=536.
	t.Run("AC3_centered_stroke", func(t *testing.T) {
		const gy = 285 // mid-height of the box
		// outer edge: first non-background pixel scanning right from x=512
		outer := -1
		for px := 512; px < 545; px++ {
			if fillFrac(im, px, gy) > 0.5 {
				outer = px
				break
			}
		}
		if outer < 522 || outer > 526 {
			t.Errorf("stroke outer edge at x=%d, want ~524 (x - strokeWidth/2)", outer)
		}
		// fill boundary: first green pixel (stroke is black, fill is green)
		isGreen := func(px int) bool {
			r, g, b, _ := im.At(px, gy).RGBA()
			return g>>8 > 150 && r>>8 < 120 && b>>8 < 120
		}
		fill := -1
		for px := outer; px < 560; px++ {
			if isGreen(px) {
				fill = px
				break
			}
		}
		if fill < 534 || fill > 538 {
			t.Errorf("fill/stroke boundary at x=%d, want ~536 (x + strokeWidth/2)", fill)
		}
		// the path edge (x=530) sits inside the black stroke band
		if r, g, b, _ := im.At(530, gy).RGBA(); r>>8 > 96 || g>>8 > 96 || b>>8 > 96 {
			t.Errorf("path edge (530,%d) not stroke-colored: rgb=(%d,%d,%d)", gy, r>>8, g>>8, b>>8)
		}
	})

	// AC3 — transparent-fill ring. r_ring: x=20,y=240,w=160,h=90, fill
	// transparent, stroke blue. The interior center must remain background
	// (not stroke-colored), and the path edge must carry the blue stroke.
	t.Run("AC3_transparent_ring", func(t *testing.T) {
		if f := fillFrac(im, 100, 285); f > 0.1 {
			t.Errorf("ring interior center is not background (fillFrac=%.2f) — transparent fill drew a solid box", f)
		}
		// left path edge x=20 at mid-height must be blue stroke
		r, g, b, _ := im.At(20, 285).RGBA()
		if b>>8 < 150 || r>>8 > 120 || g>>8 > 120 {
			t.Errorf("ring left edge (20,285) not blue stroke: rgb=(%d,%d,%d)", r>>8, g>>8, b>>8)
		}
	})
}
