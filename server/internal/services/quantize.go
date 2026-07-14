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
//     palette. With calibration "default" this is the profile's perceptual
//     panel palette (preceded by the profile's precompensation pass); with
//     "off" it is the ideal driver palette — the byte-exact legacy path.
//  2. Index-preserving palette swap: the paletted image's palette is replaced
//     with the pure driver palette. Pixel indices stay untouched, so the
//     encoded PNG contains EXCLUSIVELY driver colors; the perceptual palette
//     never leaks into the output.
//
// The invariant PanelPalette[i] <-> DisplayConfig.Colors[i] makes the swap
// correct; GetCalibrationProfile guarantees equal palette lengths.
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

	bounds := img.Bounds()
	var paletted *image.Paletted
	switch algo {
	case models.DitherAtkinson:
		paletted = ditherAtkinson(img, ditherPal)
	default: // floyd_steinberg
		paletted = image.NewPaletted(bounds, ditherPal)
		draw.FloydSteinberg.Draw(paletted, bounds, img, image.Point{})
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
// math.Pow per pixel) followed by integer-arithmetic saturation to a copy of
// src. Callers must skip identity presets entirely (quantizeForDisplay does)
// so the byte-identity guarantees for uncalibrated profiles hold.
func precompensate(src image.Image, p models.PrecompensationPreset) *image.RGBA {
	bounds := src.Bounds()
	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, src, bounds.Min, draw.Src)

	// lut[v] = clamp(round((255*(v/255)^Gamma - 128)*Contrast + 128), 0, 255)
	var lut [256]uint8
	for v := 0; v < 256; v++ {
		f := 255 * math.Pow(float64(v)/255, p.Gamma)
		lut[v] = uint8(math.Min(255, math.Max(0, math.Round((f-128)*p.Contrast+128))))
	}
	sat1000 := int32(math.Round(p.Saturation * 1000))

	pix := dst.Pix
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
	return dst
}

// ditherAtkinson quantizes src against pal using Atkinson error diffusion.
// Definition (binding, see specs/E1.6-panel-calibration.md):
//   - scan row-major, left to right, top to bottom, no serpentine,
//   - value_c = clamp(src_c + acc_c, 0, 255),
//   - nearest match by squared RGB distance, ties resolved to the LOWEST
//     palette index,
//   - err_c = value_c - palette[idx]_c, diffused as err_c/8 (Go integer
//     division, truncation toward zero) to exactly six neighbors:
//     (x+1,y), (x+2,y), (x-1,y+1), (x,y+1), (x+1,y+1), (x,y+2);
//     out-of-bounds shares and the remaining 2/8 are dropped by design.
//
// The implementation is float-free and uses a rolling 3-row int32 error
// buffer (~29 KB at 800px width).
func ditherAtkinson(src image.Image, pal color.Palette) *image.Paletted {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	paletted := image.NewPaletted(bounds, pal)
	if w == 0 || h == 0 || len(pal) == 0 {
		return paletted
	}

	// Palette channels as 8-bit values in int32.
	pr := make([]int32, len(pal))
	pg := make([]int32, len(pal))
	pb := make([]int32, len(pal))
	for i, c := range pal {
		r, g, b, _ := c.RGBA()
		pr[i], pg[i], pb[i] = int32(r>>8), int32(g>>8), int32(b>>8)
	}

	// Rolling error buffer for rows y, y+1, y+2; 3 channels per pixel.
	row0 := make([]int32, w*3)
	row1 := make([]int32, w*3)
	row2 := make([]int32, w*3)

	rgbaSrc, _ := src.(*image.RGBA)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var sr, sg, sb int32
			if rgbaSrc != nil {
				o := rgbaSrc.PixOffset(bounds.Min.X+x, bounds.Min.Y+y)
				sr = int32(rgbaSrc.Pix[o])
				sg = int32(rgbaSrc.Pix[o+1])
				sb = int32(rgbaSrc.Pix[o+2])
			} else {
				r, g, b, _ := src.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
				sr, sg, sb = int32(r>>8), int32(g>>8), int32(b>>8)
			}

			vr := clampChannel(sr + row0[x*3])
			vg := clampChannel(sg + row0[x*3+1])
			vb := clampChannel(sb + row0[x*3+2])

			best := 0
			bestDist := int32(math.MaxInt32)
			for i := range pr {
				dr := vr - pr[i]
				dg := vg - pg[i]
				db := vb - pb[i]
				// Strict < keeps the lowest index on equal distances.
				if d := dr*dr + dg*dg + db*db; d < bestDist {
					best, bestDist = i, d
				}
			}
			paletted.Pix[y*paletted.Stride+x] = uint8(best)

			er := (vr - pr[best]) / 8
			eg := (vg - pg[best]) / 8
			eb := (vb - pb[best]) / 8
			if er == 0 && eg == 0 && eb == 0 {
				continue
			}
			if x+1 < w {
				row0[(x+1)*3] += er
				row0[(x+1)*3+1] += eg
				row0[(x+1)*3+2] += eb
			}
			if x+2 < w {
				row0[(x+2)*3] += er
				row0[(x+2)*3+1] += eg
				row0[(x+2)*3+2] += eb
			}
			if x > 0 {
				row1[(x-1)*3] += er
				row1[(x-1)*3+1] += eg
				row1[(x-1)*3+2] += eb
			}
			row1[x*3] += er
			row1[x*3+1] += eg
			row1[x*3+2] += eb
			if x+1 < w {
				row1[(x+1)*3] += er
				row1[(x+1)*3+1] += eg
				row1[(x+1)*3+2] += eb
			}
			row2[x*3] += er
			row2[x*3+1] += eg
			row2[x*3+2] += eb
		}
		row0, row1, row2 = row1, row2, row0
		clear(row2)
	}
	return paletted
}
