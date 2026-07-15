package services

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"testing"

	"e-ink-picture/server/internal/models"
)

// buildBlackShapeDesign returns a design with a single black, axis-parallel
// rectangle (no stroke, no corner radius) on a white 800x480 canvas.
func buildBlackShapeDesign(x, y, w, h, rotation float64) *models.DesignV2 {
	vis := true
	return &models.DesignV2{
		Name:    "rotation-test",
		Version: 2,
		Canvas:  models.CanvasConfig{Width: 800, Height: 480, Background: "#FFFFFF"},
		Elements: []models.Element{
			{
				ID: "r1", Type: "shape",
				X: x, Y: y, Width: w, Height: h, Rotation: rotation,
				ZIndex: 0, Visible: &vis,
				Properties: map[string]any{"fill": "#000000"},
			},
		},
	}
}

// renderFastRaw renders a design at render_quality=fast (scale 1.0, no
// supersampling blur) without palette quantization and returns the PNG bytes.
func renderFastRaw(t *testing.T, design *models.DesignV2) []byte {
	t.Helper()
	previewSvc, _ := setupGoldenServices(t, models.DisplayWaveshare75V2, models.RenderQualityFast)
	data, err := previewSvc.Render(context.Background(), design, true)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	return data
}

func decodePNG(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode PNG: %v", err)
	}
	return img
}

// TestRotationRightAnglesAxisParallel proves that rotations by exact multiples
// of 90° around the top-left anchor stay exactly axis-parallel (AC4): a black
// 200x100 rectangle anchored at (400, 200) must occupy exactly the expected
// half-open pixel rectangle — every pixel inside is black, every pixel outside
// is white, tolerance 0. This only holds because 90°-multiples use exact
// {-1, 0, 1} matrix coefficients instead of math.Sin/Cos float dust.
func TestRotationRightAnglesAxisParallel(t *testing.T) {
	cases := []struct {
		rotation               float64
		minX, minY, maxX, maxY int // expected occupied rect, half-open [min, max)
	}{
		{0, 400, 200, 600, 300},
		{90, 300, 200, 400, 400},
		{180, 200, 100, 400, 200},
		{270, 400, 0, 500, 200},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("rotation_%v", tc.rotation), func(t *testing.T) {
			design := buildBlackShapeDesign(400, 200, 200, 100, tc.rotation)
			img := decodePNG(t, renderFastRaw(t, design))

			bounds := img.Bounds()
			if bounds.Dx() != 800 || bounds.Dy() != 480 {
				t.Fatalf("expected 800x480 output at fast quality, got %dx%d", bounds.Dx(), bounds.Dy())
			}

			mismatches := 0
			for y := 0; y < 480; y++ {
				for x := 0; x < 800; x++ {
					r, g, b, _ := img.At(x, y).RGBA()
					inside := x >= tc.minX && x < tc.maxX && y >= tc.minY && y < tc.maxY
					isBlack := r == 0 && g == 0 && b == 0
					isWhite := r == 0xffff && g == 0xffff && b == 0xffff
					if inside && !isBlack {
						mismatches++
						if mismatches <= 5 {
							t.Errorf("pixel (%d,%d) inside expected rect is not black: R=%d G=%d B=%d", x, y, r>>8, g>>8, b>>8)
						}
					} else if !inside && !isWhite {
						mismatches++
						if mismatches <= 5 {
							t.Errorf("pixel (%d,%d) outside expected rect is not white: R=%d G=%d B=%d", x, y, r>>8, g>>8, b>>8)
						}
					}
				}
			}
			if mismatches > 0 {
				t.Fatalf("rotation %v: %d pixels violate exact axis parallelism", tc.rotation, mismatches)
			}
		})
	}
}

// TestRotationNegativeAngleEquivalence proves the angle normalization: a
// rotation of -90° must render byte-identically to 270°.
func TestRotationNegativeAngleEquivalence(t *testing.T) {
	neg := renderFastRaw(t, buildBlackShapeDesign(400, 200, 200, 100, -90))
	pos := renderFastRaw(t, buildBlackShapeDesign(400, 200, 200, 100, 270))
	if !bytes.Equal(neg, pos) {
		t.Error("rotation -90 and 270 produced different bytes — angle normalization is broken")
	}
}

// TestRotatedElementCulling proves the culling uses the rotated AABB (AC6):
// an element whose unrotated box lies fully off-canvas (y+h = -20 < 0) but
// whose rotated body reaches into the canvas must be rendered. Counter-check:
// an element that stays fully off-canvas even after rotation renders
// byte-identically to an empty canvas (the skip path still works).
func TestRotatedElementCulling(t *testing.T) {
	// Unrotated box: x=50, y=-60, w=200, h=40 — fully above the canvas.
	// Rotated 90° clockwise around (50,-60) it occupies [10,50) x [-60,140),
	// clipped to the canvas: [10,50) x [0,140).
	design := buildBlackShapeDesign(50, -60, 200, 40, 90)
	img := decodePNG(t, renderFastRaw(t, design))

	for y := 0; y < 140; y++ {
		for x := 10; x < 50; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r != 0 || g != 0 || b != 0 {
				t.Fatalf("expected black pixel at (%d,%d) from rotated element, got R=%d G=%d B=%d — rotated AABB culling skipped a visible element", x, y, r>>8, g>>8, b>>8)
			}
		}
	}

	// Counter-check: fully off-canvas even after rotation -> skip path.
	offDesign := buildBlackShapeDesign(-500, -500, 200, 40, 90)
	offRendered := renderFastRaw(t, offDesign)

	emptyDesign := &models.DesignV2{
		Name:    "rotation-test-empty",
		Version: 2,
		Canvas:  models.CanvasConfig{Width: 800, Height: 480, Background: "#FFFFFF"},
	}
	emptyRendered := renderFastRaw(t, emptyDesign)

	if !bytes.Equal(offRendered, emptyRendered) {
		t.Error("fully off-canvas rotated element changed the output — culling skip path is broken")
	}
}
