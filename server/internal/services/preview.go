package services

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"e-ink-picture/server/internal/models"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	einkOffsetX = 200
	einkOffsetY = 160
	einkWidth   = 800
	einkHeight  = 480
)

// PreviewService renders design previews as PNGs with display-appropriate palette.
type PreviewService struct {
	design   *DesignService
	weather  *WeatherService
	image    *ImageService
	settings *SettingsService
	dataDir  string
}

// NewPreviewService creates a PreviewService with access to other services.
func NewPreviewService(d *DesignService, w *WeatherService, i *ImageService, s *SettingsService, dataDir string) *PreviewService {
	return &PreviewService{design: d, weather: w, image: i, settings: s, dataDir: dataDir}
}

// Render fills dynamic content and renders a design to a palette-quantized PNG.
// If raw is true, no palette quantization is applied (debug mode).
func (s *PreviewService) Render(design *models.Design, raw bool) ([]byte, error) {
	s.FillContent(design)

	displayCfg := s.settings.GetDisplayConfig()

	// Render to full-color RGBA canvas first
	img := image.NewRGBA(image.Rect(0, 0, einkWidth, einkHeight))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)

	for i := range design.Modules {
		m := &design.Modules[i]
		x := m.Position.X - einkOffsetX
		y := m.Position.Y - einkOffsetY
		w := m.Size.Width
		h := m.Size.Height

		if x+w < 0 || x > einkWidth || y+h < 0 || y > einkHeight {
			continue
		}

		sd := m.StyleData
		fontSize := 18
		if sd.FontSize != nil {
			if v, err := strconv.Atoi(*sd.FontSize); err == nil {
				fontSize = v
			}
		}
		bold := sd.FontBold != nil && *sd.FontBold == "true"
		italic := sd.FontItalic != nil && *sd.FontItalic == "true"
		strike := sd.FontStrike != nil && *sd.FontStrike == "true"
		align := "left"
		if sd.TextAlign != nil {
			align = *sd.TextAlign
		}

		textColor := resolveTextColor(sd.TextColor, displayCfg)
		face := s.loadFontFace(sd.Font, fontSize)

		switch m.Type {
		case "text", "news", "weather", "datetime", "timer", "calendar":
			s.renderText(img, x, y, w, h, m.Content, face, bold, italic, strike, align, textColor)
		case "image":
			s.renderImageRGBA(img, x, y, w, h, &sd)
		case "line":
			s.drawFilledRectRGBA(img, x, y, w, h, textColor)
		}
	}

	// Quantize to display palette (unless raw mode)
	var output image.Image
	if raw {
		output = img
	} else {
		output = quantizeToPalette(img, displayCfg.Colors)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, output); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return buf.Bytes(), nil
}

// resolveTextColor parses a hex color from style data, defaulting to black.
func resolveTextColor(textColor *string, _ models.DisplayConfig) color.RGBA {
	if textColor != nil && len(*textColor) == 7 && (*textColor)[0] == '#' {
		return parseHexColor(*textColor)
	}
	return color.RGBA{0, 0, 0, 255}
}

// parseHexColor converts "#RRGGBB" to color.RGBA.
func parseHexColor(hex string) color.RGBA {
	var r, g, b uint8
	fmt.Sscanf(hex[1:], "%02x%02x%02x", &r, &g, &b)
	return color.RGBA{r, g, b, 255}
}

// quantizeToPalette reduces an image to a specific color palette using Floyd-Steinberg dithering.
func quantizeToPalette(img image.Image, hexColors []string) *image.Paletted {
	pal := make(color.Palette, 0, len(hexColors))
	for _, hex := range hexColors {
		pal = append(pal, parseHexColor(hex))
	}

	// Fallback if palette is somehow empty
	if len(pal) == 0 {
		pal = color.Palette{color.White, color.Black}
	}

	bounds := img.Bounds()
	paletted := image.NewPaletted(bounds, pal)
	draw.FloydSteinberg.Draw(paletted, bounds, img, image.Point{})
	return paletted
}

// RenderActive renders the currently active design.
func (s *PreviewService) RenderActive() ([]byte, error) {
	design, err := s.design.GetActive()
	if err != nil {
		return nil, err
	}
	if design == nil {
		return nil, fmt.Errorf("no active design")
	}
	return s.Render(design, false)
}

// FillContent populates dynamic content for all modules in a design.
func (s *PreviewService) FillContent(design *models.Design) {
	for i := range design.Modules {
		m := &design.Modules[i]
		sd := m.StyleData

		switch m.Type {
		case "datetime":
			dtFmt := "YYYY-MM-DD HH:mm"
			if sd.DatetimeFormat != nil {
				dtFmt = *sd.DatetimeFormat
			}
			m.Content = formatDateTime(dtFmt)

		case "weather":
			lat := "52.52"
			lon := "13.41"
			if sd.Latitude != nil {
				lat = *sd.Latitude
			}
			if sd.Longitude != nil {
				lon = *sd.Longitude
			}
			wdata, err := s.weather.FetchForLocation(lat, lon)
			if err != nil || wdata == nil {
				m.Content = "No data"
			} else {
				ws := "default"
				if sd.WeatherStyle != nil {
					ws = *sd.WeatherStyle
				}
				m.Content = s.weather.ApplyStyle(ws, wdata)
			}

		case "timer":
			target := "2025-01-01 00:00:00"
			if sd.TimerTarget != nil {
				target = *sd.TimerTarget
			}
			tfmt := "D days, HH:MM:SS"
			if sd.TimerFormat != nil {
				tfmt = *sd.TimerFormat
			}
			m.Content = formatTimer(target, tfmt)

		case "calendar":
			calURL := ""
			if sd.CalendarURL != nil {
				calURL = *sd.CalendarURL
			}
			maxEvents := 5
			if sd.MaxEvents != nil {
				if v, err := strconv.Atoi(*sd.MaxEvents); err == nil {
					maxEvents = v
				}
			}
			m.Content = fetchCalendarContent(calURL, maxEvents)
		}
	}
}

// --- Content filling helpers ---

// formatDateTime converts a user-facing date format to Go time format and returns the current time.
func formatDateTime(format string) string {
	// Replace user tokens with Go time reference values.
	// Order matters: YYYY before MM (to avoid partial replacement of MM inside other tokens).
	r := strings.NewReplacer(
		"YYYY", "2006",
		"MM", "01",
		"DD", "02",
		"HH", "15",
		"mm", "04",
		"ss", "05",
	)
	goFmt := r.Replace(format)
	return time.Now().Format(goFmt)
}

// formatTimer calculates countdown from now to target and formats it.
func formatTimer(target, format string) string {
	targetDT, err := time.ParseInLocation("2006-01-02 15:04:05", target, time.Now().Location())
	if err != nil {
		return "Invalid timer target"
	}

	diff := targetDT.Sub(time.Now())
	if diff < 0 {
		return "Time's up!"
	}

	totalSecs := int(diff.Seconds())
	days := totalSecs / 86400
	remainder := totalSecs % 86400
	hours := remainder / 3600
	minutes := (remainder % 3600) / 60
	seconds := remainder % 60

	display := strings.Replace(format, "D", strconv.Itoa(days), 1)
	display = strings.Replace(display, "HH", fmt.Sprintf("%02d", hours), 1)
	display = strings.Replace(display, "MM", fmt.Sprintf("%02d", minutes), 1)
	display = strings.Replace(display, "SS", fmt.Sprintf("%02d", seconds), 1)
	return display
}

// fetchCalendarContent fetches iCal events and formats them.
// NOTE: Full iCal parsing without external deps is limited; we do a best-effort parse.
func fetchCalendarContent(calURL string, maxEvents int) string {
	if calURL == "" {
		return "No events"
	}

	// Convert webcal:// to https://
	if strings.HasPrefix(calURL, "webcal://") {
		calURL = "https://" + calURL[len("webcal://"):]
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(calURL)
	if err != nil {
		slog.Error("failed to fetch calendar", "error", err)
		return "No events"
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "No events"
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "No events"
	}

	events := parseICalEvents(string(body), maxEvents)
	if len(events) == 0 {
		return "No events"
	}

	var lines []string
	for _, e := range events {
		lines = append(lines, fmt.Sprintf("%s - %s", e.Start.Format("2006-01-02 15:04"), e.Summary))
	}
	return strings.Join(lines, "\n")
}

type calEvent struct {
	Start   time.Time
	Summary string
}

// parseICalEvents does a minimal iCal VEVENT parse without external dependencies.
func parseICalEvents(ical string, maxEvents int) []calEvent {
	var events []calEvent
	now := time.Now()

	lines := strings.Split(strings.ReplaceAll(ical, "\r\n", "\n"), "\n")
	// Handle line folding (lines starting with space/tab are continuations)
	var unfolded []string
	for _, line := range lines {
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') && len(unfolded) > 0 {
			unfolded[len(unfolded)-1] += line[1:]
		} else {
			unfolded = append(unfolded, line)
		}
	}

	inEvent := false
	var currentStart time.Time
	var currentSummary string

	for _, line := range unfolded {
		trimmed := strings.TrimSpace(line)
		if trimmed == "BEGIN:VEVENT" {
			inEvent = true
			currentStart = time.Time{}
			currentSummary = ""
			continue
		}
		if trimmed == "END:VEVENT" {
			if inEvent && !currentStart.IsZero() && currentStart.After(now) {
				events = append(events, calEvent{Start: currentStart, Summary: currentSummary})
			}
			inEvent = false
			continue
		}
		if !inEvent {
			continue
		}

		if strings.HasPrefix(trimmed, "DTSTART") {
			currentStart = parseICalDate(trimmed)
		} else if strings.HasPrefix(trimmed, "SUMMARY:") {
			currentSummary = strings.TrimPrefix(trimmed, "SUMMARY:")
		}
	}

	// Sort by start time
	sort.Slice(events, func(i, j int) bool {
		return events[i].Start.Before(events[j].Start)
	})

	if len(events) > maxEvents {
		events = events[:maxEvents]
	}
	return events
}

// parseICalDate parses an iCal DTSTART line.
func parseICalDate(line string) time.Time {
	// Examples:
	// DTSTART:20250315T100000Z
	// DTSTART;VALUE=DATE:20250315
	// DTSTART;TZID=Europe/Berlin:20250315T100000
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return time.Time{}
	}
	val := strings.TrimSpace(parts[1])

	formats := []string{
		"20060102T150405Z",
		"20060102T150405",
		"20060102",
	}

	for _, f := range formats {
		if t, err := time.ParseInLocation(f, val, time.Now().Location()); err == nil {
			return t
		}
	}
	return time.Time{}
}

// --- Font loading ---

// loadFontFace loads a TrueType font face, falling back to system fonts then a basic built-in face.
func (s *PreviewService) loadFontFace(fontName *string, size int) font.Face {
	// Try custom font from fonts directory
	if fontName != nil && *fontName != "" {
		fontPath, err := s.image.GetFontPath(*fontName)
		if err == nil && fontPath != "" {
			if face := loadTTFFace(fontPath, size); face != nil {
				return face
			}
		}
	}

	// Fallback: DejaVuSans-Bold (common on Linux/Raspberry Pi)
	defaultPaths := []string{
		"/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/TTF/DejaVuSans-Bold.ttf",
	}
	for _, p := range defaultPaths {
		if face := loadTTFFace(p, size); face != nil {
			return face
		}
	}

	// Ultimate fallback: basic 7x13 font from x/image
	return newBasicFace(size)
}

// loadTTFFace loads a TrueType font file and returns a font.Face.
func loadTTFFace(path string, size int) font.Face {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	f, err := opentype.Parse(data)
	if err != nil {
		return nil
	}

	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    float64(size),
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil
	}
	return face
}

// --- Basic fallback font ---

// basicFace implements font.Face with a simple monospace bitmap for fallback.
type basicFace struct {
	size    int
	advance fixed.Int26_6
	height  fixed.Int26_6
}

func newBasicFace(size int) font.Face {
	return &basicFace{
		size:    size,
		advance: fixed.I(size * 3 / 5), // approximate char width
		height:  fixed.I(size),
	}
}

func (f *basicFace) Close() error { return nil }

func (f *basicFace) Glyph(dot fixed.Point26_6, r rune) (dr image.Rectangle, mask image.Image, maskp image.Point, advance fixed.Int26_6, ok bool) {
	// Simple rectangle glyph for each character
	x := dot.X.Floor()
	y := dot.Y.Floor() - f.size + f.size/5
	w := f.advance.Floor()
	h := f.size

	dr = image.Rect(x, y, x+w, y+h)

	// Create a simple mask - all opaque for visible chars, empty for space
	if r == ' ' {
		mask = image.NewAlpha(image.Rect(0, 0, w, h))
	} else {
		// Draw a simple filled rectangle as glyph placeholder
		m := image.NewAlpha(image.Rect(0, 0, w, h))
		// Fill interior pixels, leaving 1px border for spacing
		for py := 1; py < h-1; py++ {
			for px := 0; px < w-1; px++ {
				m.SetAlpha(px, py, color.Alpha{A: 255})
			}
		}
		mask = m
	}
	maskp = image.Point{}
	advance = f.advance
	ok = true
	return
}

func (f *basicFace) GlyphBounds(r rune) (bounds fixed.Rectangle26_6, advance fixed.Int26_6, ok bool) {
	bounds = fixed.R(0, -f.size, f.advance.Floor(), 0)
	return bounds, f.advance, true
}

func (f *basicFace) GlyphAdvance(r rune) (advance fixed.Int26_6, ok bool) {
	return f.advance, true
}

func (f *basicFace) Kern(r0, r1 rune) fixed.Int26_6 {
	return 0
}

func (f *basicFace) Metrics() font.Metrics {
	return font.Metrics{
		Height:  f.height,
		Ascent:  fixed.I(f.size * 4 / 5),
		Descent: fixed.I(f.size / 5),
	}
}

// --- Text rendering ---

// renderText draws word-wrapped text on an RGBA image.
func (s *PreviewService) renderText(img *image.RGBA, x, y, w, h int, text string, face font.Face, bold, italic, strike bool, align string, textColor color.RGBA) {
	if text == "" {
		return
	}

	lines := strings.Split(text, "\n")
	var wrapped []string

	for _, line := range lines {
		if line == "" {
			wrapped = append(wrapped, "")
			continue
		}
		words := strings.Split(line, " ")
		var current []string
		currentWidth := fixed.Int26_6(0)

		for _, word := range words {
			ww := measureString(face, word)
			if len(current) > 0 {
				spaceW := measureString(face, " ")
				if (currentWidth + spaceW + ww).Ceil() <= w {
					current = append(current, word)
					currentWidth += spaceW + ww
				} else {
					wrapped = append(wrapped, strings.Join(current, " "))
					current = []string{word}
					currentWidth = ww
				}
			} else {
				current = []string{word}
				currentWidth = ww
			}
		}
		if len(current) > 0 {
			wrapped = append(wrapped, strings.Join(current, " "))
		}
	}

	// Line height matches Python: font.size + 2
	metrics := face.Metrics()
	fontSize := (metrics.Ascent + metrics.Descent).Ceil()
	lineHeight := fontSize + 2

	iy := y
	for _, line := range wrapped {
		if iy+lineHeight > y+h {
			break
		}

		lineWidth := measureString(face, line).Ceil()
		var lx int
		switch align {
		case "center":
			lx = x + (w-lineWidth)/2
		case "right":
			lx = x + (w - lineWidth)
		default:
			lx = x
		}

		if italic {
			lx++
		}

		// Draw text using font.Drawer
		dot := fixed.Point26_6{
			X: fixed.I(lx),
			Y: fixed.I(iy) + metrics.Ascent,
		}

		drawer := &font.Drawer{
			Dst:  img,
			Src:  image.NewUniform(textColor),
			Face: face,
			Dot:  dot,
		}
		drawer.DrawString(line)

		// Bold: draw again shifted 1px right
		if bold {
			drawer2 := &font.Drawer{
				Dst:  img,
				Src:  image.NewUniform(textColor),
				Face: face,
				Dot: fixed.Point26_6{
					X: fixed.I(lx + 1),
					Y: fixed.I(iy) + metrics.Ascent,
				},
			}
			drawer2.DrawString(line)
		}

		// Strikethrough: line through middle
		if strike {
			midY := iy + fontSize/2
			for px := lx; px < lx+lineWidth; px++ {
				if px >= 0 && px < einkWidth && midY >= 0 && midY < einkHeight {
					img.SetRGBA(px, midY, textColor)
				}
			}
		}

		iy += lineHeight
	}
}

// measureString measures the width of a string using a font face.
func measureString(face font.Face, s string) fixed.Int26_6 {
	var width fixed.Int26_6
	prev := rune(-1)
	for _, r := range s {
		if prev >= 0 {
			width += face.Kern(prev, r)
		}
		adv, ok := face.GlyphAdvance(r)
		if ok {
			width += adv
		}
		prev = r
	}
	return width
}

// --- Image rendering ---

// renderImageRGBA loads, crops, resizes and pastes an image module onto an RGBA canvas.
func (s *PreviewService) renderImageRGBA(img *image.RGBA, x, y, w, h int, sd *models.StyleData) {
	if sd.Image == nil || *sd.Image == "" {
		return
	}

	imgPath, err := s.image.GetImagePath(*sd.Image)
	if err != nil || imgPath == "" {
		imgPath = filepath.Join(s.dataDir, "uploaded_images", *sd.Image)
	}

	f, err := os.Open(imgPath)
	if err != nil {
		slog.Error("failed to open image", "path", imgPath, "error", err)
		return
	}
	defer f.Close()

	srcImg, _, err := image.Decode(f)
	if err != nil {
		slog.Error("failed to decode image", "path", imgPath, "error", err)
		return
	}

	bounds := srcImg.Bounds()

	cx, cy := 0, 0
	cw, ch := bounds.Dx(), bounds.Dy()
	if sd.CropX != nil {
		cx = int(*sd.CropX)
	}
	if sd.CropY != nil {
		cy = int(*sd.CropY)
	}
	if sd.CropW != nil {
		cw = int(*sd.CropW)
	}
	if sd.CropH != nil {
		ch = int(*sd.CropH)
	}

	cropRect := image.Rect(cx, cy, cx+cw, cy+ch).Intersect(bounds)
	cropped := cropSubImage(srcImg, cropRect)
	resized := resizeNearest(cropped, w, h)

	// Paste onto RGBA canvas
	destRect := image.Rect(x, y, x+w, y+h).Intersect(img.Bounds())
	draw.Draw(img, destRect, resized, image.Point{X: destRect.Min.X - x, Y: destRect.Min.Y - y}, draw.Over)
}

// cropSubImage extracts a rectangular region from an image.
func cropSubImage(src image.Image, r image.Rectangle) image.Image {
	if si, ok := src.(interface{ SubImage(image.Rectangle) image.Image }); ok {
		return si.SubImage(r)
	}
	// Manual crop fallback
	dst := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(dst, dst.Bounds(), src, r.Min, draw.Src)
	return dst
}

// resizeNearest performs nearest-neighbor resize.
func resizeNearest(src image.Image, dstW, dstH int) image.Image {
	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()

	if srcW == 0 || srcH == 0 || dstW == 0 || dstH == 0 {
		return image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	}

	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	for dy := 0; dy < dstH; dy++ {
		sy := srcBounds.Min.Y + int(math.Floor(float64(dy)*float64(srcH)/float64(dstH)))
		for dx := 0; dx < dstW; dx++ {
			sx := srcBounds.Min.X + int(math.Floor(float64(dx)*float64(srcW)/float64(dstW)))
			dst.Set(dx, dy, src.At(sx, sy))
		}
	}
	return dst
}

// --- Line rendering ---

// drawFilledRectRGBA draws a filled rectangle on an RGBA image.
func (s *PreviewService) drawFilledRectRGBA(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	rect := image.Rect(x, y, x+w+1, y+h+1).Intersect(img.Bounds())
	draw.Draw(img, rect, image.NewUniform(c), image.Point{}, draw.Src)
}
