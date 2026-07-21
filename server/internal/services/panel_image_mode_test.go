package services

import (
	"bytes"
	"context"
	"image/color"
	"image/png"
	"testing"

	"e-ink-picture/server/internal/models"
)

// countUniqueRGB decodes a PNG and returns the number of distinct RGB values
// (alpha ignored). It is the proof instrument for the two send modes (F10 AC4):
// the dithered frame stays within the driver palette, the raw frame does not.
func countUniqueRGB(t *testing.T, pngData []byte) int {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		t.Fatalf("decode PNG: %v", err)
	}
	seen := make(map[color.RGBA]struct{})
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			seen[color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(bl >> 8), A: 255}] = struct{}{}
		}
	}
	return len(seen)
}

// TestRawOutputIsUnquantized proves the two panel image modes genuinely differ
// (F10 AC4): rendering the photo-like gradient with raw=true yields far more
// unique RGB values than the driver palette holds, while raw=false stays within
// the palette. This is what wiring panel_image_mode=original vs dithered buys.
func TestRawOutputIsUnquantized(t *testing.T) {
	for _, displayType := range goldenDisplays {
		t.Run(string(displayType), func(t *testing.T) {
			previewSvc, _ := setupGoldenServices(t, displayType, models.RenderQualityHigh)
			design := loadTestDesign(t, "gradient")
			paletteSize := len(models.GetDisplayConfig(displayType).Colors)

			quantized, err := previewSvc.Render(context.Background(), design, false)
			if err != nil {
				t.Fatalf("Render(raw=false): %v", err)
			}
			if got := countUniqueRGB(t, quantized); got > paletteSize {
				t.Errorf("dithered output has %d unique colors, want <= palette size %d", got, paletteSize)
			}

			raw, err := previewSvc.Render(context.Background(), design, true)
			if err != nil {
				t.Fatalf("Render(raw=true): %v", err)
			}
			if got := countUniqueRGB(t, raw); got <= paletteSize {
				t.Errorf("raw output has %d unique colors, want > palette size %d (should be unquantized)", got, paletteSize)
			}
		})
	}
}
