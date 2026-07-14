package services

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"e-ink-picture/server/internal/models"

	"golang.org/x/image/font/gofont/goregular"
)

// updateGolden rewrites the golden reference PNGs instead of comparing against them:
//
//	go test ./internal/services -run TestGoldenRender -update
//
// Convention: update golden files ONLY deliberately, and commit the refreshed
// PNGs in the SAME commit as the renderer change that caused the difference.
// Never run -update just to silence a red test — inspect the new PNGs first.
var updateGolden = flag.Bool("update", false, "rewrite golden files")

// goldenDesigns are the deterministic test designs under testdata/designs/.
var goldenDesigns = []string{"basic", "gradient", "rotation"}

// goldenDisplays are the display profiles covered by the golden harness.
var goldenDisplays = []models.DisplayType{
	models.DisplayWaveshare75V2,
	models.DisplayWaveshare73E,
}

// setupGoldenServices builds a fully deterministic render environment:
//   - settings.json pins display_type and render_quality (note: the display
//     type constants are lowercase, e.g. "waveshare_7in5_v2"),
//   - the embedded Go font is written to <tmpDir>/fonts/testfont.ttf because
//     loadFontFace tries system font paths BEFORE the embedded goregular
//     fallback; every text element in the test designs pins
//     fontFamily=testfont.ttf so rendering is identical on any machine,
//   - a programmatically generated gradient photo is written to
//     <tmpDir>/uploaded_images/ for the gradient design.
func setupGoldenServices(t *testing.T, displayType models.DisplayType, quality models.RenderQuality) (*PreviewService, string) {
	t.Helper()
	tmpDir := t.TempDir()

	for _, sub := range []string{"designs", "uploaded_images", "fonts", "weather_styles"} {
		if err := os.MkdirAll(filepath.Join(tmpDir, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}

	settingsData := fmt.Sprintf(`{"display_type":%q,"refresh_interval":3600,"render_quality":%q}`, displayType, quality)
	if err := os.WriteFile(filepath.Join(tmpDir, "settings.json"), []byte(settingsData), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(tmpDir, "fonts", "testfont.ttf"), goregular.TTF, 0644); err != nil {
		t.Fatal(err)
	}

	writeGradientPhoto(t, filepath.Join(tmpDir, "uploaded_images", "gradient_photo.png"))

	return newGoldenPreviewService(tmpDir), tmpDir
}

// newGoldenPreviewService constructs a fresh service stack over an existing data dir.
func newGoldenPreviewService(tmpDir string) *PreviewService {
	designSvc := NewDesignService(tmpDir)
	imageSvc := NewImageService(tmpDir)
	weatherSvc := NewWeatherService("", "", tmpDir)
	settingsSvc := NewSettingsService(tmpDir, models.DisplayWaveshare75V2)
	return NewPreviewService(designSvc, weatherSvc, imageSvc, settingsSvc, tmpDir)
}

// writeGradientPhoto generates a deterministic photo-like RGB gradient across
// both axes (R=x*255/w, G=y*255/h, B=(x+y)*255/(w+h)). The intermediate tones
// force real Floyd-Steinberg dithering during palette quantization.
func writeGradientPhoto(t *testing.T, path string) {
	t.Helper()
	const w, h = 800, 480
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(x * 255 / w),
				G: uint8(y * 255 / h),
				B: uint8((x + y) * 255 / (w + h)),
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
}

// loadTestDesign reads a design JSON from testdata/designs/.
func loadTestDesign(t *testing.T, name string) *models.DesignV2 {
	t.Helper()
	path := filepath.Join("testdata", "designs", name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read test design %s: %v", path, err)
	}
	var design models.DesignV2
	if err := json.Unmarshal(data, &design); err != nil {
		t.Fatalf("unmarshal test design %s: %v", path, err)
	}
	return &design
}

// pixelDiffStats decodes both PNGs and returns a human-readable diff summary:
// differing pixel count, total, percentage and the first mismatch coordinates.
func pixelDiffStats(golden, got []byte) string {
	goldenImg, err := png.Decode(bytes.NewReader(golden))
	if err != nil {
		return fmt.Sprintf("golden PNG not decodable: %v", err)
	}
	gotImg, err := png.Decode(bytes.NewReader(got))
	if err != nil {
		return fmt.Sprintf("rendered PNG not decodable: %v", err)
	}

	gb, rb := goldenImg.Bounds(), gotImg.Bounds()
	if gb.Dx() != rb.Dx() || gb.Dy() != rb.Dy() {
		return fmt.Sprintf("dimensions differ: golden %dx%d, rendered %dx%d", gb.Dx(), gb.Dy(), rb.Dx(), rb.Dy())
	}

	total := gb.Dx() * gb.Dy()
	diff := 0
	var firstDiffs []string
	for y := 0; y < gb.Dy(); y++ {
		for x := 0; x < gb.Dx(); x++ {
			wr, wg, wb, wa := goldenImg.At(gb.Min.X+x, gb.Min.Y+y).RGBA()
			gr, gg, gbl, ga := gotImg.At(rb.Min.X+x, rb.Min.Y+y).RGBA()
			if wr != gr || wg != gg || wb != gbl || wa != ga {
				diff++
				if len(firstDiffs) < 10 {
					firstDiffs = append(firstDiffs, fmt.Sprintf("(%d,%d)", x, y))
				}
			}
		}
	}
	return fmt.Sprintf("%d of %d pixels differ (%.2f%%), first mismatches: %s",
		diff, total, float64(diff)*100/float64(total), strings.Join(firstDiffs, " "))
}

// TestGoldenRender renders every test design for both display profiles at
// render_quality=high through the public API and compares byte-for-byte
// against the checked-in golden PNGs. Run with -update to regenerate them.
func TestGoldenRender(t *testing.T) {
	for _, designName := range goldenDesigns {
		for _, displayType := range goldenDisplays {
			name := fmt.Sprintf("%s__%s", designName, displayType)
			t.Run(name, func(t *testing.T) {
				previewSvc, _ := setupGoldenServices(t, displayType, models.RenderQualityHigh)
				design := loadTestDesign(t, designName)

				got, err := previewSvc.Render(design, false)
				if err != nil {
					t.Fatalf("Render failed: %v", err)
				}

				goldenPath := filepath.Join("testdata", "golden", name+".png")
				if *updateGolden {
					if err := os.MkdirAll(filepath.Dir(goldenPath), 0755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(goldenPath, got, 0644); err != nil {
						t.Fatal(err)
					}
					t.Logf("golden file written: %s (%d bytes)", goldenPath, len(got))
					return
				}

				want, err := os.ReadFile(goldenPath)
				if err != nil {
					t.Fatalf("read golden file %s: %v (run `go test ./internal/services -run TestGoldenRender -update` to regenerate)", goldenPath, err)
				}
				if !bytes.Equal(got, want) {
					t.Fatalf("rendered PNG differs from golden %s:\n%s", goldenPath, pixelDiffStats(want, got))
				}
			})
		}
	}
}

// TestPaletteExactness proves palette fidelity of the quantized output: the
// gradient design (which forces real dithering) and the rotation design
// (whose anti-aliased rotated edges must be dithered BEFORE quantization)
// are rendered for all render quality levels and both display profiles, and
// every unique RGB value in the output must be a member of the display
// palette — no intermediate tones.
func TestPaletteExactness(t *testing.T) {
	hexToRGB := func(hex string) color.RGBA {
		c := parseHexColor(hex)
		c.A = 255
		return c
	}
	allowedColors := map[models.DisplayType]map[color.RGBA]bool{
		models.DisplayWaveshare75V2: {
			hexToRGB("#000000"): true,
			hexToRGB("#FFFFFF"): true,
		},
		models.DisplayWaveshare73E: {
			hexToRGB("#000000"): true,
			hexToRGB("#FFFFFF"): true,
			hexToRGB("#FF0000"): true,
			hexToRGB("#FFFF00"): true,
			hexToRGB("#00FF00"): true,
			hexToRGB("#0000FF"): true,
		},
	}
	qualities := []models.RenderQuality{
		models.RenderQualityFast,
		models.RenderQualityMedium,
		models.RenderQualityHigh,
	}

	ditherDesigns := []string{"gradient", "rotation"}

	for _, designName := range ditherDesigns {
		for _, displayType := range goldenDisplays {
			for _, quality := range qualities {
				t.Run(fmt.Sprintf("%s__%s__%s", designName, displayType, quality), func(t *testing.T) {
					previewSvc, _ := setupGoldenServices(t, displayType, quality)
					design := loadTestDesign(t, designName)

					pngData, err := previewSvc.Render(design, false)
					if err != nil {
						t.Fatalf("Render failed: %v", err)
					}

					img, err := png.Decode(bytes.NewReader(pngData))
					if err != nil {
						t.Fatalf("decode PNG: %v", err)
					}
					if _, ok := img.(*image.Paletted); !ok {
						t.Errorf("expected *image.Paletted output, got %T", img)
					}

					allowed := allowedColors[displayType]
					seen := make(map[color.RGBA]bool)
					foreign := make(map[color.RGBA]int)
					bounds := img.Bounds()
					for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
						for x := bounds.Min.X; x < bounds.Max.X; x++ {
							r, g, b, _ := img.At(x, y).RGBA()
							c := color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 255}
							if allowed[c] {
								seen[c] = true
							} else {
								foreign[c]++
							}
						}
					}

					if len(foreign) > 0 {
						var lines []string
						for c, count := range foreign {
							lines = append(lines, fmt.Sprintf("#%02X%02X%02X: %d pixels", c.R, c.G, c.B, count))
						}
						sort.Strings(lines)
						t.Errorf("found %d RGB values outside the %s palette:\n%s",
							len(foreign), displayType, strings.Join(lines, "\n"))
					}
					if len(seen) < 2 {
						t.Errorf("only %d palette colors used — %s content did not exercise dithering", len(seen), designName)
					}
				})
			}
		}
	}
}

// TestRenderDeterminism proves byte-identical output for (a) repeated renders
// on the same service instance (warm font cache) and (b) a render on a freshly
// constructed instance over the same data dir (cold font cache).
func TestRenderDeterminism(t *testing.T) {
	for _, designName := range goldenDesigns {
		for _, displayType := range goldenDisplays {
			t.Run(fmt.Sprintf("%s__%s", designName, displayType), func(t *testing.T) {
				previewSvc, tmpDir := setupGoldenServices(t, displayType, models.RenderQualityHigh)
				design := loadTestDesign(t, designName)

				first, err := previewSvc.Render(design, false)
				if err != nil {
					t.Fatalf("first render failed: %v", err)
				}
				second, err := previewSvc.Render(design, false)
				if err != nil {
					t.Fatalf("second render failed: %v", err)
				}
				if !bytes.Equal(first, second) {
					t.Error("repeated render on the same instance produced different bytes")
				}

				freshSvc := newGoldenPreviewService(tmpDir)
				third, err := freshSvc.Render(design, false)
				if err != nil {
					t.Fatalf("fresh-instance render failed: %v", err)
				}
				if !bytes.Equal(first, third) {
					t.Error("render on a freshly constructed instance produced different bytes")
				}
			})
		}
	}
}
