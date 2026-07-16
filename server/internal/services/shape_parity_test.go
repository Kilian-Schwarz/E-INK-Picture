package services

import (
	"bytes"
	"context"
	"flag"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"e-ink-picture/server/internal/models"

	"golang.org/x/image/vector"
)

// updateParity regenerates the committed canvas-reference PNG:
//
//	go test ./internal/services -run TestCanvasPanelParity -updateparity
//
// The reference (testdata/parity/rounding__canvas_ref.png) is an INDEPENDENT
// analytic rasterization of rounding.json's shapes: rounded-rect fills and
// centered stroke rings drawn with golang.org/x/image/vector's scanline
// area-coverage rasterizer (SVG/Fabric bezier-corner geometry, kappa arcs).
// It shares no code path with the panel renderer (integer inside-test +
// supersample + CatmullRom), so agreement within the AC6 thresholds is a real
// canvas-vs-panel geometry cross-check, not a tautology.
//
// L2 procedure to replace this with a true Fabric export (for the reviewer /
// hardware-validator, per specs/B2-rounding-parity.md L2): open the designer,
// load rounding.json, hide the grid, run in the console
//
//	CanvasManager.getCanvas().toDataURL({format:'png', multiplier:1})
//
// and save the 800x480 data-URL over rounding__canvas_ref.png. The panel-side
// diff test below is unchanged either way.
var updateParity = flag.Bool("updateparity", false, "regenerate the canvas-reference PNG")

// bezierKappa is the control-point factor for approximating a quarter
// circle/ellipse with a cubic bezier (the same approximation Fabric uses for
// rounded-rect corners).
const bezierKappa = 0.5522847498307936

// appendRoundedRect adds a closed rounded-rect subpath to r, wound clockwise
// (cw=true) or counter-clockwise (cw=false). Winding a hole opposite to its
// outer contour lets the nonzero rasterizer cut a ring. Radii are clamped
// per-axis to w/2, h/2 (Fabric's per-axis clamp).
func appendRoundedRect(r *vector.Rasterizer, x, y, w, h, rx, ry float64, cw bool) {
	if rx < 0 {
		rx = 0
	}
	if ry < 0 {
		ry = 0
	}
	if rx > w/2 {
		rx = w / 2
	}
	if ry > h/2 {
		ry = h / 2
	}
	kx, ky := rx*bezierKappa, ry*bezierKappa

	if cw {
		r.MoveTo(float32(x+rx), float32(y))
		r.LineTo(float32(x+w-rx), float32(y))
		r.CubeTo(float32(x+w-rx+kx), float32(y), float32(x+w), float32(y+ry-ky), float32(x+w), float32(y+ry))
		r.LineTo(float32(x+w), float32(y+h-ry))
		r.CubeTo(float32(x+w), float32(y+h-ry+ky), float32(x+w-rx+kx), float32(y+h), float32(x+w-rx), float32(y+h))
		r.LineTo(float32(x+rx), float32(y+h))
		r.CubeTo(float32(x+rx-kx), float32(y+h), float32(x), float32(y+h-ry+ky), float32(x), float32(y+h-ry))
		r.LineTo(float32(x), float32(y+ry))
		r.CubeTo(float32(x), float32(y+ry-ky), float32(x+rx-kx), float32(y), float32(x+rx), float32(y))
		r.ClosePath()
		return
	}
	// counter-clockwise (reverse traversal)
	r.MoveTo(float32(x+rx), float32(y))
	r.CubeTo(float32(x+rx-kx), float32(y), float32(x), float32(y+ry-ky), float32(x), float32(y+ry))
	r.LineTo(float32(x), float32(y+h-ry))
	r.CubeTo(float32(x), float32(y+h-ry+ky), float32(x+rx-kx), float32(y+h), float32(x+rx), float32(y+h))
	r.LineTo(float32(x+w-rx), float32(y+h))
	r.CubeTo(float32(x+w-rx+kx), float32(y+h), float32(x+w), float32(y+h-ry+ky), float32(x+w), float32(y+h-ry))
	r.LineTo(float32(x+w), float32(y+ry))
	r.CubeTo(float32(x+w), float32(y+ry-ky), float32(x+w-rx+kx), float32(y), float32(x+w-rx), float32(y))
	r.LineTo(float32(x+rx), float32(y))
	r.ClosePath()
}

// rasterFill draws a single anti-aliased rounded-rect fill onto dst.
func rasterFill(dst *image.RGBA, x, y, w, h, rx, ry float64, c color.RGBA) {
	b := dst.Bounds()
	r := vector.NewRasterizer(b.Dx(), b.Dy())
	appendRoundedRect(r, x, y, w, h, rx, ry, true)
	r.Draw(dst, b, image.NewUniform(c), image.Point{})
}

// rasterStroke draws a centered stroke ring (outer minus inner rounded rect)
// onto dst using nonzero winding: the outer contour is clockwise, the inner
// hole counter-clockwise.
func rasterStroke(dst *image.RGBA, x, y, w, h, rx, ry, sw float64, c color.RGBA) {
	b := dst.Bounds()
	r := vector.NewRasterizer(b.Dx(), b.Dy())
	half := sw / 2
	appendRoundedRect(r, x-half, y-half, w+sw, h+sw, rx+half, ry+half, true)
	irx, iry := rx-half, ry-half
	if irx < 0 {
		irx = 0
	}
	if iry < 0 {
		iry = 0
	}
	appendRoundedRect(r, x+half, y+half, w-sw, h-sw, irx, iry, false)
	r.Draw(dst, b, image.NewUniform(c), image.Point{})
}

// generateCanvasReference rasterizes the shape elements of a design into an
// 800x480 white canvas with the independent vector rasterizer. Text elements
// are intentionally skipped: font rendering parity is a separate task (B2
// Non-Goal) and the diff test masks the caption band accordingly.
func generateCanvasReference(design *models.DesignV2) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, 800, 480))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)

	for _, elem := range design.Elements {
		if elem.Type != "shape" {
			continue
		}
		props := elem.Properties
		x, y, w, h := elem.X, elem.Y, elem.Width, elem.Height

		fillStr := GetPropString(props, "fill", "")
		rx := float64(GetPropInt(props, "rx", 0))
		ry := float64(GetPropInt(props, "ry", GetPropInt(props, "rx", 0)))
		if fillStr != "" && fillStr != "transparent" {
			rasterFill(dst, x, y, w, h, rx, ry, parseHexColor(fillStr))
		}

		strokeStr := GetPropString(props, "stroke", "")
		sw := float64(GetPropInt(props, "strokeWidth", 0))
		if strokeStr != "" && strokeStr != "transparent" && sw > 0 {
			rasterStroke(dst, x, y, w, h, rx, ry, sw, parseHexColor(strokeStr))
		}
	}
	return dst
}

// captionMask reports whether (x, y) lies in the caption text band of
// rounding.json (x∈[20,780], y∈[356,424]); those pixels are excluded from the
// parity diff because text parity is out of B2 scope.
func captionMask(x, y int) bool {
	return y >= 356 && y <= 424 && x >= 20 && x <= 780
}

// TestCanvasPanelParity (AC6) compares the panel's RAW high-quality render of
// rounding.json against the independent canvas reference. Guardrails:
//   - < 3.0 % of the compared pixels differ by > 24/255 on any channel,
//   - every shape corner (40x40 window) differs in < 20 % of its pixels.
//
// The visual review (panel render vs reference side by side) is the real gate;
// these thresholds are regression guardrails.
func TestCanvasPanelParity(t *testing.T) {
	refPath := filepath.Join("testdata", "parity", "rounding__canvas_ref.png")
	design := loadTestDesign(t, "rounding")

	if *updateParity {
		ref := generateCanvasReference(design)
		var buf bytes.Buffer
		if err := png.Encode(&buf, ref); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(refPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(refPath, buf.Bytes(), 0644); err != nil {
			t.Fatal(err)
		}
		t.Logf("canvas reference written: %s (%d bytes)", refPath, buf.Len())
		return
	}

	refBytes, err := os.ReadFile(refPath)
	if err != nil {
		t.Fatalf("read canvas reference %s: %v (run `go test ./internal/services -run TestCanvasPanelParity -updateparity`)", refPath, err)
	}
	refImg, err := png.Decode(bytes.NewReader(refBytes))
	if err != nil {
		t.Fatal(err)
	}

	svc, _ := setupGoldenServices(t, models.DisplayWaveshare73E, models.RenderQualityHigh)
	panelBytes, err := svc.Render(context.Background(), design, true) // raw, no quantization
	if err != nil {
		t.Fatalf("panel raw render: %v", err)
	}
	panelImg, err := png.Decode(bytes.NewReader(panelBytes))
	if err != nil {
		t.Fatal(err)
	}

	const chanThresh = 24 // > 24/255 per-channel counts as a differing pixel
	b := refImg.Bounds()
	compared, diff := 0, 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if captionMask(x, y) {
				continue
			}
			compared++
			rr, rg, rb, _ := refImg.At(x, y).RGBA()
			pr, pg, pb, _ := panelImg.At(x, y).RGBA()
			if absDiff8(rr, pr) > chanThresh || absDiff8(rg, pg) > chanThresh || absDiff8(rb, pb) > chanThresh {
				diff++
			}
		}
	}
	pct := float64(diff) * 100 / float64(compared)
	t.Logf("canvas↔panel: %d / %d compared pixels differ (%.2f%%), threshold 3.00%%", diff, compared, pct)
	if pct >= 3.0 {
		t.Errorf("canvas↔panel diff %.2f%% exceeds 3.0%% — geometry drift between designer and panel", pct)
	}

	// Per-corner windows: the corners must line up tightly.
	corners := shapeCornerWindows(design)
	for _, cw := range corners {
		cdiff, ctotal := 0, 0
		for y := cw.y; y < cw.y+40; y++ {
			for x := cw.x; x < cw.x+40; x++ {
				if x < 0 || y < 0 || x >= 800 || y >= 480 || captionMask(x, y) {
					continue
				}
				ctotal++
				rr, rg, rb, _ := refImg.At(x, y).RGBA()
				pr, pg, pb, _ := panelImg.At(x, y).RGBA()
				if absDiff8(rr, pr) > chanThresh || absDiff8(rg, pg) > chanThresh || absDiff8(rb, pb) > chanThresh {
					cdiff++
				}
			}
		}
		if ctotal == 0 {
			continue
		}
		cpct := float64(cdiff) * 100 / float64(ctotal)
		if cpct >= 20.0 {
			t.Errorf("corner window %s at (%d,%d): %.1f%% differ (>= 20%%) — corner misaligned", cw.id, cw.x, cw.y, cpct)
		}
	}
}

type cornerWindow struct {
	id   string
	x, y int
}

// shapeCornerWindows returns a 40x40 window anchored at each corner of every
// shape element (clamped near the box corner).
func shapeCornerWindows(design *models.DesignV2) []cornerWindow {
	var out []cornerWindow
	for _, e := range design.Elements {
		if e.Type != "shape" {
			continue
		}
		x, y, w, h := int(e.X), int(e.Y), int(e.Width), int(e.Height)
		out = append(out,
			cornerWindow{e.ID + "_tl", x - 4, y - 4},
			cornerWindow{e.ID + "_tr", x + w - 36, y - 4},
			cornerWindow{e.ID + "_bl", x - 4, y + h - 36},
			cornerWindow{e.ID + "_br", x + w - 36, y + h - 36},
		)
	}
	return out
}

// absDiff8 returns the absolute difference of two 16-bit color samples,
// expressed on the 0–255 scale.
func absDiff8(a, b uint32) int {
	d := int(a>>8) - int(b>>8)
	if d < 0 {
		d = -d
	}
	return d
}
