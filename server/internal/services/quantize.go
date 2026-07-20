package services

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"e-ink-picture/server/internal/models"
)

// paletteFromHex converts hex color strings into a color.Palette, falling
// back to white/black for an empty input (legacy quantizeToPalette behavior).
func paletteFromHex(hexColors []string) color.Palette {
	pal := make(color.Palette, 0, len(hexColors))
	for _, hex := range hexColors {
		pal = append(pal, parseHexColor(hex))
	}
	if len(pal) == 0 {
		pal = color.Palette{color.White, color.Black}
	}
	return pal
}

// quantizeForDisplay reduces img to the display's driver palette in two stages:
//
//  1. Error diffusion (Floyd-Steinberg or Atkinson) against the DITHER
//     palette, performed in LINEAR LIGHT (see ditherErrorDiffusion). With
//     calibration "default" this is the profile's perceptual panel palette
//     (preceded by the profile's precompensation pass); with "off" it is the
//     ideal driver palette.
//  2. Index-preserving palette swap: the paletted image's palette is replaced
//     with the pure driver palette. Pixel indices stay untouched, so the
//     encoded PNG contains EXCLUSIVELY driver colors; the perceptual palette
//     never leaks into the output.
//
// The invariant PanelPalette[i] <-> DisplayConfig.Colors[i] makes the swap
// correct; GetCalibrationProfile guarantees equal palette lengths.
//
// Ownership contract: quantizeForDisplay MAY MUTATE img in place when it is
// an *image.RGBA (the precompensation pass writes into img's pixel buffer to
// avoid a full-canvas copy). Callers must pass a render-private buffer —
// both current call paths do (Render's downscale destination, or the private
// supersample canvas itself at render_quality=fast).
func quantizeForDisplay(img image.Image, cfg models.DisplayConfig, algo models.DitherAlgorithm, mode models.CalibrationMode) *image.Paletted {
	driverPal := paletteFromHex(cfg.Colors)

	ditherPal := driverPal
	if mode != models.CalibrationOff {
		profile := models.GetCalibrationProfile(cfg.Type)
		if !profile.Precomp.IsIdentity() {
			img = precompensate(img, profile.Precomp)
		}
		if len(profile.PanelPalette) == len(driverPal) {
			ditherPal = paletteFromHex(profile.PanelPalette)
		}
	}

	var paletted *image.Paletted
	switch algo {
	case models.DitherAtkinson:
		paletted = ditherAtkinson(img, ditherPal)
	default: // floyd_steinberg
		paletted = ditherFloydSteinberg(img, ditherPal)
	}

	// Stage 2: index-preserving swap back to the pure driver colors.
	paletted.Palette = driverPal
	return paletted
}

// clampChannel clamps an 8-bit channel value held in an int32 to [0, 255].
func clampChannel(v int32) int32 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// precompensate applies gamma+contrast (via a per-render 256-entry LUT, no
// math.Pow per pixel) followed by integer-arithmetic saturation. For
// *image.RGBA input it mutates src IN PLACE (no full-canvas copy — see the
// ownership contract on quantizeForDisplay); other image types are copied
// into a fresh RGBA first. Callers must skip identity presets entirely
// (quantizeForDisplay does): a {1,1,1} preset is not exactly a no-op once it
// round-trips through the LUT, so skipping is what keeps calibration
// "default" byte-identical to "off" on identity profiles such as the 7.5" V2
// (asserted by TestCalibrationAffectsOnlyCalibratedProfiles).
//
// NOTE: this pass still operates on gamma-encoded sRGB values. That is
// deliberate — it is a calibration/appearance control, not part of the
// colorimetric quantization, which linearizes downstream in
// ditherErrorDiffusion.
//
// The former byte-identity guarantee against the pre-E1.6 stdlib
// Floyd-Steinberg output is INTENTIONALLY GONE: that path diffused error in
// the gamma-encoded domain and rendered mid-tones systematically too light.
func precompensate(src image.Image, p models.PrecompensationPreset) *image.RGBA {
	bounds := src.Bounds()
	dst, ok := src.(*image.RGBA)
	if !ok {
		dst = image.NewRGBA(bounds)
		draw.Draw(dst, bounds, src, bounds.Min, draw.Src)
	}

	// lut[v] = clamp(round((255*(v/255)^Gamma - 128)*Contrast + 128), 0, 255)
	var lut [256]uint8
	for v := 0; v < 256; v++ {
		f := 255 * math.Pow(float64(v)/255, p.Gamma)
		lut[v] = uint8(math.Min(255, math.Max(0, math.Round((f-128)*p.Contrast+128))))
	}
	sat1000 := int32(math.Round(p.Saturation * 1000))

	// Iterate row-wise within bounds so in-place RGBA sub-images (stride >
	// 4*width or non-zero origin) never touch pixels outside their bounds.
	w := bounds.Dx()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		rowStart := dst.PixOffset(bounds.Min.X, y)
		pix := dst.Pix[rowStart : rowStart+w*4 : rowStart+w*4]
		for i := 0; i < len(pix); i += 4 {
			r := int32(lut[pix[i]])
			g := int32(lut[pix[i+1]])
			b := int32(lut[pix[i+2]])
			if sat1000 != 1000 {
				luma := (299*r + 587*g + 114*b) / 1000
				r = clampChannel(luma + (r-luma)*sat1000/1000)
				g = clampChannel(luma + (g-luma)*sat1000/1000)
				b = clampChannel(luma + (b-luma)*sat1000/1000)
			}
			pix[i], pix[i+1], pix[i+2] = uint8(r), uint8(g), uint8(b)
		}
	}
	return dst
}

// sRGB transfer function constants (IEC 61966-2-1). The linear segment below
// the 0.04045 breakpoint is part of the standard — the widespread "gamma 2.2"
// approximation is NOT the sRGB curve and diverges most in the deep shadows.
const (
	srgbLinearCutoff  = 0.04045
	srgbInverseCutoff = 0.0031308
	srgbSlope         = 12.92
	srgbAlpha         = 0.055
	srgbExponent      = 2.4
)

// Rec.709 / sRGB luminance coefficients. They map LINEAR RGB to CIE Y; applied
// to gamma-encoded values they would yield luma (Y'), a different quantity.
const (
	lumaR = 0.2126
	lumaG = 0.7152
	lumaB = 0.0722
)

// srgbToLinear converts a gamma-encoded sRGB channel in [0,1] to linear light.
func srgbToLinear(c float64) float64 {
	if c <= srgbLinearCutoff {
		return c / srgbSlope
	}
	return math.Pow((c+srgbAlpha)/(1+srgbAlpha), srgbExponent)
}

// linearToSRGB is the exact inverse of srgbToLinear for inputs in [0,1].
func linearToSRGB(v float64) float64 {
	if v <= srgbInverseCutoff {
		return v * srgbSlope
	}
	return (1+srgbAlpha)*math.Pow(v, 1/srgbExponent) - srgbAlpha
}

// srgb8ToLinear is a lookup table for the 256 possible 8-bit sRGB channel
// values, so the pixel loop never calls math.Pow.
var srgb8ToLinear = func() [256]float32 {
	var lut [256]float32
	for i := range lut {
		lut[i] = float32(srgbToLinear(float64(i) / 255))
	}
	return lut
}()

// diffusionOffset is one neighbor share of an error diffusion kernel. dx is
// given for a left-to-right pass and is mirrored on serpentine return rows.
type diffusionOffset struct {
	dx, dy int
	weight float32
}

// diffusionKernel describes an error diffusion algorithm. rows is the number
// of error accumulator rows required (1 + max dy).
type diffusionKernel struct {
	offsets []diffusionOffset
	rows    int
}

// floydSteinbergKernel distributes the FULL error over 4 neighbors, which
// preserves mean luminance exactly.
var floydSteinbergKernel = diffusionKernel{
	rows: 2,
	offsets: []diffusionOffset{
		{1, 0, 7.0 / 16},
		{-1, 1, 3.0 / 16},
		{0, 1, 5.0 / 16},
		{1, 1, 1.0 / 16},
	},
}

// atkinsonKernel spreads only 6/8 of the error over 6 neighbors; the dropped
// 2/8 is what gives Atkinson its higher contrast and quieter flat areas.
var atkinsonKernel = diffusionKernel{
	rows: 3,
	offsets: []diffusionOffset{
		{1, 0, 1.0 / 8},
		{2, 0, 1.0 / 8},
		{-1, 1, 1.0 / 8},
		{0, 1, 1.0 / 8},
		{1, 1, 1.0 / 8},
		{0, 2, 1.0 / 8},
	},
}

// ditherFloydSteinberg quantizes src against pal using Floyd-Steinberg error
// diffusion in linear light with a serpentine scan.
func ditherFloydSteinberg(src image.Image, pal color.Palette) *image.Paletted {
	return ditherErrorDiffusion(src, pal, floydSteinbergKernel)
}

// ditherAtkinson quantizes src against pal using Atkinson error diffusion in
// linear light with a serpentine scan.
func ditherAtkinson(src image.Image, pal color.Palette) *image.Paletted {
	return ditherErrorDiffusion(src, pal, atkinsonKernel)
}

// isGrayscalePalette reports whether every palette entry is achromatic.
func isGrayscalePalette(pal color.Palette) bool {
	for _, c := range pal {
		r, g, b, _ := c.RGBA()
		if r != g || g != b {
			return false
		}
	}
	return len(pal) > 0
}

// ditherErrorDiffusion is the single quantizer for both dither algorithms.
// Binding definition (supersedes the 8-bit integer Atkinson of E1.6):
//
//   - Source and palette channels are linearized with the exact sRGB transfer
//     function before anything else. Dithering reconstructs a color by spatial
//     averaging, and that averaging happens physically in linear light — only
//     a linear-space error diffusion is energy preserving. Diffusing in the
//     gamma-encoded domain makes every mid-tone systematically too light
//     (sRGB #808080 would become ~50% white pixels instead of the correct
//     ~21.4%, its actual relative luminance).
//   - For an achromatic palette (epd7in5_V2) the source is desaturated with
//     the Rec.709 coefficients applied to the LINEARIZED channels, i.e. true
//     relative luminance Y, not luma.
//   - Scan is serpentine: even rows left-to-right, odd rows right-to-left with
//     mirrored kernel dx. This removes the directional error drift that shows
//     up as diagonal worms and edge streaks.
//   - value_c = clamp(src_c + acc_c, 0, 1) — clamping the value (not the
//     error) bounds the per-pixel error to [-1,1] and, since the kernel
//     weights sum to <= 1, keeps the accumulator bounded without a separate
//     error clamp.
//   - Nearest match by squared Euclidean distance in linear RGB; ties resolve
//     to the LOWEST palette index (strict <).
//
// The accumulator is float32 and rolls over kernel.rows rows (~29 KB at 800px
// width for Atkinson).
func ditherErrorDiffusion(src image.Image, pal color.Palette, kernel diffusionKernel) *image.Paletted {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	paletted := image.NewPaletted(bounds, pal)
	if w == 0 || h == 0 || len(pal) == 0 {
		return paletted
	}

	// Palette in linear light.
	pr := make([]float32, len(pal))
	pg := make([]float32, len(pal))
	pb := make([]float32, len(pal))
	for i, c := range pal {
		r, g, b, _ := c.RGBA()
		pr[i] = srgb8ToLinear[r>>8]
		pg[i] = srgb8ToLinear[g>>8]
		pb[i] = srgb8ToLinear[b>>8]
	}

	gray := isGrayscalePalette(pal)
	if gray {
		for i := range pr {
			y := lumaR*pr[i] + lumaG*pg[i] + lumaB*pb[i]
			pr[i], pg[i], pb[i] = y, y, y
		}
	}

	// Rolling error accumulator: kernel.rows rows, 3 channels per pixel.
	rows := make([][]float32, kernel.rows)
	for i := range rows {
		rows[i] = make([]float32, w*3)
	}

	rgbaSrc, _ := src.(*image.RGBA)

	for y := 0; y < h; y++ {
		reverse := y%2 == 1
		for i := 0; i < w; i++ {
			x := i
			if reverse {
				x = w - 1 - i
			}

			var sr, sg, sb float32
			if rgbaSrc != nil {
				o := rgbaSrc.PixOffset(bounds.Min.X+x, bounds.Min.Y+y)
				sr = srgb8ToLinear[rgbaSrc.Pix[o]]
				sg = srgb8ToLinear[rgbaSrc.Pix[o+1]]
				sb = srgb8ToLinear[rgbaSrc.Pix[o+2]]
			} else {
				r, g, b, _ := src.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
				sr = srgb8ToLinear[r>>8]
				sg = srgb8ToLinear[g>>8]
				sb = srgb8ToLinear[b>>8]
			}
			if gray {
				yl := lumaR*sr + lumaG*sg + lumaB*sb
				sr, sg, sb = yl, yl, yl
			}

			vr := clampUnit(sr + rows[0][x*3])
			vg := clampUnit(sg + rows[0][x*3+1])
			vb := clampUnit(sb + rows[0][x*3+2])

			best := 0
			bestDist := float32(math.MaxFloat32)
			for i := range pr {
				dr, dg, db := vr-pr[i], vg-pg[i], vb-pb[i]
				// Strict < keeps the lowest index on equal distances.
				if d := dr*dr + dg*dg + db*db; d < bestDist {
					best, bestDist = i, d
				}
			}
			paletted.Pix[y*paletted.Stride+x] = uint8(best)

			er := vr - pr[best]
			eg := vg - pg[best]
			eb := vb - pb[best]
			if er == 0 && eg == 0 && eb == 0 {
				continue
			}
			for _, off := range kernel.offsets {
				dx := off.dx
				if reverse {
					dx = -dx
				}
				nx := x + dx
				if nx < 0 || nx >= w {
					continue
				}
				row := rows[off.dy]
				row[nx*3] += er * off.weight
				row[nx*3+1] += eg * off.weight
				row[nx*3+2] += eb * off.weight
			}
		}

		// Rotate: row 0 is consumed, the trailing row is reused zeroed.
		first := rows[0]
		copy(rows, rows[1:])
		clear(first)
		rows[len(rows)-1] = first
	}
	return paletted
}

// clampUnit clamps a linear-light channel value to [0, 1].
func clampUnit(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
