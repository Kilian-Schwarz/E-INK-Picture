package services

import (
	"image"
	"image/color"
	"image/png"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"e-ink-picture/server/internal/models"
)

func setupTestServices(t *testing.T) (*PreviewService, string) {
	t.Helper()
	tmpDir := t.TempDir()

	// Create required subdirectories
	for _, sub := range []string{"designs", "uploaded_images", "fonts", "weather_styles"} {
		if err := os.MkdirAll(filepath.Join(tmpDir, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Write a default settings file
	settingsData := `{"display_type":"waveshare_7in5_V2","refresh_interval":3600}`
	os.WriteFile(filepath.Join(tmpDir, "settings.json"), []byte(settingsData), 0644)

	designSvc := NewDesignService(tmpDir)
	imageSvc := NewImageService(tmpDir)
	weatherSvc := NewWeatherService("", "", tmpDir)
	settingsSvc := NewSettingsService(tmpDir)
	previewSvc := NewPreviewService(designSvc, weatherSvc, imageSvc, settingsSvc, tmpDir)

	return previewSvc, tmpDir
}

func TestRenderTextWidget(t *testing.T) {
	previewSvc, _ := setupTestServices(t)

	vis := true
	design := &models.DesignV2{
		Name:    "test",
		Version: 2,
		Canvas:  models.CanvasConfig{Width: 800, Height: 480, Background: "#FFFFFF"},
		Elements: []models.Element{
			{
				ID:     "t1",
				Type:   "text",
				X:      100,
				Y:      50,
				Width:  200,
				Height: 60,
				ZIndex: 0,
				Visible: &vis,
				Properties: map[string]any{
					"text":      "Hello World",
					"fontSize":  24,
					"color":     "#000000",
					"textAlign": "left",
				},
			},
		},
	}

	pngData, err := previewSvc.Render(design, true)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if len(pngData) == 0 {
		t.Fatal("Render returned empty data")
	}

	// Decode and check that the text area has non-white pixels
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		t.Fatalf("Failed to decode PNG: %v", err)
	}

	hasNonWhite := false
	for y := 50; y < 110; y++ {
		for x := 100; x < 300; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r < 0xffff || g < 0xffff || b < 0xffff {
				hasNonWhite = true
				break
			}
		}
		if hasNonWhite {
			break
		}
	}

	if !hasNonWhite {
		t.Error("Text widget area is entirely white — text was not rendered")
	}
}

func TestRenderWidgetClock(t *testing.T) {
	previewSvc, _ := setupTestServices(t)

	vis := true
	design := &models.DesignV2{
		Name:    "test-clock",
		Version: 2,
		Canvas:  models.CanvasConfig{Width: 800, Height: 480, Background: "#FFFFFF"},
		Elements: []models.Element{
			{
				ID:     "c1",
				Type:   "widget_clock",
				X:      50,
				Y:      50,
				Width:  300,
				Height: 80,
				ZIndex: 0,
				Visible: &vis,
				Properties: map[string]any{
					"layout":   "digital_large",
					"fontSize": float64(48),
					"color":    "#000000",
				},
			},
		},
	}

	pngData, err := previewSvc.Render(design, true)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		t.Fatalf("Failed to decode PNG: %v", err)
	}

	// Check that the clock area has some rendered content
	hasNonWhite := false
	for y := 50; y < 130; y++ {
		for x := 50; x < 350; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r < 0xffff || g < 0xffff || b < 0xffff {
				hasNonWhite = true
				break
			}
		}
		if hasNonWhite {
			break
		}
	}

	if !hasNonWhite {
		t.Error("Clock widget area is entirely white — clock was not rendered")
	}
}

func TestRenderAlignmentCenter(t *testing.T) {
	previewSvc, _ := setupTestServices(t)

	vis := true
	design := &models.DesignV2{
		Name:    "test-align",
		Version: 2,
		Canvas:  models.CanvasConfig{Width: 800, Height: 480, Background: "#FFFFFF"},
		Elements: []models.Element{
			{
				ID:     "a1",
				Type:   "text",
				X:      0,
				Y:      0,
				Width:  400,
				Height: 60,
				ZIndex: 0,
				Visible: &vis,
				Properties: map[string]any{
					"text":      "X",
					"fontSize":  24,
					"color":     "#000000",
					"textAlign": "center",
				},
			},
		},
	}

	pngData, err := previewSvc.Render(design, true)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		t.Fatalf("Failed to decode PNG: %v", err)
	}

	// For center alignment, the text should be roughly in the middle of the 400px width
	// Find the leftmost non-white pixel
	leftMost := 400
	for y := 0; y < 60; y++ {
		for x := 0; x < 400; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r < 0xffff || g < 0xffff || b < 0xffff {
				if x < leftMost {
					leftMost = x
				}
			}
		}
	}

	// Center-aligned single char "X" with fontSize 24: expect leftmost pixel > 150
	if leftMost < 100 {
		t.Errorf("Center-aligned text starts too far left at x=%d, expected > 100", leftMost)
	}
}

func TestRenderAlignmentRight(t *testing.T) {
	previewSvc, _ := setupTestServices(t)

	vis := true
	design := &models.DesignV2{
		Name:    "test-align-right",
		Version: 2,
		Canvas:  models.CanvasConfig{Width: 800, Height: 480, Background: "#FFFFFF"},
		Elements: []models.Element{
			{
				ID:     "a2",
				Type:   "text",
				X:      0,
				Y:      0,
				Width:  400,
				Height: 60,
				ZIndex: 0,
				Visible: &vis,
				Properties: map[string]any{
					"text":      "X",
					"fontSize":  24,
					"color":     "#000000",
					"textAlign": "right",
				},
			},
		},
	}

	pngData, err := previewSvc.Render(design, true)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		t.Fatalf("Failed to decode PNG: %v", err)
	}

	// For right alignment, the text should be near x=400
	leftMost := 400
	for y := 0; y < 60; y++ {
		for x := 0; x < 400; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r < 0xffff || g < 0xffff || b < 0xffff {
				if x < leftMost {
					leftMost = x
				}
			}
		}
	}

	if leftMost < 300 {
		t.Errorf("Right-aligned text starts too far left at x=%d, expected > 300", leftMost)
	}
}

func TestTextOverflow(t *testing.T) {
	previewSvc, _ := setupTestServices(t)

	vis := true
	design := &models.DesignV2{
		Name:    "test-overflow",
		Version: 2,
		Canvas:  models.CanvasConfig{Width: 800, Height: 480, Background: "#FFFFFF"},
		Elements: []models.Element{
			{
				ID:     "o1",
				Type:   "text",
				X:      100,
				Y:      100,
				Width:  100,
				Height: 30,
				ZIndex: 0,
				Visible: &vis,
				Properties: map[string]any{
					"text":      "This is a very long text that should be clipped within the bounding box",
					"fontSize":  24,
					"color":     "#000000",
					"textAlign": "left",
				},
			},
		},
	}

	pngData, err := previewSvc.Render(design, true)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		t.Fatalf("Failed to decode PNG: %v", err)
	}

	// Check that no pixels are rendered outside the bounding box (x: 100-200, y: 100-130)
	for y := 131; y < 200; y++ {
		for x := 100; x < 200; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r < 0xffff || g < 0xffff || b < 0xffff {
				t.Errorf("Text overflows bounding box at pixel (%d, %d)", x, y)
				return
			}
		}
	}
}

func TestRenderImageWithCrop(t *testing.T) {
	previewSvc, tmpDir := setupTestServices(t)

	// Create a simple test image (100x100 with top-left red, rest blue)
	testImg := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			if x < 50 && y < 50 {
				testImg.Set(x, y, color.RGBA{255, 0, 0, 255})
			} else {
				testImg.Set(x, y, color.RGBA{0, 0, 255, 255})
			}
		}
	}

	// Save test image
	imgPath := filepath.Join(tmpDir, "uploaded_images", "test.png")
	f, err := os.Create(imgPath)
	if err != nil {
		t.Fatal(err)
	}
	png.Encode(f, testImg)
	f.Close()

	vis := true
	design := &models.DesignV2{
		Name:    "test-crop",
		Version: 2,
		Canvas:  models.CanvasConfig{Width: 800, Height: 480, Background: "#FFFFFF"},
		Elements: []models.Element{
			{
				ID:     "i1",
				Type:   "image",
				X:      0,
				Y:      0,
				Width:  50,
				Height: 50,
				ZIndex: 0,
				Visible: &vis,
				Properties: map[string]any{
					"image": "test.png",
					"cropX": float64(0),
					"cropY": float64(0),
					"cropW": float64(50),
					"cropH": float64(50),
				},
			},
		},
	}

	pngData, err := previewSvc.Render(design, true)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	rendered, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		t.Fatalf("Failed to decode PNG: %v", err)
	}

	// The cropped image (top-left quadrant) should be mostly red
	r, g, b, _ := rendered.At(10, 10).RGBA()
	if r>>8 < 200 || g>>8 > 50 || b>>8 > 50 {
		t.Errorf("Expected red pixel at (10,10) in cropped image, got R=%d G=%d B=%d", r>>8, g>>8, b>>8)
	}
}

func TestRenderDesignPosition(t *testing.T) {
	previewSvc, _ := setupTestServices(t)

	// Test that widget at position (100, 50) with size (200, 80) renders in correct area
	vis := true
	design := &models.DesignV2{
		Name:    "test-position",
		Version: 2,
		Canvas:  models.CanvasConfig{Width: 800, Height: 480, Background: "#FFFFFF"},
		Elements: []models.Element{
			{
				ID:     "p1",
				Type:   "text",
				X:      100,
				Y:      50,
				Width:  200,
				Height: 80,
				ZIndex: 0,
				Visible: &vis,
				Properties: map[string]any{
					"text":      "Position Test",
					"fontSize":  24,
					"color":     "#000000",
					"textAlign": "left",
				},
			},
		},
	}

	pngData, err := previewSvc.Render(design, true)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		t.Fatalf("Failed to decode PNG: %v", err)
	}

	// Check that pixels before x=100 and before y=50 are all white
	for y := 0; y < 50; y++ {
		for x := 100; x < 300; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r < 0xffff || g < 0xffff || b < 0xffff {
				t.Errorf("Found non-white pixel at (%d, %d) before widget y-position", x, y)
				return
			}
		}
	}

	// Check that text IS rendered within the expected area
	hasContent := false
	for y := 50; y < 80; y++ {
		for x := 100; x < 300; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r < 0xffff || g < 0xffff || b < 0xffff {
				hasContent = true
				break
			}
		}
		if hasContent {
			break
		}
	}
	if !hasContent {
		t.Error("No content rendered at expected widget position")
	}
}

func TestVerticalAlignMiddle(t *testing.T) {
	previewSvc, _ := setupTestServices(t)

	vis := true
	design := &models.DesignV2{
		Name:    "test-valign-middle",
		Version: 2,
		Canvas:  models.CanvasConfig{Width: 800, Height: 480, Background: "#FFFFFF"},
		Elements: []models.Element{
			{
				ID:     "vm1",
				Type:   "text",
				X:      0,
				Y:      0,
				Width:  200,
				Height: 200,
				ZIndex: 0,
				Visible: &vis,
				Properties: map[string]any{
					"text":          "X",
					"fontSize":      24,
					"color":         "#000000",
					"textAlign":     "center",
					"verticalAlign": "middle",
				},
			},
		},
	}

	pngData, err := previewSvc.Render(design, true)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		t.Fatalf("Failed to decode PNG: %v", err)
	}

	// For vertical middle alignment in a 200px tall box, text should be roughly at y=80-120
	topMost := 200
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r < 0xffff || g < 0xffff || b < 0xffff {
				if y < topMost {
					topMost = y
				}
			}
		}
	}

	if topMost < 60 {
		t.Errorf("Vertically middle-aligned text starts too high at y=%d, expected > 60", topMost)
	}
	if topMost > 140 {
		t.Errorf("Vertically middle-aligned text starts too low at y=%d, expected < 140", topMost)
	}
}

func TestVerticalAlignBottom(t *testing.T) {
	previewSvc, _ := setupTestServices(t)

	vis := true
	design := &models.DesignV2{
		Name:    "test-valign-bottom",
		Version: 2,
		Canvas:  models.CanvasConfig{Width: 800, Height: 480, Background: "#FFFFFF"},
		Elements: []models.Element{
			{
				ID:     "vb1",
				Type:   "text",
				X:      0,
				Y:      0,
				Width:  200,
				Height: 200,
				ZIndex: 0,
				Visible: &vis,
				Properties: map[string]any{
					"text":          "X",
					"fontSize":      24,
					"color":         "#000000",
					"textAlign":     "left",
					"verticalAlign": "bottom",
				},
			},
		},
	}

	pngData, err := previewSvc.Render(design, true)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		t.Fatalf("Failed to decode PNG: %v", err)
	}

	// For bottom alignment in a 200px tall box, text should be near the bottom
	topMost := 200
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r < 0xffff || g < 0xffff || b < 0xffff {
				if y < topMost {
					topMost = y
				}
			}
		}
	}

	if topMost < 150 {
		t.Errorf("Bottom-aligned text starts too high at y=%d, expected > 150", topMost)
	}
}

func TestMultipleWidgetZOrder(t *testing.T) {
	previewSvc, _ := setupTestServices(t)

	vis := true
	design := &models.DesignV2{
		Name:    "test-zorder",
		Version: 2,
		Canvas:  models.CanvasConfig{Width: 800, Height: 480, Background: "#FFFFFF"},
		Elements: []models.Element{
			{
				ID:     "bg",
				Type:   "shape",
				X:      0,
				Y:      0,
				Width:  400,
				Height: 200,
				ZIndex: 0,
				Visible: &vis,
				Properties: map[string]any{
					"fill": "#FF0000",
				},
			},
			{
				ID:     "fg",
				Type:   "shape",
				X:      50,
				Y:      50,
				Width:  100,
				Height: 100,
				ZIndex: 1,
				Visible: &vis,
				Properties: map[string]any{
					"fill": "#0000FF",
				},
			},
		},
	}

	pngData, err := previewSvc.Render(design, true)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		t.Fatalf("Failed to decode PNG: %v", err)
	}

	// At (75, 75) — overlap area — should be blue (higher z-index)
	r, g, b, _ := img.At(75, 75).RGBA()
	if b>>8 < 200 || r>>8 > 50 {
		t.Errorf("Expected blue at overlap area, got R=%d G=%d B=%d", r>>8, g>>8, b>>8)
	}

	// At (10, 10) — only red shape
	r2, g2, b2, _ := img.At(10, 10).RGBA()
	if r2>>8 < 200 || b2>>8 > 50 {
		t.Errorf("Expected red outside overlap, got R=%d G=%d B=%d", r2>>8, g2>>8, b2>>8)
	}
}

func TestRenderAllWidgetTypes(t *testing.T) {
	previewSvc, _ := setupTestServices(t)
	vis := true

	widgetTypes := []struct {
		typ   string
		props map[string]any
	}{
		{"widget_weather", map[string]any{"layout": "compact", "fontSize": float64(18), "color": "#000000"}},
		{"widget_forecast", map[string]any{"layout": "vertical", "fontSize": float64(13), "color": "#000000", "days": float64(3)}},
		{"widget_timer", map[string]any{"layout": "countdown_large", "fontSize": float64(24), "color": "#000000", "targetDate": "2027-01-01T00:00:00"}},
		{"widget_custom", map[string]any{"fontSize": float64(24), "color": "#000000"}},
		{"widget_system", map[string]any{"layout": "vertical", "fontSize": float64(12), "color": "#000000"}},
	}

	for _, wt := range widgetTypes {
		t.Run(wt.typ, func(t *testing.T) {
			design := &models.DesignV2{
				Name:    "test-" + wt.typ,
				Version: 2,
				Canvas:  models.CanvasConfig{Width: 800, Height: 480, Background: "#FFFFFF"},
				Elements: []models.Element{
					{
						ID: "w1", Type: wt.typ,
						X: 50, Y: 50, Width: 300, Height: 100,
						ZIndex: 0, Visible: &vis,
						Properties: wt.props,
					},
				},
			}

			pngData, err := previewSvc.Render(design, true)
			if err != nil {
				t.Fatalf("Render %s failed: %v", wt.typ, err)
			}

			img, err := png.Decode(bytes.NewReader(pngData))
			if err != nil {
				t.Fatalf("Failed to decode PNG for %s: %v", wt.typ, err)
			}

			hasContent := false
			for y := 50; y < 150; y++ {
				for x := 50; x < 350; x++ {
					r, g, b, _ := img.At(x, y).RGBA()
					if r < 0xffff || g < 0xffff || b < 0xffff {
						hasContent = true
						break
					}
				}
				if hasContent {
					break
				}
			}

			if !hasContent {
				t.Errorf("Widget %s rendered no visible content", wt.typ)
			}
		})
	}
}

func TestImageResizeQuality(t *testing.T) {
	previewSvc, tmpDir := setupTestServices(t)

	// Create a gradient test image (200x200) with fine detail
	testImg := image.NewRGBA(image.Rect(0, 0, 200, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			// Create alternating thin stripes for detail testing
			if (x/2)%2 == 0 {
				testImg.Set(x, y, color.RGBA{0, 0, 0, 255})
			} else {
				testImg.Set(x, y, color.RGBA{255, 255, 255, 255})
			}
		}
	}

	imgPath := filepath.Join(tmpDir, "uploaded_images", "gradient.png")
	f, err := os.Create(imgPath)
	if err != nil {
		t.Fatal(err)
	}
	png.Encode(f, testImg)
	f.Close()

	vis := true
	design := &models.DesignV2{
		Name:    "test-resize-quality",
		Version: 2,
		Canvas:  models.CanvasConfig{Width: 800, Height: 480, Background: "#FFFFFF"},
		Elements: []models.Element{
			{
				ID: "img1", Type: "image",
				X: 0, Y: 0, Width: 100, Height: 100,
				ZIndex: 0, Visible: &vis,
				Properties: map[string]any{"image": "gradient.png"},
			},
		},
	}

	pngData, err := previewSvc.Render(design, true)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		t.Fatalf("Failed to decode PNG: %v", err)
	}

	// With CatmullRom downsampling, the striped pattern should produce
	// intermediate gray values (anti-aliasing), not just pure black/white.
	// Count pixels that are neither pure black nor pure white.
	intermediateCount := 0
	for y := 10; y < 90; y++ {
		for x := 10; x < 90; x++ {
			r, _, _, _ := img.At(x, y).RGBA()
			val := r >> 8
			if val > 20 && val < 235 {
				intermediateCount++
			}
		}
	}

	// CatmullRom should produce many intermediate values from the stripe pattern
	if intermediateCount == 0 {
		t.Error("No intermediate gray values found — image resampling may be using nearest-neighbor")
	}
	t.Logf("Intermediate pixel count: %d (expected >0 for CatmullRom quality)", intermediateCount)
}

func TestTextNotOutsideBoundingBox(t *testing.T) {
	previewSvc, _ := setupTestServices(t)

	vis := true
	design := &models.DesignV2{
		Name:    "test-clip-strict",
		Version: 2,
		Canvas:  models.CanvasConfig{Width: 800, Height: 480, Background: "#FFFFFF"},
		Elements: []models.Element{
			{
				ID: "clip1", Type: "text",
				X: 200, Y: 200, Width: 150, Height: 40,
				ZIndex: 0, Visible: &vis,
				Properties: map[string]any{
					"text":      "This text overflows the box definitely",
					"fontSize":  32,
					"color":     "#000000",
					"textAlign": "left",
				},
			},
		},
	}

	pngData, err := previewSvc.Render(design, true)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		t.Fatalf("Failed to decode PNG: %v", err)
	}

	// Check NO significant pixels rendered outside bounding box in any direction.
	// Allow near-white anti-aliasing bleed (threshold 0xF000) from CatmullRom supersampling
	// at the 1px boundary zone, but check strictly 2+ pixels away.
	const aaThreshold uint32 = 0xF000 // near-white anti-aliasing is acceptable
	const strictMargin = 2            // pixels away from boundary to check strictly

	// Below the box (2+ pixels away)
	for y := 240 + strictMargin; y < 300; y++ {
		for x := 200; x < 350; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r < aaThreshold || g < aaThreshold || b < aaThreshold {
				t.Fatalf("Text overflows below bounding box at (%d, %d)", x, y)
			}
		}
	}

	// Right of the box (2+ pixels away)
	for y := 200; y < 240; y++ {
		for x := 350 + strictMargin; x < 500; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r < aaThreshold || g < aaThreshold || b < aaThreshold {
				t.Fatalf("Text overflows right of bounding box at (%d, %d)", x, y)
			}
		}
	}

	// Left of the box (2+ pixels away)
	for y := 200; y < 240; y++ {
		for x := 150; x < 200 - strictMargin; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r < aaThreshold || g < aaThreshold || b < aaThreshold {
				t.Fatalf("Text overflows left of bounding box at (%d, %d)", x, y)
			}
		}
	}
}

func TestPaletteQuantization(t *testing.T) {
	previewSvc, _ := setupTestServices(t)

	vis := true
	design := &models.DesignV2{
		Name:    "test-quantize",
		Version: 2,
		Canvas:  models.CanvasConfig{Width: 800, Height: 480, Background: "#FFFFFF"},
		Elements: []models.Element{
			{
				ID: "s1", Type: "shape",
				X: 0, Y: 0, Width: 400, Height: 240,
				ZIndex: 0, Visible: &vis,
				Properties: map[string]any{"fill": "#FF0000"},
			},
			{
				ID: "s2", Type: "shape",
				X: 400, Y: 0, Width: 400, Height: 240,
				ZIndex: 0, Visible: &vis,
				Properties: map[string]any{"fill": "#0000FF"},
			},
		},
	}

	// Render with quantization (raw=false)
	pngData, err := previewSvc.Render(design, false)
	if err != nil {
		t.Fatalf("Render with quantization failed: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		t.Fatalf("Failed to decode PNG: %v", err)
	}

	// The quantized image should be a paletted image
	if _, ok := img.(*image.Paletted); !ok {
		t.Error("Expected paletted image after quantization")
	}
}

func TestFullDesignAllElementTypes(t *testing.T) {
	previewSvc, tmpDir := setupTestServices(t)

	// Create a test image
	testImg := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			testImg.Set(x, y, color.RGBA{128, 64, 32, 255})
		}
	}
	imgPath := filepath.Join(tmpDir, "uploaded_images", "test_full.png")
	f, _ := os.Create(imgPath)
	png.Encode(f, testImg)
	f.Close()

	vis := true
	design := &models.DesignV2{
		Name:    "test-full-design",
		Version: 2,
		Canvas:  models.CanvasConfig{Width: 800, Height: 480, Background: "#FFFFFF"},
		Elements: []models.Element{
			{ID: "bg", Type: "shape", X: 0, Y: 0, Width: 800, Height: 480, ZIndex: 0, Visible: &vis,
				Properties: map[string]any{"fill": "#F0F0F0", "rx": float64(0)}},
			{ID: "t1", Type: "text", X: 10, Y: 10, Width: 300, Height: 40, ZIndex: 1, Visible: &vis,
				Properties: map[string]any{"text": "Hello E-Ink", "fontSize": float64(24), "color": "#000000", "textAlign": "left"}},
			{ID: "t2", Type: "textbox", X: 10, Y: 60, Width: 300, Height: 80, ZIndex: 2, Visible: &vis,
				Properties: map[string]any{"text": "Multiline text that wraps across lines", "fontSize": float64(16), "color": "#333333", "textAlign": "center", "verticalAlign": "middle"}},
			{ID: "img1", Type: "image", X: 320, Y: 10, Width: 150, Height: 100, ZIndex: 3, Visible: &vis,
				Properties: map[string]any{"image": "test_full.png"}},
			{ID: "s1", Type: "shape", X: 500, Y: 10, Width: 120, Height: 60, ZIndex: 4, Visible: &vis,
				Properties: map[string]any{"fill": "#0000FF", "stroke": "#000000", "strokeWidth": float64(2), "rx": float64(8)}},
			{ID: "clk", Type: "widget_clock", X: 10, Y: 160, Width: 200, Height: 60, ZIndex: 5, Visible: &vis,
				Properties: map[string]any{"layout": "digital_large", "fontSize": float64(48), "color": "#000000", "textAlign": "center"}},
			{ID: "tmr", Type: "widget_timer", X: 220, Y: 160, Width: 250, Height: 50, ZIndex: 6, Visible: &vis,
				Properties: map[string]any{"layout": "countdown_large", "fontSize": float64(20), "color": "#000000", "targetDate": "2027-01-01T00:00:00"}},
			{ID: "sys", Type: "widget_system", X: 10, Y: 230, Width: 300, Height: 80, ZIndex: 7, Visible: &vis,
				Properties: map[string]any{"layout": "vertical", "fontSize": float64(12), "color": "#000000"}},
			{ID: "cust", Type: "widget_custom", X: 320, Y: 230, Width: 200, Height: 40, ZIndex: 8, Visible: &vis,
				Properties: map[string]any{"fontSize": float64(16), "color": "#000000"}},
		},
	}

	// Render raw (no quantization)
	pngRaw, err := previewSvc.Render(design, true)
	if err != nil {
		t.Fatalf("Raw render failed: %v", err)
	}
	if len(pngRaw) == 0 {
		t.Fatal("Raw render returned empty data")
	}

	imgRaw, err := png.Decode(bytes.NewReader(pngRaw))
	if err != nil {
		t.Fatalf("Failed to decode raw PNG: %v", err)
	}

	// Verify canvas dimensions
	bounds := imgRaw.Bounds()
	if bounds.Dx() != 800 || bounds.Dy() != 480 {
		t.Errorf("Expected 800x480 canvas, got %dx%d", bounds.Dx(), bounds.Dy())
	}

	// Verify background is not pure white (we set #F0F0F0)
	r, g, b, _ := imgRaw.At(700, 450).RGBA()
	if r>>8 == 255 && g>>8 == 255 && b>>8 == 255 {
		t.Error("Background should be #F0F0F0, not pure white")
	}

	// Verify text area has content
	hasText := false
	for y := 10; y < 50; y++ {
		for x := 10; x < 310; x++ {
			r, g, b, _ := imgRaw.At(x, y).RGBA()
			if r>>8 < 200 || g>>8 < 200 || b>>8 < 200 {
				hasText = true
				break
			}
		}
		if hasText {
			break
		}
	}
	if !hasText {
		t.Error("No text rendered in text element area")
	}

	// Verify image area has non-background pixels
	hasImage := false
	for y := 10; y < 110; y++ {
		for x := 320; x < 470; x++ {
			r, _, _, _ := imgRaw.At(x, y).RGBA()
			val := r >> 8
			if val != 240 { // not #F0F0F0 background
				hasImage = true
				break
			}
		}
		if hasImage {
			break
		}
	}
	if !hasImage {
		t.Error("No image rendered in image element area")
	}

	// Verify blue shape
	r, g, b, _ = imgRaw.At(560, 40).RGBA()
	if b>>8 < 200 {
		t.Errorf("Expected blue shape at (560,40), got R=%d G=%d B=%d", r>>8, g>>8, b>>8)
	}

	// Render with quantization
	pngQuant, err := previewSvc.Render(design, false)
	if err != nil {
		t.Fatalf("Quantized render failed: %v", err)
	}

	imgQuant, err := png.Decode(bytes.NewReader(pngQuant))
	if err != nil {
		t.Fatalf("Failed to decode quantized PNG: %v", err)
	}

	if _, ok := imgQuant.(*image.Paletted); !ok {
		t.Error("Quantized output should be paletted")
	}

	t.Logf("Full design rendered: raw=%d bytes, quantized=%d bytes", len(pngRaw), len(pngQuant))
}
