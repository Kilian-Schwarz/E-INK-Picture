package services

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"testing"

	"e-ink-picture/server/internal/models"
)

// newUniformImage returns a w×h RGBA image filled with a single opaque color.
func newUniformImage(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(c), image.Point{}, draw.Src)
	return img
}

// TestAtkinsonDiffusionMatrix checks ditherAtkinson against index matrices
// computed BY HAND from the binding definition in
// specs/E1.6-panel-calibration.md (Architektur-Richtung point 3): row-major
// left-to-right scan, value=clamp(src+acc), nearest match by squared RGB
// distance with ties going to the LOWEST index, err/8 with Go truncation
// toward zero, diffused to exactly (x+1,y), (x+2,y), (x-1,y+1), (x,y+1),
// (x+1,y+1), (x,y+2), out-of-bounds shares dropped.
//
// The matrices are hard-coded — NOT generated from the implementation — so
// any change to weights, neighbors, scan order or tie-break turns this red.
func TestAtkinsonDiffusionMatrix(t *testing.T) {
	cases := []struct {
		name    string
		w, h    int
		fill    color.RGBA
		palette color.Palette
		want    [][]uint8
	}{
		{
			// Uniform #808080 against black/white. Hand computation of the
			// first row (single channel, all channels identical):
			//   x0: v=128 -> white (127^2 < 128^2), err=-127, -127/8=-15
			//   x1: acc-15 -> v=113 -> black, err=113, 113/8=14
			//   x2: acc(-15+14)=-1 -> v=127 -> black, err=127, 127/8=15
			//   x3: acc(14+15)=29 -> v=157 -> white, err=-98, -98/8=-12
			//   x4: acc(15-12)=3 -> v=131 -> white, err=-124, -124/8=-15
			//   x5: acc(-12-15)=-27 -> v=101 -> black, err=101, 101/8=12
			// Rows 1-3 continue with the accumulated row1/row2 errors.
			name: "gray128_bw_6x4",
			w:    6, h: 4,
			fill: color.RGBA{128, 128, 128, 255},
			palette: color.Palette{
				color.RGBA{0, 0, 0, 255},
				color.RGBA{255, 255, 255, 255},
			},
			want: [][]uint8{
				{1, 0, 0, 1, 1, 0},
				{0, 1, 1, 0, 0, 1},
				{0, 1, 1, 0, 0, 1},
				{1, 0, 0, 1, 1, 0},
			},
		},
		{
			// Tie-break: #404040 against {#000000, #808080}. At x0 the value
			// 64 is exactly equidistant (64^2*3 both) -> LOWEST index (0)
			// must win. err=64, 64/8=8 diffused to x1 and x2:
			//   x1: v=72 -> idx1 (56^2 < 72^2), err=-56, -56/8=-7
			//   x2: acc(8-7)=1 -> v=65 -> idx1 (63^2 < 65^2)
			name: "tiebreak_lowest_index_3x1",
			w:    3, h: 1,
			fill: color.RGBA{64, 64, 64, 255},
			palette: color.Palette{
				color.RGBA{0, 0, 0, 255},
				color.RGBA{128, 128, 128, 255},
			},
			want: [][]uint8{
				{0, 1, 1},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := newUniformImage(tc.w, tc.h, tc.fill)
			got := ditherAtkinson(src, tc.palette)

			for y := 0; y < tc.h; y++ {
				for x := 0; x < tc.w; x++ {
					if idx := got.Pix[y*got.Stride+x]; idx != tc.want[y][x] {
						t.Errorf("pixel (%d,%d): got index %d, want %d", x, y, idx, tc.want[y][x])
					}
				}
			}
		})
	}
}

// TestCalibrationOffMatchesLegacy proves AC2: calibration "off" with
// floyd_steinberg is byte-exactly the pre-E1.6 behavior. The reference is the
// LITERAL legacy quantizeToPalette body inlined here (palette from
// DisplayConfig.Colors + stdlib draw.FloydSteinberg), NOT the new pipeline.
func TestCalibrationOffMatchesLegacy(t *testing.T) {
	src := newGradientImage(800, 480)

	for _, displayType := range goldenDisplays {
		t.Run(string(displayType), func(t *testing.T) {
			cfg := models.GetDisplayConfig(displayType)

			// Legacy reference (verbatim pre-E1.6 quantizeToPalette).
			pal := make(color.Palette, 0, len(cfg.Colors))
			for _, hex := range cfg.Colors {
				pal = append(pal, parseHexColor(hex))
			}
			if len(pal) == 0 {
				pal = color.Palette{color.White, color.Black}
			}
			bounds := src.Bounds()
			legacy := image.NewPaletted(bounds, pal)
			draw.FloydSteinberg.Draw(legacy, bounds, src, image.Point{})

			got := quantizeForDisplay(src, cfg, models.DitherFloydSteinberg, models.CalibrationOff)

			if got.Bounds() != legacy.Bounds() || got.Stride != legacy.Stride {
				t.Fatalf("geometry differs: got %v/%d, want %v/%d", got.Bounds(), got.Stride, legacy.Bounds(), legacy.Stride)
			}
			if !bytes.Equal(got.Pix, legacy.Pix) {
				diff := 0
				for i := range got.Pix {
					if got.Pix[i] != legacy.Pix[i] {
						diff++
					}
				}
				t.Errorf("pixel indices differ from legacy Floyd-Steinberg output: %d of %d bytes", diff, len(legacy.Pix))
			}
			if len(got.Palette) != len(legacy.Palette) {
				t.Fatalf("palette length %d, want %d", len(got.Palette), len(legacy.Palette))
			}
			for i := range legacy.Palette {
				wr, wg, wb, wa := legacy.Palette[i].RGBA()
				gr, gg, gb, ga := got.Palette[i].RGBA()
				if wr != gr || wg != gg || wb != gb || wa != ga {
					t.Errorf("palette[%d] differs: got %v, want %v", i, got.Palette[i], legacy.Palette[i])
				}
			}
		})
	}
}

// TestCalibrationAffectsOnlyCalibratedProfiles proves AC4: on epd7in3e the
// calibration changes the rendered bytes, on the identity-profile 7.5" V2 it
// must not change a single byte.
func TestCalibrationAffectsOnlyCalibratedProfiles(t *testing.T) {
	renderGradient := func(t *testing.T, displayType models.DisplayType, mode models.CalibrationMode) []byte {
		t.Helper()
		previewSvc, _ := setupGoldenServicesVariant(t, displayType, models.RenderQualityHigh, models.DitherFloydSteinberg, mode)
		out, err := previewSvc.Render(loadTestDesign(t, "gradient"), false)
		if err != nil {
			t.Fatalf("Render failed: %v", err)
		}
		return out
	}

	t.Run("waveshare_7in3_e_default_differs_from_off", func(t *testing.T) {
		withCal := renderGradient(t, models.DisplayWaveshare73E, models.CalibrationDefault)
		withoutCal := renderGradient(t, models.DisplayWaveshare73E, models.CalibrationOff)
		if bytes.Equal(withCal, withoutCal) {
			t.Error("calibration default produced identical bytes to off — panel calibration has no effect")
		}
	})

	t.Run("waveshare_7in5_v2_default_equals_off", func(t *testing.T) {
		withCal := renderGradient(t, models.DisplayWaveshare75V2, models.CalibrationDefault)
		withoutCal := renderGradient(t, models.DisplayWaveshare75V2, models.CalibrationOff)
		if !bytes.Equal(withCal, withoutCal) {
			t.Error("identity profile must be byte-identical with calibration on and off")
		}
	})
}

// TestDitherAlgorithmsProduceDifferentOutput proves AC5b: Atkinson and
// Floyd-Steinberg produce different bytes on the gradient design for both
// display profiles (palette exactness of both is covered by
// TestPaletteExactness).
func TestDitherAlgorithmsProduceDifferentOutput(t *testing.T) {
	for _, displayType := range goldenDisplays {
		t.Run(string(displayType), func(t *testing.T) {
			render := func(algo models.DitherAlgorithm) []byte {
				previewSvc, _ := setupGoldenServicesVariant(t, displayType, models.RenderQualityHigh, algo, models.CalibrationDefault)
				out, err := previewSvc.Render(loadTestDesign(t, "gradient"), false)
				if err != nil {
					t.Fatalf("Render failed: %v", err)
				}
				return out
			}
			if bytes.Equal(render(models.DitherFloydSteinberg), render(models.DitherAtkinson)) {
				t.Error("floyd_steinberg and atkinson produced identical bytes")
			}
		})
	}
}

// TestGetCalibrationProfile covers the profile invariants and the defensive
// identity fallback (Architektur-Richtung point 5).
func TestGetCalibrationProfile(t *testing.T) {
	t.Run("known_profiles_match_driver_palette_length", func(t *testing.T) {
		for displayType := range models.DisplayProfiles {
			cfg := models.GetDisplayConfig(displayType)
			profile := models.GetCalibrationProfile(displayType)
			if len(profile.PanelPalette) != len(cfg.Colors) {
				t.Errorf("%s: panel palette has %d entries, driver palette %d", displayType, len(profile.PanelPalette), len(cfg.Colors))
			}
		}
	})

	t.Run("bw_profile_is_identity", func(t *testing.T) {
		cfg := models.GetDisplayConfig(models.DisplayWaveshare75V2)
		profile := models.GetCalibrationProfile(models.DisplayWaveshare75V2)
		if !profile.Precomp.IsIdentity() {
			t.Errorf("expected identity precompensation, got %+v", profile.Precomp)
		}
		for i, hex := range cfg.Colors {
			if profile.PanelPalette[i] != hex {
				t.Errorf("panel palette[%d] = %s, want driver color %s", i, profile.PanelPalette[i], hex)
			}
		}
	})

	t.Run("unknown_display_type_falls_back_to_identity", func(t *testing.T) {
		profile := models.GetCalibrationProfile(models.DisplayType("banana"))
		cfg := models.GetDisplayConfig(models.DisplayType("banana"))
		if !profile.Precomp.IsIdentity() {
			t.Errorf("expected identity precompensation, got %+v", profile.Precomp)
		}
		if len(profile.PanelPalette) != len(cfg.Colors) {
			t.Fatalf("panel palette has %d entries, driver palette %d", len(profile.PanelPalette), len(cfg.Colors))
		}
		for i, hex := range cfg.Colors {
			if profile.PanelPalette[i] != hex {
				t.Errorf("panel palette[%d] = %s, want driver color %s", i, profile.PanelPalette[i], hex)
			}
		}
	})

	t.Run("palette_length_mismatch_falls_back_to_identity", func(t *testing.T) {
		orig := models.CalibrationProfiles[models.DisplayWaveshare73E]
		models.CalibrationProfiles[models.DisplayWaveshare73E] = models.CalibrationProfile{
			PanelPalette: []string{"#191E21", "#E8E8E8"}, // wrong length (2 != 6)
			Precomp:      models.PrecompensationPreset{Gamma: 1, Saturation: 1.15, Contrast: 1},
		}
		defer func() { models.CalibrationProfiles[models.DisplayWaveshare73E] = orig }()

		cfg := models.GetDisplayConfig(models.DisplayWaveshare73E)
		profile := models.GetCalibrationProfile(models.DisplayWaveshare73E)
		if !profile.Precomp.IsIdentity() {
			t.Errorf("expected identity precompensation on mismatch, got %+v", profile.Precomp)
		}
		if len(profile.PanelPalette) != len(cfg.Colors) {
			t.Fatalf("panel palette has %d entries, driver palette %d", len(profile.PanelPalette), len(cfg.Colors))
		}
		for i, hex := range cfg.Colors {
			if profile.PanelPalette[i] != hex {
				t.Errorf("panel palette[%d] = %s, want driver color %s", i, profile.PanelPalette[i], hex)
			}
		}
	})
}

// BenchmarkQuantize measures the quantization phase on a synthetic 800×480
// gradient for the 6-color profile (AC8 budget: fs_default and
// atkinson_default each within ~2x fs_off).
func BenchmarkQuantize(b *testing.B) {
	src := newGradientImage(800, 480)
	cfg := models.GetDisplayConfig(models.DisplayWaveshare73E)

	benches := []struct {
		name string
		algo models.DitherAlgorithm
		mode models.CalibrationMode
	}{
		{"fs_off", models.DitherFloydSteinberg, models.CalibrationOff},
		{"fs_default", models.DitherFloydSteinberg, models.CalibrationDefault},
		{"atkinson_default", models.DitherAtkinson, models.CalibrationDefault},
	}
	for _, bc := range benches {
		b.Run(bc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				out := quantizeForDisplay(src, cfg, bc.algo, bc.mode)
				if len(out.Pix) == 0 {
					b.Fatal(fmt.Errorf("empty output"))
				}
			}
		})
	}
}
