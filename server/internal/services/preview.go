package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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

// PreviewService renders design previews as 1-bit monochrome PNGs.
type PreviewService struct {
	design  *DesignService
	weather *WeatherService
	image   *ImageService
	dataDir string
}

// NewPreviewService creates a PreviewService with access to other services.
func NewPreviewService(d *DesignService, w *WeatherService, i *ImageService, dataDir string) *PreviewService {
	return &PreviewService{design: d, weather: w, image: i, dataDir: dataDir}
}

// Render fills dynamic content and renders a design to a 1-bit PNG.
func (s *PreviewService) Render(design *models.Design) ([]byte, error) {
	s.FillContent(design)

	palette := color.Palette{color.White, color.Black}
	img := image.NewPaletted(image.Rect(0, 0, einkWidth, einkHeight), palette)
	// Fill with white (index 0)
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)

	for i := range design.Modules {
		m := &design.Modules[i]
		x := m.Position.X - einkOffsetX
		y := m.Position.Y - einkOffsetY
		w := m.Size.Width
		h := m.Size.Height

		// Skip if outside frame
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

		face := s.loadFontFace(sd.Font, fontSize)

		switch m.Type {
		case "text", "news", "weather", "datetime", "timer", "calendar":
			s.renderText(img, x, y, w, h, m.Content, face, bold, italic, strike, align)
		case "image":
			s.renderImage(img, x, y, w, h, &sd)
		case "line":
			s.drawFilledRect(img, x, y, w, h)
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return buf.Bytes(), nil
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
	return s.Render(design)
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
			wdata, err := fetchWeather(lat, lon)
			if err != nil || wdata == nil {
				m.Content = "No data"
			} else {
				ws := "default"
				if sd.WeatherStyle != nil {
					ws = *sd.WeatherStyle
				}
				stylesDir := filepath.Join(s.dataDir, "weather_styles")
				m.Content = applyWeatherStyle(ws, wdata, stylesDir)
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
func formatDateTime(fmt string) string {
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
	goFmt := r.Replace(fmt)
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

// weatherData holds parsed weather information from open-meteo API.
type weatherData struct {
	CurrentTemp float64         `json:"current_temp"`
	CurrentDesc string          `json:"current_desc"`
	Daily       []weatherDaily  `json:"daily"`
	Hourly      []weatherHourly `json:"hourly"`
}

type weatherDaily struct {
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Desc    string  `json:"desc"`
	Weekday string  `json:"weekday"`
}

type weatherHourly struct {
	Time   string  `json:"time"`
	Temp   float64 `json:"temp"`
	Desc   string  `json:"desc"`
	Precip float64 `json:"precip"`
}

// weathercodeToDesc maps WMO weather codes to descriptions.
func weathercodeToDesc(code int) string {
	m := map[int]string{
		0: "Clear sky", 1: "Mainly clear", 2: "Partly cloudy", 3: "Overcast",
		45: "Fog", 48: "Rime fog", 51: "Light drizzle",
		61: "Slight rain", 63: "Moderate rain", 65: "Heavy rain", 80: "Rain showers",
	}
	if d, ok := m[code]; ok {
		return d
	}
	return "Unknown"
}

// fetchWeather fetches current weather from open-meteo API.
func fetchWeather(lat, lon string) (*weatherData, error) {
	apiURL := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%s&longitude=%s&hourly=temperature_2m,weathercode,precipitation&daily=weathercode,temperature_2m_max,temperature_2m_min,sunrise,sunset&current_weather=true&forecast_days=4&timezone=Europe%%2FBerlin",
		url.QueryEscape(lat), url.QueryEscape(lon),
	)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weather API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var raw struct {
		CurrentWeather struct {
			Temperature float64 `json:"temperature"`
			Weathercode int     `json:"weathercode"`
		} `json:"current_weather"`
		Daily struct {
			Weathercode []int     `json:"weathercode"`
			TempMax     []float64 `json:"temperature_2m_max"`
			TempMin     []float64 `json:"temperature_2m_min"`
			Time        []string  `json:"time"`
		} `json:"daily"`
		Hourly struct {
			Time          []string  `json:"time"`
			Temperature2m []float64 `json:"temperature_2m"`
			Weathercode   []int     `json:"weathercode"`
			Precipitation []float64 `json:"precipitation"`
		} `json:"hourly"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	wd := &weatherData{
		CurrentTemp: raw.CurrentWeather.Temperature,
		CurrentDesc: weathercodeToDesc(raw.CurrentWeather.Weathercode),
	}

	// Parse daily forecast
	for i := 0; i < len(raw.Daily.Time) && i < 4; i++ {
		t, _ := time.Parse("2006-01-02", raw.Daily.Time[i])
		wd.Daily = append(wd.Daily, weatherDaily{
			Min:     raw.Daily.TempMin[i],
			Max:     raw.Daily.TempMax[i],
			Desc:    weathercodeToDesc(raw.Daily.Weathercode[i]),
			Weekday: t.Weekday().String(),
		})
	}

	// Parse hourly (every 2 hours like Python)
	for i := 0; i < len(raw.Hourly.Time); i += 2 {
		htime := ""
		if len(raw.Hourly.Time[i]) > 15 {
			htime = raw.Hourly.Time[i][11:16]
		}
		wd.Hourly = append(wd.Hourly, weatherHourly{
			Time:   htime,
			Temp:   raw.Hourly.Temperature2m[i],
			Desc:   weathercodeToDesc(raw.Hourly.Weathercode[i]),
			Precip: raw.Hourly.Precipitation[i],
		})
	}

	return wd, nil
}

// weatherStyleTemplate represents a weather style JSON file.
type weatherStyleTemplate struct {
	Format string `json:"format"`
}

// applyWeatherStyle applies a named weather style template to weather data.
func applyWeatherStyle(style string, wdata *weatherData, stylesDir string) string {
	styleFile := filepath.Join(stylesDir, style+".json")
	data, err := os.ReadFile(styleFile)
	if err != nil {
		return "No data"
	}

	var tmpl weatherStyleTemplate
	if err := json.Unmarshal(data, &tmpl); err != nil {
		return "No data"
	}

	text := tmpl.Format
	if text == "" {
		text = "No format"
	}

	text = strings.ReplaceAll(text, "{current_temp}", fmt.Sprintf("%.1f", wdata.CurrentTemp))
	text = strings.ReplaceAll(text, "{current_desc}", wdata.CurrentDesc)

	var dfLines []string
	for _, day := range wdata.Daily {
		line := fmt.Sprintf("%s: %d-%d\u00b0C %s", day.Weekday, int(day.Min), int(day.Max), day.Desc)
		dfLines = append(dfLines, line)
	}
	text = strings.ReplaceAll(text, "{daily_forecast}", strings.Join(dfLines, "\n"))

	return text
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
		log.Printf("Error fetching calendar: %v", err)
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
	for i := 0; i < len(events); i++ {
		for j := i + 1; j < len(events); j++ {
			if events[j].Start.Before(events[i].Start) {
				events[i], events[j] = events[j], events[i]
			}
		}
	}

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

// renderText draws word-wrapped text on a paletted image, matching Python behavior.
func (s *PreviewService) renderText(img *image.Paletted, x, y, w, h int, text string, face font.Face, bold, italic, strike bool, align string) {
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
			Src:  image.NewUniform(color.Black),
			Face: face,
			Dot:  dot,
		}
		drawer.DrawString(line)

		// Bold: draw again shifted 1px right
		if bold {
			drawer2 := &font.Drawer{
				Dst:  img,
				Src:  image.NewUniform(color.Black),
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
					img.SetColorIndex(px, midY, 1) // 1 = black in our palette
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

// renderImage loads, crops, resizes and pastes an image module.
func (s *PreviewService) renderImage(img *image.Paletted, x, y, w, h int, sd *models.StyleData) {
	if sd.Image == nil || *sd.Image == "" {
		return
	}

	imgPath, err := s.image.GetImagePath(*sd.Image)
	if err != nil || imgPath == "" {
		// Fallback: try direct path
		imgPath = filepath.Join(s.dataDir, "uploaded_images", *sd.Image)
	}

	f, err := os.Open(imgPath)
	if err != nil {
		log.Printf("Failed to open image %s: %v", imgPath, err)
		return
	}
	defer f.Close()

	srcImg, _, err := image.Decode(f)
	if err != nil {
		log.Printf("Failed to decode image %s: %v", imgPath, err)
		return
	}

	bounds := srcImg.Bounds()

	// Crop
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

	// Crop by creating a sub-image
	cropRect := image.Rect(cx, cy, cx+cw, cy+ch).Intersect(bounds)
	cropped := cropSubImage(srcImg, cropRect)

	// Resize to module dimensions using nearest-neighbor
	resized := resizeNearest(cropped, w, h)

	// Convert to 1-bit and paste
	for py := 0; py < h; py++ {
		for px := 0; px < w; px++ {
			destX := x + px
			destY := y + py
			if destX < 0 || destX >= einkWidth || destY < 0 || destY >= einkHeight {
				continue
			}
			r, g, b, _ := resized.At(px, py).RGBA()
			// Convert to grayscale and threshold
			gray := (r*299 + g*587 + b*114) / 1000
			if gray < 0x8000 {
				img.SetColorIndex(destX, destY, 1) // black
			} else {
				img.SetColorIndex(destX, destY, 0) // white
			}
		}
	}
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

// drawFilledRect draws a filled black rectangle (for line modules).
func (s *PreviewService) drawFilledRect(img *image.Paletted, x, y, w, h int) {
	for py := y; py <= y+h; py++ {
		for px := x; px <= x+w; px++ {
			if px >= 0 && px < einkWidth && py >= 0 && py < einkHeight {
				img.SetColorIndex(px, py, 1) // black
			}
		}
	}
}
