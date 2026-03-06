package services

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"log/slog"
	"math"
	"os"
	"sync"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"e-ink-picture/server/internal/models"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	einkOffsetX = 200
	einkOffsetY = 160
	einkWidth   = 800
	einkHeight  = 480
)

const maxFontCacheEntries = 10

type fontCacheKey struct {
	path string
	size int
}

// PreviewService renders design previews as PNGs with display-appropriate palette.
type PreviewService struct {
	design   *DesignService
	weather  *WeatherService
	image    *ImageService
	settings *SettingsService
	dataDir  string
	fontMu   sync.RWMutex
	fontCache map[fontCacheKey]font.Face
}

// NewPreviewService creates a PreviewService with access to other services.
func NewPreviewService(d *DesignService, w *WeatherService, i *ImageService, s *SettingsService, dataDir string) *PreviewService {
	return &PreviewService{
		design:    d,
		weather:   w,
		image:     i,
		settings:  s,
		dataDir:   dataDir,
		fontCache: make(map[fontCacheKey]font.Face),
	}
}

// Render fills dynamic content and renders a v2 design to a palette-quantized PNG.
// If raw is true, no palette quantization is applied (debug mode).
func (s *PreviewService) Render(design *models.DesignV2, raw bool) ([]byte, error) {
	displayCfg := s.settings.GetDisplayConfig()

	canvasW := design.Canvas.Width
	canvasH := design.Canvas.Height
	if canvasW == 0 {
		canvasW = einkWidth
	}
	if canvasH == 0 {
		canvasH = einkHeight
	}

	// Render to full-color RGBA canvas
	img := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))

	// Background color
	var bgColor color.Color = color.White
	if design.Canvas.Background != "" && design.Canvas.Background != "#FFFFFF" {
		bgColor = parseHexColor(design.Canvas.Background)
	}
	draw.Draw(img, img.Bounds(), image.NewUniform(bgColor), image.Point{}, draw.Src)

	// Sort elements by zIndex
	sorted := make([]models.Element, len(design.Elements))
	copy(sorted, design.Elements)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ZIndex < sorted[j].ZIndex
	})

	for i := range sorted {
		elem := &sorted[i]
		if !elem.Visible {
			continue
		}

		x := int(elem.X)
		y := int(elem.Y)
		w := int(elem.Width)
		h := int(elem.Height)

		if x+w < 0 || x > canvasW || y+h < 0 || y > canvasH {
			continue
		}

		props := elem.Properties
		if props == nil {
			props = make(map[string]any)
		}

		fontSize := GetPropInt(props, "fontSize", 18)
		bold := GetPropBool(props, "bold", false)
		italic := GetPropBool(props, "italic", false)
		strike := GetPropBool(props, "strikethrough", false)
		align := GetPropString(props, "textAlign", "left")
		colorStr := GetPropString(props, "color", "#000000")
		textColor := parseHexColor(colorStr)
		fontFamily := GetPropString(props, "fontFamily", "")
		var fontPtr *string
		if fontFamily != "" {
			fontPtr = &fontFamily
		}
		face := s.loadFontFace(fontPtr, fontSize)

		switch elem.Type {
		case "text":
			content := s.fillTextContent(props)
			s.renderText(img, x, y, w, h, content, face, bold, italic, strike, align, textColor)

		case "image":
			s.renderImageElement(img, x, y, w, h, props)

		case "shape":
			s.renderShapeElement(img, x, y, w, h, props, textColor)

		case "widget_clock":
			content := s.fillClockContent(props)
			s.renderText(img, x, y, w, h, content, face, bold, italic, strike, align, textColor)

		case "widget_weather":
			content := s.fillWeatherContent(props)
			s.renderText(img, x, y, w, h, content, face, bold, italic, strike, align, textColor)

		case "widget_forecast":
			content := s.fillForecastContent(props)
			s.renderText(img, x, y, w, h, content, face, bold, italic, strike, align, textColor)

		case "widget_calendar":
			content := s.fillCalendarContent(props)
			s.renderText(img, x, y, w, h, content, face, bold, italic, strike, align, textColor)

		case "widget_news":
			content := s.fillNewsContent(props)
			s.renderText(img, x, y, w, h, content, face, bold, italic, strike, align, textColor)

		case "widget_timer":
			content := s.fillTimerContent(props)
			s.renderText(img, x, y, w, h, content, face, bold, italic, strike, align, textColor)

		case "widget_custom":
			content := s.fillCustomContent(props)
			s.renderText(img, x, y, w, h, content, face, bold, italic, strike, align, textColor)

		case "widget_system":
			content := s.fillSystemContent(props)
			s.renderText(img, x, y, w, h, content, face, bold, italic, strike, align, textColor)
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

// --- Content filling methods for v2 elements ---

func (s *PreviewService) fillTextContent(props map[string]any) string {
	if t := GetPropString(props, "text", ""); t != "" {
		return t
	}
	return GetPropString(props, "content", "")
}

func (s *PreviewService) fillClockContent(props map[string]any) string {
	format := GetPropString(props, "format", "YYYY-MM-DD HH:mm")
	tz := GetPropString(props, "timezone", "")

	loc := time.Now().Location()
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}

	r := strings.NewReplacer(
		"YYYY", "2006",
		"MM", "01",
		"DD", "02",
		"HH", "15",
		"mm", "04",
		"ss", "05",
	)
	goFmt := r.Replace(format)
	return time.Now().In(loc).Format(goFmt)
}

func (s *PreviewService) fillWeatherContent(props map[string]any) string {
	lat := GetPropString(props, "latitude", "52.52")
	lon := GetPropString(props, "longitude", "13.41")
	style := GetPropString(props, "style", "compact")

	wdata, err := s.weather.FetchForLocation(lat, lon)
	if err != nil || wdata == nil {
		return "No data"
	}

	switch style {
	case "detailed":
		return fmt.Sprintf("%.0f°C %s\nHumidity: --%%\nWind: -- km/h", wdata.CurrentTemp, wdata.CurrentDesc)
	case "minimal":
		return fmt.Sprintf("%.0f°C", wdata.CurrentTemp)
	case "icon_only":
		return wdata.CurrentIcon
	default: // compact
		return fmt.Sprintf("%.0f°C %s", wdata.CurrentTemp, wdata.CurrentDesc)
	}
}

func (s *PreviewService) fillForecastContent(props map[string]any) string {
	lat := GetPropString(props, "latitude", "52.52")
	lon := GetPropString(props, "longitude", "13.41")
	days := GetPropInt(props, "days", 3)

	wdata, err := s.weather.FetchForLocation(lat, lon)
	if err != nil || wdata == nil {
		return "No forecast data"
	}

	var lines []string
	for i, day := range wdata.Daily {
		if i >= days {
			break
		}
		lines = append(lines, fmt.Sprintf("%s: %d-%d°C %s",
			day.Weekday, int(day.Min), int(day.Max), day.Desc))
	}
	if len(lines) == 0 {
		return "No forecast data"
	}
	return strings.Join(lines, "\n")
}

func (s *PreviewService) fillCalendarContent(props map[string]any) string {
	calURL := GetPropString(props, "icalUrl", "")
	maxEvents := GetPropInt(props, "maxEvents", 5)
	return fetchCalendarContent(calURL, maxEvents)
}

func (s *PreviewService) fillNewsContent(props map[string]any) string {
	feedURL := GetPropString(props, "feedUrl", "")
	maxItems := GetPropInt(props, "maxItems", 5)
	title := GetPropString(props, "title", "")

	if feedURL == "" {
		return "No feed URL"
	}

	items := fetchRSSFeed(feedURL, maxItems)
	if len(items) == 0 {
		return "No news"
	}

	var lines []string
	if title != "" {
		lines = append(lines, title)
	}
	for _, item := range items {
		lines = append(lines, "- "+item.Title)
	}
	return strings.Join(lines, "\n")
}

func (s *PreviewService) fillTimerContent(props map[string]any) string {
	target := GetPropString(props, "targetDate", "2025-01-01 00:00:00")
	format := GetPropString(props, "format", "D days, HH:MM:SS")
	finishedText := GetPropString(props, "finishedText", "Time's up!")

	targetDT, err := time.ParseInLocation("2006-01-02 15:04:05", target, time.Now().Location())
	if err != nil {
		return "Invalid timer target"
	}

	diff := targetDT.Sub(time.Now())
	if diff < 0 {
		return finishedText
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

func (s *PreviewService) fillCustomContent(props map[string]any) string {
	url := GetPropString(props, "url", "")
	if url == "" {
		return "No URL configured"
	}
	result := fetchCustomAPI(url, props)
	prefix := GetPropString(props, "prefix", "")
	suffix := GetPropString(props, "suffix", "")
	return prefix + result + suffix
}

func (s *PreviewService) fillSystemContent(props map[string]any) string {
	return fetchSystemInfo(props)
}

// renderImageElement renders an image element from v2 properties.
func (s *PreviewService) renderImageElement(img *image.RGBA, x, y, w, h int, props map[string]any) {
	src := GetPropString(props, "image", "")
	if src == "" {
		src = GetPropString(props, "src", "")
	}
	if src == "" {
		return
	}

	// Build a minimal StyleData for the existing renderImageRGBA method
	sd := &models.StyleData{
		Image: &src,
	}
	if cropX, ok := props["cropX"]; ok {
		if v, vok := cropX.(float64); vok {
			sd.CropX = &v
		}
	}
	if cropY, ok := props["cropY"]; ok {
		if v, vok := cropY.(float64); vok {
			sd.CropY = &v
		}
	}
	if cropW, ok := props["cropW"]; ok {
		if v, vok := cropW.(float64); vok {
			sd.CropW = &v
		}
	}
	if cropH, ok := props["cropH"]; ok {
		if v, vok := cropH.(float64); vok {
			sd.CropH = &v
		}
	}
	s.renderImageRGBA(img, x, y, w, h, sd)
}

// renderShapeElement renders a shape element with fill and stroke.
func (s *PreviewService) renderShapeElement(img *image.RGBA, x, y, w, h int, props map[string]any, defaultColor color.RGBA) {
	// Read fill color: try "fill" (v2 frontend), then "fillColor" (legacy)
	fillColor := defaultColor
	if c := GetPropString(props, "fill", ""); c != "" && c != "transparent" {
		fillColor = parseHexColor(c)
	} else if c := GetPropString(props, "fillColor", ""); c != "" && c != "transparent" {
		fillColor = parseHexColor(c)
	}
	s.drawFilledRectRGBA(img, x, y, w, h, fillColor)

	// Stroke
	strokeColor := GetPropString(props, "stroke", "")
	strokeWidth := GetPropInt(props, "strokeWidth", 0)
	if strokeColor != "" && strokeColor != "transparent" && strokeWidth > 0 {
		sc := parseHexColor(strokeColor)
		s.drawStrokeRectRGBA(img, x, y, w, h, strokeWidth, sc)
	}
}

// drawStrokeRectRGBA draws a rectangular stroke outline.
func (s *PreviewService) drawStrokeRectRGBA(img *image.RGBA, x, y, w, h, strokeWidth int, c color.RGBA) {
	uni := image.NewUniform(c)
	// Top edge
	draw.Draw(img, image.Rect(x, y, x+w, y+strokeWidth).Intersect(img.Bounds()), uni, image.Point{}, draw.Src)
	// Bottom edge
	draw.Draw(img, image.Rect(x, y+h-strokeWidth, x+w, y+h).Intersect(img.Bounds()), uni, image.Point{}, draw.Src)
	// Left edge
	draw.Draw(img, image.Rect(x, y, x+strokeWidth, y+h).Intersect(img.Bounds()), uni, image.Point{}, draw.Src)
	// Right edge
	draw.Draw(img, image.Rect(x+w-strokeWidth, y, x+w, y+h).Intersect(img.Bounds()), uni, image.Point{}, draw.Src)
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
	if len(hex) < 7 || hex[0] != '#' {
		return color.RGBA{0, 0, 0, 255}
	}
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

// --- Content filling helpers (legacy, used by v2 fill methods) ---

// fetchCalendarContent fetches iCal events and formats them.
func fetchCalendarContent(calURL string, maxEvents int) string {
	if calURL == "" {
		return "No events"
	}

	// Convert webcal:// to https://
	if strings.HasPrefix(calURL, "webcal://") {
		calURL = "https://" + calURL[len("webcal://"):]
	}

	client := &defaultHTTPClient
	resp, err := client.Get(calURL)
	if err != nil {
		slog.Error("failed to fetch calendar", "error", err)
		return "No events"
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "No events"
	}

	body, err := readLimitedBody(resp.Body, 1<<20) // 1MB limit
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
	if fontName != nil && *fontName != "" {
		fontPath, err := s.image.GetFontPath(*fontName)
		if err == nil && fontPath != "" {
			if face := s.loadTTFFace(fontPath, size); face != nil {
				return face
			}
		}
	}

	defaultPaths := []string{
		"/usr/share/fonts/noto/NotoSans-Regular.ttf",
		"/usr/share/fonts/noto/NotoSans-Bold.ttf",
		"/usr/share/fonts/truetype/noto/NotoSans-Regular.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/TTF/DejaVuSans-Bold.ttf",
	}
	for _, p := range defaultPaths {
		if face := s.loadTTFFace(p, size); face != nil {
			return face
		}
	}

	// Go built-in vector font (scales properly to any size)
	if face := s.loadGoFont(size); face != nil {
		return face
	}

	slog.Warn("no fonts available, using basic fallback", "checked_paths", len(defaultPaths))
	return newBasicFace(size)
}

// loadGoFont returns Go's built-in regular font at the given size.
func (s *PreviewService) loadGoFont(size int) font.Face {
	key := fontCacheKey{path: "__goregular__", size: size}

	s.fontMu.RLock()
	if f, ok := s.fontCache[key]; ok {
		s.fontMu.RUnlock()
		return f
	}
	s.fontMu.RUnlock()

	f, err := opentype.Parse(goregular.TTF)
	if err != nil {
		slog.Error("failed to parse Go built-in font", "error", err)
		return nil
	}

	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    float64(size),
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		slog.Error("failed to create Go font face", "error", err)
		return nil
	}

	s.fontMu.Lock()
	if len(s.fontCache) >= maxFontCacheEntries {
		for k := range s.fontCache {
			delete(s.fontCache, k)
			break
		}
	}
	s.fontCache[key] = face
	s.fontMu.Unlock()

	slog.Info("using Go built-in font", "size", size)
	return face
}

// loadTTFFace loads a TrueType font file and returns a font.Face, using cache.
func (s *PreviewService) loadTTFFace(path string, size int) font.Face {
	key := fontCacheKey{path: path, size: size}

	s.fontMu.RLock()
	if f, ok := s.fontCache[key]; ok {
		s.fontMu.RUnlock()
		return f
	}
	s.fontMu.RUnlock()

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

	s.fontMu.Lock()
	if len(s.fontCache) >= maxFontCacheEntries {
		// Evict a random entry
		for k := range s.fontCache {
			delete(s.fontCache, k)
			break
		}
	}
	s.fontCache[key] = face
	s.fontMu.Unlock()

	return face
}

// --- Basic fallback font ---

// newBasicFace returns Go's built-in 7x13 bitmap font face.
// It renders actual readable glyphs instead of solid blocks.
func newBasicFace(_ int) font.Face {
	return basicfont.Face7x13
}

// --- Text rendering ---

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

	destRect := image.Rect(x, y, x+w, y+h).Intersect(img.Bounds())
	draw.Draw(img, destRect, resized, image.Point{X: destRect.Min.X - x, Y: destRect.Min.Y - y}, draw.Over)
}

func cropSubImage(src image.Image, r image.Rectangle) image.Image {
	if si, ok := src.(interface{ SubImage(image.Rectangle) image.Image }); ok {
		return si.SubImage(r)
	}
	dst := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(dst, dst.Bounds(), src, r.Min, draw.Src)
	return dst
}

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

func (s *PreviewService) drawFilledRectRGBA(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	rect := image.Rect(x, y, x+w+1, y+h+1).Intersect(img.Bounds())
	draw.Draw(img, rect, image.NewUniform(c), image.Point{}, draw.Src)
}
