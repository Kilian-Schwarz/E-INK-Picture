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

	xdraw "golang.org/x/image/draw"
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

// supersampleScale returns the render scale factor based on render quality setting.
func (s *PreviewService) supersampleScale() float64 {
	q := s.settings.GetRenderQuality()
	switch q {
	case models.RenderQualityMedium:
		return 1.5
	case models.RenderQualityFast:
		return 1.0
	default: // high
		return 2.0
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

	// Supersampling: render at higher resolution for anti-aliased edges
	scale := s.supersampleScale()
	renderW := int(float64(canvasW) * scale)
	renderH := int(float64(canvasH) * scale)

	// Render to full-color RGBA canvas at supersampled resolution
	img := image.NewRGBA(image.Rect(0, 0, renderW, renderH))

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
		if elem.Visible != nil && !*elem.Visible {
			continue
		}

		// Scale element coordinates/dimensions for supersampling
		x := int(elem.X * scale)
		y := int(elem.Y * scale)
		w := int(elem.Width * scale)
		h := int(elem.Height * scale)

		if x+w < 0 || x > renderW || y+h < 0 || y > renderH {
			continue
		}

		props := elem.Properties
		if props == nil {
			props = make(map[string]any)
		}

		// Per-type fontSize defaults matching the frontend designer
		defaultFontSize := 18
		switch elem.Type {
		case "widget_clock":
			defaultFontSize = 48
		case "widget_forecast", "widget_calendar", "widget_news":
			defaultFontSize = 13
		case "widget_system":
			defaultFontSize = 12
		case "widget_custom":
			defaultFontSize = 24
		case "widget_timer":
			defaultFontSize = 24
		case "text":
			defaultFontSize = 24
		}
		fontSize := int(float64(GetPropInt(props, "fontSize", defaultFontSize)) * scale)
		// Frontend exports fontWeight:"bold" and fontStyle:"italic" as strings
		bold := GetPropString(props, "fontWeight", "normal") == "bold" || GetPropBool(props, "bold", false)
		italic := GetPropString(props, "fontStyle", "normal") == "italic" || GetPropBool(props, "italic", false)
		strike := GetPropBool(props, "strikethrough", false)
		align := GetPropString(props, "textAlign", "left")
		vAlign := GetPropString(props, "verticalAlign", "top")
		colorStr := GetPropString(props, "color", "#000000")
		textColor := parseHexColor(colorStr)
		fontFamily := GetPropString(props, "fontFamily", "")
		var fontPtr *string
		if fontFamily != "" {
			fontPtr = &fontFamily
		}
		face := s.loadFontFace(fontPtr, fontSize)

		// Widget padding for consistent positioning with designer (scaled)
		px, py := 0, 0
		if strings.HasPrefix(elem.Type, "widget_") {
			px = int(8 * scale)
			py = int(4 * scale)
		}

		switch elem.Type {
		case "text", "i-text", "textbox":
			content := s.fillTextContent(props)
			s.renderTextV(img, x, y, w, h, content, face, fontSize, bold, italic, strike, align, vAlign, textColor)

		case "image":
			s.renderImageElement(img, x, y, w, h, props)

		case "shape":
			s.renderShapeElement(img, x, y, w, h, props, textColor)

		case "widget_clock":
			content := s.fillClockContent(props)
			s.renderTextV(img, x+px, y+py, w-2*px, h-2*py, content, face, fontSize, bold, italic, strike, align, vAlign, textColor)

		case "widget_weather":
			content := s.fillWeatherContent(props)
			s.renderTextV(img, x+px, y+py, w-2*px, h-2*py, content, face, fontSize, bold, italic, strike, align, vAlign, textColor)

		case "widget_forecast":
			content := s.fillForecastContent(props)
			s.renderTextV(img, x+px, y+py, w-2*px, h-2*py, content, face, fontSize, bold, italic, strike, align, vAlign, textColor)

		case "widget_calendar":
			content := s.fillCalendarContent(props)
			s.renderTextV(img, x+px, y+py, w-2*px, h-2*py, content, face, fontSize, bold, italic, strike, align, vAlign, textColor)

		case "widget_news":
			content := s.fillNewsContent(props)
			s.renderTextV(img, x+px, y+py, w-2*px, h-2*py, content, face, fontSize, bold, italic, strike, align, vAlign, textColor)

		case "widget_timer":
			content := s.fillTimerContent(props)
			s.renderTextV(img, x+px, y+py, w-2*px, h-2*py, content, face, fontSize, bold, italic, strike, align, vAlign, textColor)

		case "widget_custom":
			content := s.fillCustomContent(props)
			s.renderTextV(img, x+px, y+py, w-2*px, h-2*py, content, face, fontSize, bold, italic, strike, align, vAlign, textColor)

		case "widget_system":
			content := s.fillSystemContent(props)
			s.renderTextV(img, x+px, y+py, w-2*px, h-2*py, content, face, fontSize, bold, italic, strike, align, vAlign, textColor)
		}
	}

	// Downscale supersampled image to target resolution with CatmullRom anti-aliasing
	var finalImg image.Image = img
	if scale > 1.0 {
		dst := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), xdraw.Over, nil)
		finalImg = dst
	}

	// Quantize to display palette (unless raw mode)
	var output image.Image
	if raw {
		output = finalImg
	} else {
		output = quantizeToPalette(finalImg, displayCfg.Colors)
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
	layout := GetPropString(props, "layout", "digital_large")
	tz := GetPropString(props, "timezone", "")

	loc := time.Now().Location()
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	now := time.Now().In(loc)

	switch layout {
	case "digital_with_seconds":
		return now.Format("15:04:05")
	case "digital_with_date":
		return now.Format("15:04") + "\n" + formatGermanDate(now)
	case "date_only":
		return now.Format("02.01.2006")
	case "full":
		return formatGermanDate(now) + " — " + now.Format("15:04") + " Uhr"
	case "custom":
		template := GetPropString(props, "customTemplate", "%HH%:%MM%")
		return applyClockPlaceholders(template, now)
	default: // digital_large
		// Also support legacy format property
		format := GetPropString(props, "format", "")
		if format != "" && format != "HH:mm" {
			r := strings.NewReplacer(
				"YYYY", "2006", "MM", "01", "DD", "02",
				"HH", "15", "mm", "04", "ss", "05",
			)
			return now.Format(r.Replace(format))
		}
		return now.Format("15:04")
	}
}

func formatGermanDate(t time.Time) string {
	weekdays := []string{"Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"}
	months := []string{"", "Januar", "Februar", "März", "April", "Mai", "Juni",
		"Juli", "August", "September", "Oktober", "November", "Dezember"}
	return fmt.Sprintf("%s, %d. %s %d", weekdays[t.Weekday()], t.Day(), months[t.Month()], t.Year())
}

func applyClockPlaceholders(template string, t time.Time) string {
	weekdays := []string{"Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"}
	weekdaysShort := []string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"}
	months := []string{"", "Januar", "Februar", "März", "April", "Mai", "Juni",
		"Juli", "August", "September", "Oktober", "November", "Dezember"}

	h12 := t.Hour() % 12
	if h12 == 0 {
		h12 = 12
	}
	ampm := "AM"
	if t.Hour() >= 12 {
		ampm = "PM"
	}

	r := strings.NewReplacer(
		"%HH%", fmt.Sprintf("%02d", t.Hour()),
		"%hh%", fmt.Sprintf("%02d", h12),
		"%MM%", fmt.Sprintf("%02d", t.Minute()),
		"%SS%", fmt.Sprintf("%02d", t.Second()),
		"%dd%", fmt.Sprintf("%02d", t.Day()),
		"%mm%", fmt.Sprintf("%02d", int(t.Month())),
		"%yyyy%", fmt.Sprintf("%d", t.Year()),
		"%WEEKDAY%", weekdays[t.Weekday()],
		"%WEEKDAY_SHORT%", weekdaysShort[t.Weekday()],
		"%MONTH_NAME%", months[t.Month()],
		"%AMPM%", ampm,
	)
	return r.Replace(template)
}

func (s *PreviewService) fillWeatherContent(props map[string]any) string {
	lat := GetPropString(props, "latitude", "52.52")
	lon := GetPropString(props, "longitude", "13.41")
	layout := GetPropString(props, "layout", "")
	// Backward compat: fall back to "style" if no layout set
	if layout == "" {
		layout = GetPropString(props, "style", "compact")
	}

	wdata, err := s.weather.FetchForLocation(lat, lon)
	if err != nil || wdata == nil {
		return "No data"
	}

	switch layout {
	case "standard":
		return fmt.Sprintf("%.0f°C\n%s", wdata.CurrentTemp, wdata.CurrentDesc)
	case "detailed":
		return fmt.Sprintf("%.0f°C %s\nHumidity: --%%\nWind: -- km/h", wdata.CurrentTemp, wdata.CurrentDesc)
	case "minimal":
		return fmt.Sprintf("%.0f°", wdata.CurrentTemp)
	case "custom":
		template := GetPropString(props, "customTemplate", "%temperature%°C")
		return applyWeatherPlaceholders(template, wdata)
	default: // compact
		return fmt.Sprintf("%.0f°C %s", wdata.CurrentTemp, wdata.CurrentDesc)
	}
}

func applyWeatherPlaceholders(template string, data *WeatherData) string {
	tempMin, tempMax := "--", "--"
	desc := data.CurrentDesc
	if len(data.Daily) > 0 {
		tempMin = fmt.Sprintf("%.0f", data.Daily[0].Min)
		tempMax = fmt.Sprintf("%.0f", data.Daily[0].Max)
	}
	r := strings.NewReplacer(
		"%temperature%", fmt.Sprintf("%.0f", data.CurrentTemp),
		"%feels_like%", fmt.Sprintf("%.0f", data.CurrentTemp),
		"%description%", desc,
		"%humidity%", "--",
		"%wind_speed%", "--",
		"%icon%", data.CurrentIcon,
		"%temp_min%", tempMin,
		"%temp_max%", tempMax,
	)
	return r.Replace(template)
}

func (s *PreviewService) fillForecastContent(props map[string]any) string {
	lat := GetPropString(props, "latitude", "52.52")
	lon := GetPropString(props, "longitude", "13.41")
	days := GetPropInt(props, "days", 3)
	layout := GetPropString(props, "layout", "vertical")

	wdata, err := s.weather.FetchForLocation(lat, lon)
	if err != nil || wdata == nil {
		return "No forecast data"
	}

	var lines []string
	for i, day := range wdata.Daily {
		if i >= days {
			break
		}
		switch layout {
		case "compact_row":
			lines = append(lines, fmt.Sprintf("%s %d/%d°", day.Weekday[:3], int(day.Min), int(day.Max)))
		case "detailed_list":
			lines = append(lines, fmt.Sprintf("%s: %d°/%d° %s", day.Weekday, int(day.Min), int(day.Max), day.Desc))
		case "custom":
			template := GetPropString(props, "customTemplate", "%day_name%: %temp_min%-%temp_max%°C")
			r := strings.NewReplacer(
				"%day_name%", day.Weekday,
				"%temp_min%", fmt.Sprintf("%d", int(day.Min)),
				"%temp_max%", fmt.Sprintf("%d", int(day.Max)),
				"%description%", day.Desc,
			)
			lines = append(lines, r.Replace(template))
		default: // vertical
			lines = append(lines, fmt.Sprintf("%s: %d-%d°C %s",
				day.Weekday, int(day.Min), int(day.Max), day.Desc))
		}
	}
	if len(lines) == 0 {
		return "No forecast data"
	}
	return strings.Join(lines, "\n")
}

func (s *PreviewService) fillCalendarContent(props map[string]any) string {
	calURL := GetPropString(props, "icalUrl", "")
	maxEvents := GetPropInt(props, "maxEvents", 5)
	layout := GetPropString(props, "layout", "list")
	title := GetPropString(props, "title", "")
	daysAhead := GetPropInt(props, "daysAhead", 7)

	if calURL == "" {
		return "No calendar URL"
	}

	if strings.HasPrefix(calURL, "webcal://") {
		calURL = "https://" + calURL[len("webcal://"):]
	}

	if layout == "compact" && maxEvents > 3 {
		maxEvents = 3
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
	body, err := readLimitedBody(resp.Body, 1<<20)
	if err != nil {
		return "No events"
	}

	events := parseICalEvents(string(body), maxEvents)
	if len(events) == 0 {
		return "No events"
	}

	// Filter by daysAhead
	cutoff := time.Now().Add(time.Duration(daysAhead) * 24 * time.Hour)
	var filtered []calEvent
	for _, e := range events {
		if e.Start.Before(cutoff) {
			filtered = append(filtered, e)
		}
	}
	if len(filtered) == 0 {
		return "No events"
	}

	var lines []string
	if title != "" {
		lines = append(lines, title)
	}
	switch layout {
	case "agenda":
		var lastDate string
		for _, e := range filtered {
			dateStr := e.Start.Format("02.01.2006")
			if dateStr != lastDate {
				lines = append(lines, dateStr)
				lastDate = dateStr
			}
			lines = append(lines, "  "+e.Start.Format("15:04")+" "+e.Summary)
		}
	case "compact":
		for _, e := range filtered {
			lines = append(lines, e.Start.Format("15:04")+" "+e.Summary)
		}
	default: // list
		for _, e := range filtered {
			lines = append(lines, fmt.Sprintf("%s - %s", e.Start.Format("2006-01-02 15:04"), e.Summary))
		}
	}
	return strings.Join(lines, "\n")
}

func (s *PreviewService) fillNewsContent(props map[string]any) string {
	feedURL := GetPropString(props, "feedUrl", "")
	maxItems := GetPropInt(props, "maxItems", 5)
	title := GetPropString(props, "title", "")
	layout := GetPropString(props, "layout", "headlines")
	showDesc := GetPropBool(props, "showDescription", false)

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

	switch layout {
	case "summary":
		for _, item := range items {
			desc := item.Description
			if len(desc) > 80 {
				desc = desc[:80] + "..."
			}
			lines = append(lines, "- "+item.Title)
			if desc != "" {
				lines = append(lines, "  "+desc)
			}
		}
	case "single":
		if len(items) > 0 {
			lines = append(lines, items[0].Title)
		}
	default: // headlines
		for _, item := range items {
			if showDesc && item.Description != "" {
				desc := item.Description
				if len(desc) > 80 {
					desc = desc[:80] + "..."
				}
				lines = append(lines, "- "+item.Title+": "+desc)
			} else {
				lines = append(lines, "- "+item.Title)
			}
		}
	}
	return strings.Join(lines, "\n")
}

func (s *PreviewService) fillTimerContent(props map[string]any) string {
	target := GetPropString(props, "targetDate", "2025-01-01 00:00:00")
	layout := GetPropString(props, "layout", "countdown_large")
	finishedText := GetPropString(props, "finishedText", "Time's up!")
	label := GetPropString(props, "label", "")

	// Try multiple date formats
	var targetDT time.Time
	var err error
	for _, fmt := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02T15:04"} {
		targetDT, err = time.ParseInLocation(fmt, target, time.Now().Location())
		if err == nil {
			break
		}
	}
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

	var display string
	switch layout {
	case "countdown_compact":
		display = fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	case "label_above":
		display = fmt.Sprintf("%d days %02d:%02d:%02d", days, hours, minutes, seconds)
		if label != "" {
			display = label + "\n" + display
		}
		return display
	case "days_only":
		display = fmt.Sprintf("%d days", days)
	case "custom":
		template := GetPropString(props, "customTemplate", "%days% days %hours%:%minutes%:%seconds%")
		r := strings.NewReplacer(
			"%days%", strconv.Itoa(days),
			"%hours%", fmt.Sprintf("%02d", hours),
			"%minutes%", fmt.Sprintf("%02d", minutes),
			"%seconds%", fmt.Sprintf("%02d", seconds),
			"%total_hours%", strconv.Itoa(totalSecs/3600),
			"%label%", label,
		)
		display = r.Replace(template)
	default: // countdown_large
		// Also support legacy format property
		legacyFmt := GetPropString(props, "format", "")
		if legacyFmt != "" && legacyFmt != "days" {
			display = strings.Replace(legacyFmt, "D", strconv.Itoa(days), 1)
			display = strings.Replace(display, "HH", fmt.Sprintf("%02d", hours), 1)
			display = strings.Replace(display, "MM", fmt.Sprintf("%02d", minutes), 1)
			display = strings.Replace(display, "SS", fmt.Sprintf("%02d", seconds), 1)
		} else {
			display = fmt.Sprintf("%d days %02d:%02d:%02d", days, hours, minutes, seconds)
		}
	}

	if label != "" && layout != "label_above" {
		display = label + ": " + display
	}
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
	layout := GetPropString(props, "layout", "vertical")
	if layout == "custom" {
		template := GetPropString(props, "customTemplate", "%cpu% | %memory% | %temperature%")
		return applySystemPlaceholders(template)
	}
	// For horizontal layout, join with " | " instead of newlines
	result := fetchSystemInfo(props)
	if layout == "horizontal" {
		result = strings.ReplaceAll(result, "\n", " | ")
	}
	return result
}

func applySystemPlaceholders(template string) string {
	cpu, mem, temp := "--", "--", "--"
	if loadData, err := os.ReadFile("/proc/loadavg"); err == nil {
		parts := strings.Fields(string(loadData))
		if len(parts) >= 1 {
			cpu = parts[0]
		}
	}
	if memData, err := os.ReadFile("/proc/meminfo"); err == nil {
		memLines := strings.Split(string(memData), "\n")
		var totalKB, availKB int64
		for _, line := range memLines {
			if strings.HasPrefix(line, "MemTotal:") {
				fmt.Sscanf(line, "MemTotal: %d kB", &totalKB)
			} else if strings.HasPrefix(line, "MemAvailable:") {
				fmt.Sscanf(line, "MemAvailable: %d kB", &availKB)
			}
		}
		if totalKB > 0 {
			mem = fmt.Sprintf("%dMB/%dMB", (totalKB-availKB)/1024, totalKB/1024)
		}
	}
	if tempData, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp"); err == nil {
		tempStr := strings.TrimSpace(string(tempData))
		if tempMilliC, err := strconv.ParseInt(tempStr, 10, 64); err == nil {
			temp = fmt.Sprintf("%.1f°C", float64(tempMilliC)/1000.0)
		}
	}
	r := strings.NewReplacer(
		"%cpu%", cpu,
		"%memory%", mem,
		"%temperature%", temp,
	)
	return r.Replace(template)
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

// renderShapeElement renders a shape element with fill, stroke, and rounded corners.
func (s *PreviewService) renderShapeElement(img *image.RGBA, x, y, w, h int, props map[string]any, defaultColor color.RGBA) {
	// Read fill color: try "fill" (v2 frontend), then "fillColor" (legacy)
	fillStr := GetPropString(props, "fill", "")
	if fillStr == "" {
		fillStr = GetPropString(props, "fillColor", "")
	}
	hasFill := fillStr != "" && fillStr != "transparent"

	var fillColor color.RGBA
	if hasFill {
		fillColor = parseHexColor(fillStr)
	} else {
		fillColor = defaultColor
		hasFill = true
	}

	strokeStr := GetPropString(props, "stroke", "")
	sw := GetPropInt(props, "strokeWidth", 0)
	hasStroke := strokeStr != "" && strokeStr != "transparent" && sw > 0

	rx := GetPropInt(props, "rx", 0)

	if hasStroke {
		sc := parseHexColor(strokeStr)
		// Draw outer shape with stroke color
		drawRoundedRectFilled(img, x, y, w, h, rx, sc)
		// Draw inner shape with fill color
		innerRx := rx - sw
		if innerRx < 0 {
			innerRx = 0
		}
		if hasFill {
			drawRoundedRectFilled(img, x+sw, y+sw, w-2*sw, h-2*sw, innerRx, fillColor)
		}
	} else if hasFill {
		drawRoundedRectFilled(img, x, y, w, h, rx, fillColor)
	}
}


// resolveTextColor parses a hex color from style data, defaulting to black.
func resolveTextColor(textColor *string, _ models.DisplayConfig) color.RGBA {
	if textColor != nil && len(*textColor) == 7 && (*textColor)[0] == '#' {
		return parseHexColor(*textColor)
	}
	return color.RGBA{0, 0, 0, 255}
}

// parseHexColor converts "#RRGGBB" or "#RGB" to color.RGBA.
func parseHexColor(hex string) color.RGBA {
	if len(hex) == 0 || hex[0] != '#' {
		return color.RGBA{0, 0, 0, 255}
	}
	var r, g, b uint8
	switch len(hex) {
	case 7: // #RRGGBB
		fmt.Sscanf(hex[1:], "%02x%02x%02x", &r, &g, &b)
	case 4: // #RGB
		fmt.Sscanf(hex[1:], "%1x%1x%1x", &r, &g, &b)
		r, g, b = r*17, g*17, b*17
	default:
		return color.RGBA{0, 0, 0, 255}
	}
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

// RenderActive renders the currently active design with palette quantization.
func (s *PreviewService) RenderActive() ([]byte, error) {
	return s.RenderActiveRaw(false)
}

// RenderActiveRaw renders the currently active design. If raw is true, no palette quantization is applied.
func (s *PreviewService) RenderActiveRaw(raw bool) ([]byte, error) {
	design, err := s.design.GetActive()
	if err != nil {
		return nil, err
	}
	if design == nil {
		return nil, fmt.Errorf("no active design")
	}
	return s.Render(design, raw)
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

func (s *PreviewService) renderText(img *image.RGBA, x, y, w, h int, text string, face font.Face, fontSize int, bold, italic, strike bool, align string, textColor color.RGBA) {
	s.renderTextV(img, x, y, w, h, text, face, fontSize, bold, italic, strike, align, "top", textColor)
}

func (s *PreviewService) renderTextV(img *image.RGBA, x, y, w, h int, text string, face font.Face, fontSize int, bold, italic, strike bool, align, vAlign string, textColor color.RGBA) {
	if text == "" || w <= 0 || h <= 0 {
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
	fSize := (metrics.Ascent + metrics.Descent).Ceil()
	// Match Fabric.js lineHeight: fontSize * 1.16 (default multiplier)
	lineHeight := int(math.Round(float64(fontSize) * 1.16))
	if lineHeight < 1 {
		lineHeight = fSize + 2
	}

	// Render text to a temporary clipped buffer to prevent overflow
	buf := image.NewRGBA(image.Rect(0, 0, w, h))

	// Calculate vertical offset for vertical alignment
	totalTextHeight := len(wrapped) * lineHeight
	if totalTextHeight > h {
		totalTextHeight = h
	}
	iy := 0
	switch vAlign {
	case "middle":
		iy = (h - totalTextHeight) / 2
		if iy < 0 {
			iy = 0
		}
	case "bottom":
		iy = h - totalTextHeight
		if iy < 0 {
			iy = 0
		}
	}

	for _, line := range wrapped {
		if iy >= h {
			break
		}

		lineWidth := measureString(face, line).Ceil()
		var lx int
		switch align {
		case "center":
			lx = (w - lineWidth) / 2
		case "right":
			lx = w - lineWidth
		default:
			lx = 0
		}

		if italic {
			lx++
		}

		dot := fixed.Point26_6{
			X: fixed.I(lx),
			Y: fixed.I(iy) + metrics.Ascent,
		}

		drawer := &font.Drawer{
			Dst:  buf,
			Src:  image.NewUniform(textColor),
			Face: face,
			Dot:  dot,
		}
		drawer.DrawString(line)

		if bold {
			drawer2 := &font.Drawer{
				Dst:  buf,
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
			midY := iy + fSize/2
			if midY >= 0 && midY < h {
				for px := lx; px < lx+lineWidth && px < w; px++ {
					if px >= 0 {
						buf.SetRGBA(px, midY, textColor)
					}
				}
			}
		}

		iy += lineHeight
	}

	// Composite clipped text onto main image
	destRect := image.Rect(x, y, x+w, y+h).Intersect(img.Bounds())
	draw.Draw(img, destRect, buf, image.Point{X: destRect.Min.X - x, Y: destRect.Min.Y - y}, draw.Over)
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
	resized := resizeImage(cropped, w, h)

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

// resizeImage scales an image using high-quality CatmullRom interpolation.
// This produces smooth, sharp results equivalent to Lanczos resampling.
func resizeImage(src image.Image, dstW, dstH int) image.Image {
	srcBounds := src.Bounds()
	if srcBounds.Dx() == 0 || srcBounds.Dy() == 0 || dstW == 0 || dstH == 0 {
		return image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	}

	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, srcBounds, xdraw.Over, nil)
	return dst
}

// --- Shape rendering helpers ---

func (s *PreviewService) drawFilledRectRGBA(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	rect := image.Rect(x, y, x+w, y+h).Intersect(img.Bounds())
	draw.Draw(img, rect, image.NewUniform(c), image.Point{}, draw.Src)
}

// drawRoundedRectFilled draws a filled rectangle with rounded corners.
func drawRoundedRectFilled(img *image.RGBA, x, y, w, h, r int, c color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	if r <= 0 {
		rect := image.Rect(x, y, x+w, y+h).Intersect(img.Bounds())
		draw.Draw(img, rect, image.NewUniform(c), image.Point{}, draw.Src)
		return
	}
	if r > w/2 {
		r = w / 2
	}
	if r > h/2 {
		r = h / 2
	}

	uni := image.NewUniform(c)
	bounds := img.Bounds()

	// Center horizontal strip
	draw.Draw(img, image.Rect(x+r, y, x+w-r, y+h).Intersect(bounds), uni, image.Point{}, draw.Src)
	// Left vertical strip (between corners)
	draw.Draw(img, image.Rect(x, y+r, x+r, y+h-r).Intersect(bounds), uni, image.Point{}, draw.Src)
	// Right vertical strip (between corners)
	draw.Draw(img, image.Rect(x+w-r, y+r, x+w, y+h-r).Intersect(bounds), uni, image.Point{}, draw.Src)

	// Draw four corner arcs
	rr := float64(r)
	for py := 0; py < r; py++ {
		for px := 0; px < r; px++ {
			dx := float64(r-1-px) + 0.5
			dy := float64(r-1-py) + 0.5
			if dx*dx+dy*dy <= rr*rr {
				// Top-left
				ix, iy := x+px, y+py
				if ix >= bounds.Min.X && ix < bounds.Max.X && iy >= bounds.Min.Y && iy < bounds.Max.Y {
					img.SetRGBA(ix, iy, c)
				}
				// Top-right
				ix = x + w - 1 - px
				if ix >= bounds.Min.X && ix < bounds.Max.X && iy >= bounds.Min.Y && iy < bounds.Max.Y {
					img.SetRGBA(ix, iy, c)
				}
				// Bottom-left
				ix = x + px
				iy = y + h - 1 - py
				if ix >= bounds.Min.X && ix < bounds.Max.X && iy >= bounds.Min.Y && iy < bounds.Max.Y {
					img.SetRGBA(ix, iy, c)
				}
				// Bottom-right
				ix = x + w - 1 - px
				if ix >= bounds.Min.X && ix < bounds.Max.X && iy >= bounds.Min.Y && iy < bounds.Max.Y {
					img.SetRGBA(ix, iy, c)
				}
			}
		}
	}
}
