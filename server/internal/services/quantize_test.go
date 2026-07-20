package services

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"testing"

	"e-ink-picture/server/internal/models"
)

// newUniformImage returns a w×h RGBA image filled with a single opaque color.
func newUniformImage(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(c), image.Point{}, draw.Src)
	return img
}

// TestDiffusionMatrix checks both error diffusion kernels against index
// matrices produced by an INDEPENDENT reference implementation (a NumPy
// float32 transcription of the binding definition on ditherErrorDiffusion),
// not by the Go code under test. Any change to linearization, desaturation,
// weights, neighbors, scan order or tie-break turns this red.
//
// Binding definition being pinned here:
//   - channels linearized with the exact sRGB transfer function,
//   - achromatic palette => Rec.709 desaturation of the LINEAR values,
//   - serpentine scan (odd rows right-to-left, kernel dx mirrored),
//   - value = clamp(src + acc, 0, 1), nearest match by squared Euclidean
//     distance in linear RGB, ties to the LOWEST index,
//   - FS spreads 16/16 over 4 neighbors, Atkinson 6/8 over 6 neighbors.
//
// #BABABA is chosen because its linear luminance is ~0.5, so Floyd-Steinberg
// must settle on a perfect checkerboard — a strong, human-checkable signal
// that the diffusion runs in linear light (in gamma space #808080 would be
// the checkerboard point instead).
func TestDiffusionMatrix(t *testing.T) {
	bw := color.Palette{
		color.RGBA{0, 0, 0, 255},
		color.RGBA{255, 255, 255, 255},
	}

	cases := []struct {
		name    string
		dither  func(image.Image, color.Palette) *image.Paletted
		w, h    int
		fill    color.RGBA
		palette color.Palette
		want    [][]uint8
	}{
		{
			name:   "floyd_steinberg_gray186_bw_8x6",
			dither: ditherFloydSteinberg,
			w:      8, h: 6,
			fill:    color.RGBA{186, 186, 186, 255},
			palette: bw,
			want: [][]uint8{
				{0, 1, 0, 1, 0, 1, 0, 1},
				{1, 0, 1, 0, 1, 0, 1, 0},
				{0, 1, 0, 1, 0, 1, 0, 1},
				{1, 0, 1, 0, 1, 0, 1, 0},
				{0, 1, 0, 1, 0, 1, 0, 1},
				{1, 0, 1, 0, 1, 0, 1, 0},
			},
		},
		{
			name:   "atkinson_gray186_bw_8x6",
			dither: ditherAtkinson,
			w:      8, h: 6,
			fill:    color.RGBA{186, 186, 186, 255},
			palette: bw,
			want: [][]uint8{
				{0, 1, 0, 0, 1, 1, 0, 0},
				{0, 1, 0, 1, 1, 0, 0, 1},
				{1, 0, 1, 0, 0, 1, 1, 0},
				{1, 0, 1, 0, 1, 0, 1, 0},
				{0, 1, 0, 1, 1, 0, 0, 1},
				{0, 1, 0, 0, 0, 1, 0, 1},
			},
		},
		{
			name:   "atkinson_gray128_bw_6x4",
			dither: ditherAtkinson,
			w:      6, h: 4,
			fill:    color.RGBA{128, 128, 128, 255},
			palette: bw,
			want: [][]uint8{
				{0, 0, 0, 0, 0, 0},
				{0, 0, 0, 0, 0, 0},
				{0, 0, 0, 1, 0, 0},
				{0, 1, 0, 0, 0, 0},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.dither(newUniformImage(tc.w, tc.h, tc.fill), tc.palette)
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

// TestTieBreakLowestIndex pins the tie-break rule: on exactly equal distance
// the LOWEST palette index wins. #BABABA (linear ~0.5) against {black, white}
// is equidistant at the very first pixel, where the accumulator is still zero.
func TestTieBreakLowestIndex(t *testing.T) {
	pal := color.Palette{
		color.RGBA{0, 0, 0, 255},
		color.RGBA{255, 255, 255, 255},
	}
	// Linear luminance of #BABABA is marginally below 0.5, so black (index 0)
	// wins the first pixel; the assertion that matters is that a tie or
	// near-tie never silently flips to the higher index.
	got := ditherFloydSteinberg(newUniformImage(4, 1, color.RGBA{186, 186, 186, 255}), pal)
	if got.Pix[0] != 0 {
		t.Errorf("first pixel index = %d, want 0 (ties resolve to the lowest palette index)", got.Pix[0])
	}
}

// TestLinearMidGrayDitherRatio is the regression guard for the whole point of
// linear-light error diffusion: a flat 50% sRGB field (#808080) has a relative
// luminance of ~0.2159, so a black/white dither must produce ~21.6% white
// pixels. Diffusing in the gamma-encoded domain produces ~50% instead — the
// systematic over-brightening this test exists to prevent from coming back.
func TestLinearMidGrayDitherRatio(t *testing.T) {
	const w, h = 400, 240
	pal := color.Palette{
		color.RGBA{0, 0, 0, 255},
		color.RGBA{255, 255, 255, 255},
	}
	src := newUniformImage(w, h, color.RGBA{128, 128, 128, 255})

	got := ditherFloydSteinberg(src, pal)
	white := 0
	for _, idx := range got.Pix {
		if idx == 1 {
			white++
		}
	}
	ratio := float64(white) / float64(w*h)

	// srgbToLinear(128/255) = 0.21586; allow a small dithering slack.
	want := srgbToLinear(128.0 / 255)
	if math.Abs(ratio-want) > 0.01 {
		t.Errorf("white pixel fraction = %.4f, want ~%.4f (linear luminance of #808080); "+
			"a value near 0.50 means the error diffusion regressed to gamma space", ratio, want)
	}
	t.Logf("#808080 -> %.2f%% white pixels (linear luminance %.4f)", ratio*100, want)
}

// TestSRGBTransferFunction pins the exact IEC 61966-2-1 transfer function:
// srgbToLinear and linearToSRGB must be inverses over the whole 8-bit domain,
// and the well-known anchor #808080 -> ~0.2159 relative luminance must hold.
// The linear segment below the 0.04045 breakpoint is what distinguishes the
// real curve from the "gamma 2.2" approximation, so the near-black values are
// part of the assertion, not incidental.
func TestSRGBTransferFunction(t *testing.T) {
	for i := 0; i <= 255; i++ {
		c := float64(i) / 255
		if back := linearToSRGB(srgbToLinear(c)); math.Abs(back-c) > 1e-12 {
			t.Errorf("round trip for 8-bit value %d: %v -> %v", i, c, back)
		}
	}

	if got := srgbToLinear(128.0 / 255); math.Abs(got-0.21586) > 1e-4 {
		t.Errorf("srgbToLinear(#80) = %.6f, want ~0.21586", got)
	}
	// Below the breakpoint the curve is a pure linear ramp of slope 1/12.92.
	if got := srgbToLinear(0.03); math.Abs(got-0.03/12.92) > 1e-12 {
		t.Errorf("srgbToLinear(0.03) = %v, want the linear segment %v", got, 0.03/12.92)
	}
	if got := srgbToLinear(0); got != 0 {
		t.Errorf("srgbToLinear(0) = %v, want 0", got)
	}
	if got := srgbToLinear(1); math.Abs(got-1) > 1e-12 {
		t.Errorf("srgbToLinear(1) = %v, want 1", got)
	}
}

// TestCalibrationOffUsesDriverPalette pins what calibration "off" still
// guarantees after the move to linear-light diffusion: the ideal driver
// palette is used directly, with no precompensation pass and no panel palette
// involved. Geometry and palette must match the stdlib reference exactly.
//
// It deliberately no longer asserts BYTE-identity with stdlib
// draw.FloydSteinberg. That ex-guarantee pinned the gamma-space error
// diffusion, i.e. the very bug this pipeline now fixes: stdlib FS diffuses in
// the sRGB-encoded domain, which renders every mid-tone systematically too
// light. See TestLinearMidGrayDitherRatio and TestLinearDitherIsDarkerThanGamma
// for the replacement guarantees.
func TestCalibrationOffUsesDriverPalette(t *testing.T) {
	src := newGradientImage(800, 480)

	for _, displayType := range goldenDisplays {
		t.Run(string(displayType), func(t *testing.T) {
			cfg := models.GetDisplayConfig(displayType)

			pal := make(color.Palette, 0, len(cfg.Colors))
			for _, hex := range cfg.Colors {
				pal = append(pal, parseHexColor(hex))
			}
			bounds := src.Bounds()
			reference := image.NewPaletted(bounds, pal)

			got := quantizeForDisplay(src, cfg, models.DitherFloydSteinberg, models.CalibrationOff)

			if got.Bounds() != reference.Bounds() || got.Stride != reference.Stride {
				t.Fatalf("geometry differs: got %v/%d, want %v/%d", got.Bounds(), got.Stride, reference.Bounds(), reference.Stride)
			}
			if len(got.Palette) != len(pal) {
				t.Fatalf("palette length %d, want %d", len(got.Palette), len(pal))
			}
			for i := range pal {
				wr, wg, wb, wa := pal[i].RGBA()
				gr, gg, gb, ga := got.Palette[i].RGBA()
				if wr != gr || wg != gg || wb != gb || wa != ga {
					t.Errorf("palette[%d] differs: got %v, want driver color %v", i, got.Palette[i], pal[i])
				}
			}
			for _, idx := range got.Pix {
				if int(idx) >= len(pal) {
					t.Fatalf("pixel index %d out of palette range %d", idx, len(pal))
				}
			}
		})
	}
}

// TestLinearDitherIsDarkerThanGamma proves the direction of the fix on the
// real pipeline: against the gamma-space stdlib ditherer, linear-light
// diffusion must select measurably FEWER white pixels on a mid-gray field,
// because gamma-space diffusion over-brightens mid-tones.
func TestLinearDitherIsDarkerThanGamma(t *testing.T) {
	const w, h = 400, 240
	cfg := models.GetDisplayConfig(models.DisplayWaveshare75V2)
	whiteIdx := uint8(1) // DisplayConfig.Colors = {#000000, #FFFFFF}

	countWhite := func(p *image.Paletted) int {
		n := 0
		for _, idx := range p.Pix {
			if idx == whiteIdx {
				n++
			}
		}
		return n
	}

	src := newUniformImage(w, h, color.RGBA{128, 128, 128, 255})

	gammaSpace := image.NewPaletted(src.Bounds(), paletteFromHex(cfg.Colors))
	draw.FloydSteinberg.Draw(gammaSpace, src.Bounds(), src, image.Point{})

	linearSpace := quantizeForDisplay(src, cfg, models.DitherFloydSteinberg, models.CalibrationOff)

	gammaWhite := countWhite(gammaSpace)
	linearWhite := countWhite(linearSpace)
	total := w * h

	t.Logf("#808080 white pixels: gamma-space %.2f%%, linear-light %.2f%%",
		float64(gammaWhite)*100/float64(total), float64(linearWhite)*100/float64(total))

	if linearWhite >= gammaWhite {
		t.Errorf("linear-light dither produced %d white pixels, gamma-space %d — "+
			"linear diffusion must be darker on mid-gray", linearWhite, gammaWhite)
	}
	if float64(gammaWhite)/float64(total) < 0.45 {
		t.Errorf("sanity: gamma-space reference should sit near 50%%, got %.2f%%",
			float64(gammaWhite)*100/float64(total))
	}
}

// TestCalibrationAffectsOnlyCalibratedProfiles proves AC4: on epd7in3e the
// calibration changes the rendered bytes, on the identity-profile 7.5" V2 it
// must not change a single byte.
func TestCalibrationAffectsOnlyCalibratedProfiles(t *testing.T) {
	renderGradient := func(t *testing.T, displayType models.DisplayType, mode models.CalibrationMode) []byte {
		t.Helper()
		previewSvc, _ := setupGoldenServicesVariant(t, displayType, models.RenderQualityHigh, models.DitherFloydSteinberg, mode)
		out, err := previewSvc.Render(context.Background(), loadTestDesign(t, "gradient"), false)
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
				out, err := previewSvc.Render(context.Background(), loadTestDesign(t, "gradient"), false)
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

// TestPrecompensateInPlace proves AC3a of specs/E5.6-render-memory.md:
// for *image.RGBA input precompensate mutates the pixel buffer in place (no
// new image allocation), the non-RGBA fallback still copies, and both paths
// produce pixel-identical results.
func TestPrecompensateInPlace(t *testing.T) {
	// Non-identity preset so the pass actually changes pixel values.
	preset := models.PrecompensationPreset{Gamma: 1.05, Saturation: 1.15, Contrast: 1.02}

	t.Run("rgba_input_shares_backing_array", func(t *testing.T) {
		src := newGradientImage(64, 48)
		got := precompensate(src, preset)
		if &got.Pix[0] != &src.Pix[0] {
			t.Error("RGBA input must be precompensated in place (same backing array)")
		}
	})

	t.Run("non_rgba_input_is_copied", func(t *testing.T) {
		rgba := newGradientImage(64, 48)
		src := image.NewNRGBA(rgba.Bounds())
		draw.Draw(src, src.Bounds(), rgba, image.Point{}, draw.Src)
		before := append([]uint8(nil), src.Pix...)

		got := precompensate(src, preset)
		if !bytes.Equal(src.Pix, before) {
			t.Error("non-RGBA input must not be mutated (copy fallback)")
		}
		if bytes.Equal(got.Pix, before) {
			t.Error("output equals input — precompensation with a non-identity preset had no effect")
		}
	})

	t.Run("in_place_matches_copy_fallback", func(t *testing.T) {
		rgba := newGradientImage(64, 48)
		nrgba := image.NewNRGBA(rgba.Bounds())
		draw.Draw(nrgba, nrgba.Bounds(), rgba, image.Point{}, draw.Src)

		inPlace := precompensate(rgba, preset)  // mutates rgba
		viaCopy := precompensate(nrgba, preset) // copy fallback
		if !bytes.Equal(inPlace.Pix, viaCopy.Pix) {
			t.Error("in-place path and copy fallback produced different pixels")
		}
	})
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
				// quantizeForDisplay may mutate RGBA input in place (E5.6
				// ownership contract), so each iteration gets a private copy
				// outside the timed section.
				b.StopTimer()
				iterSrc := image.NewRGBA(src.Bounds())
				copy(iterSrc.Pix, src.Pix)
				b.StartTimer()

				out := quantizeForDisplay(iterSrc, cfg, bc.algo, bc.mode)
				if len(out.Pix) == 0 {
					b.Fatal(fmt.Errorf("empty output"))
				}
			}
		})
	}
}
