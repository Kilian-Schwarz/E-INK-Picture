package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"e-ink-picture/server/internal/models"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/f64"
	"golang.org/x/image/math/fixed"
)

const (
	einkOffsetX = 200
	einkOffsetY = 160
	einkWidth   = 800
	einkHeight  = 480
)

const maxFontCacheEntries = 10

// goFontCacheKey is the font cache key for the embedded Go regular font.
const goFontCacheKey = "__goregular__"

// maxScalerCacheEntries bounds the downscale scaler cache. The standard
// 800x480 canvas produces at most two distinct downscale geometries (high
// 2.0x, medium 1.5x; fast has no downscale) — three entries leave one slot of
// slack. Further geometries (design-driven canvas sizes) fall back to the
// unpooled xdraw.CatmullRom.Scale so the cache can never grow unboundedly.
const maxScalerCacheEntries = 3

// scalerKey identifies a fixed downscale geometry (destination and source
// width/height), matching the contract of draw.Kernel.NewScaler.
type scalerKey struct {
	dw, dh, sw, sh int
}

// PreviewService renders design previews as PNGs with display-appropriate palette.
type PreviewService struct {
	design   *DesignService
	weather  *WeatherService
	image    *ImageService
	settings *SettingsService
	dataDir  string

	// hass is the read-only Home-Assistant fetch layer (B5a). It is optional:
	// fillHassContent tolerates a nil hass (returns germanHassNotConfigured) so
	// existing PreviewService constructions that never wire it keep working.
	// main injects it via SetHassService after both services are built.
	hass *HassService

	// fontCache holds PARSED fonts keyed by font path. It must never cache
	// font.Face instances: opentype.Face is not safe for concurrent use
	// (lazy metrics, sfnt.Buffer, rasterizer state mutate per glyph), while
	// the parsed *opentype.Font is read-only and safe to share. Faces are
	// created fresh per loadGoFont/loadTTFFace call (cheap; opentype.Parse
	// is the expensive part).
	fontMu    sync.RWMutex
	fontCache map[string]*opentype.Font

	// renderSem is the global render semaphore: at most cap(renderSem)
	// renders run concurrently, waiters select on ctx.Done(). renderActive
	// counts renders inside the gate (test observability, AC1).
	renderSem    chan struct{}
	renderActive atomic.Int32

	scalerMu    sync.Mutex
	scalerCache map[scalerKey]xdraw.Scaler

	// now is the injectable wall clock. It exists so time-dependent widget
	// content is deterministically testable (F7). It is currently used ONLY by
	// fillProgressContent; every other fill*Content still calls time.Now()
	// directly. Always read it through nowOrDefault, never directly: a
	// zero-value PreviewService (or one built without NewPreviewService) has a
	// nil now and would panic in the render path.
	now func() time.Time
}

// nowOrDefault returns the injected clock, falling back to time.Now when the
// service was built without NewPreviewService (zero value).
func (s *PreviewService) nowOrDefault() time.Time {
	if s.now == nil {
		return time.Now()
	}
	return s.now()
}

// NewPreviewService creates a PreviewService with access to other services.
// The render concurrency limit defaults to 1; main overrides it via
// SetMaxConcurrentRenders from EINK_MAX_CONCURRENT_RENDERS.
func NewPreviewService(d *DesignService, w *WeatherService, i *ImageService, s *SettingsService, dataDir string) *PreviewService {
	return &PreviewService{
		design:      d,
		weather:     w,
		image:       i,
		settings:    s,
		dataDir:     dataDir,
		fontCache:   make(map[string]*opentype.Font),
		renderSem:   make(chan struct{}, 1),
		scalerCache: make(map[scalerKey]xdraw.Scaler),
		now:         time.Now,
	}
}

// SetHassService injects the read-only Home-Assistant fetch layer used by
// fillHassContent (widget_hass). It is optional and set once at startup (main),
// mirroring SetMaxConcurrentRenders: a PreviewService without it renders every
// widget_hass element as germanHassNotConfigured.
func (s *PreviewService) SetHassService(h *HassService) {
	s.hass = h
}

// SetMaxConcurrentRenders resizes the render semaphore. Must be called before
// the service starts serving renders (main does, right after construction);
// values < 1 are clamped to 1.
func (s *PreviewService) SetMaxConcurrentRenders(n int) {
	if n < 1 {
		n = 1
	}
	s.renderSem = make(chan struct{}, n)
}

// ActiveRenders reports how many renders currently hold the semaphore
// (observability for the AC1 serialization test).
func (s *PreviewService) ActiveRenders() int32 {
	return s.renderActive.Load()
}

// downscaleScaler returns a cached, reusable CatmullRom scaler for the given
// geometry. Kernel.NewScaler pre-computes the distribution weights and pools
// the ~24.6 MB temp buffer via sync.Pool (emptied under GC pressure, so idle
// RSS is unaffected) while producing byte-identical results to Kernel.Scale.
// Returns nil when the bounded cache is full and the key is unknown — the
// caller then falls back to the unpooled xdraw.CatmullRom.Scale.
func (s *PreviewService) downscaleScaler(dw, dh, sw, sh int) xdraw.Scaler {
	key := scalerKey{dw: dw, dh: dh, sw: sw, sh: sh}
	s.scalerMu.Lock()
	defer s.scalerMu.Unlock()
	if sc, ok := s.scalerCache[key]; ok {
		return sc
	}
	if len(s.scalerCache) >= maxScalerCacheEntries {
		return nil
	}
	sc := xdraw.CatmullRom.NewScaler(dw, dh, sw, sh)
	s.scalerCache[key] = sc
	return sc
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
//
// The whole render body (including synchronous widget fetches, see E4.4) runs
// inside the global render semaphore; queued callers abort with ctx.Err()
// when ctx is canceled (e.g. client disconnect or http.Server WriteTimeout).
func (s *PreviewService) Render(ctx context.Context, design *models.DesignV2, raw bool) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Capture the channel locally so the deferred release is guaranteed to
	// hit the same semaphore even if SetMaxConcurrentRenders were (contrary
	// to its contract) called mid-flight.
	sem := s.renderSem
	waitStart := time.Now()
	select {
	case sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if wait := time.Since(waitStart); wait > 5*time.Second {
		slog.Warn("render queue wait exceeded 5s", "wait", wait.Round(time.Millisecond))
	}
	s.renderActive.Add(1)
	defer func() {
		s.renderActive.Add(-1)
		<-sem
	}()

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

		// Culling: unrotated box for rotation 0, rotated AABB otherwise
		rot := normalizeRotation(elem.Rotation)
		if rot == 0 {
			if x+w < 0 || x > renderW || y+h < 0 || y > renderH {
				continue
			}
		} else {
			minX, minY, maxX, maxY := rotatedAABB(x, y, w, h, rot)
			if maxX < 0 || minX > float64(renderW) || maxY < 0 || minY > float64(renderH) {
				continue
			}
		}

		props := elem.Properties
		if props == nil {
			props = make(map[string]any)
		}

		// Per-type fontSize defaults matching the frontend designer
		fontSize := int(float64(GetPropInt(props, "fontSize", defaultFontSizeFor(elem.Type))) * scale)
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

		if rot == 0 {
			// Direct path: draw straight into the supersampled canvas
			s.drawElement(img, elem.Type, props, x, y, w, h, px, py, face, fontSize, bold, italic, strike, align, vAlign, textColor, scale)
		} else if w > 0 && h > 0 {
			// Rotated path: draw into a transparent offscreen buffer at origin
			// (0,0), then composite it into the supersampled canvas with an
			// affine rotation around the top-left anchor (x, y). This happens
			// BEFORE downscale/quantization, so anti-aliased edges get dithered.
			offscreen := image.NewRGBA(image.Rect(0, 0, w, h))
			s.drawElement(offscreen, elem.Type, props, 0, 0, w, h, px, py, face, fontSize, bold, italic, strike, align, vAlign, textColor, scale)

			cos, sin := rotationCoeffs(rot)
			aff := f64.Aff3{cos, -sin, float64(x), sin, cos, float64(y)}
			xdraw.CatmullRom.Transform(img, aff, offscreen, offscreen.Bounds(), xdraw.Over, nil)
		}
	}

	// Downscale supersampled image to target resolution with CatmullRom
	// anti-aliasing. The cached scaler (Kernel.NewScaler) is byte-identical
	// to Kernel.Scale but pools the large temp buffer across renders.
	var finalImg image.Image = img
	if scale > 1.0 {
		dst := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))
		if sc := s.downscaleScaler(canvasW, canvasH, renderW, renderH); sc != nil {
			sc.Scale(dst, dst.Bounds(), img, img.Bounds(), xdraw.Over, nil)
		} else {
			xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), xdraw.Over, nil)
		}
		finalImg = dst
	}

	// Quantize to display palette (unless raw mode). The two-stage pipeline
	// in quantize.go dithers against the perceptual panel palette (when
	// calibration is active) and swaps back to the pure driver colors.
	var output image.Image
	if raw {
		output = finalImg
	} else {
		output = quantizeForDisplay(finalImg, displayCfg, s.settings.GetDitherAlgorithm(), s.settings.GetCalibration())
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, output); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return buf.Bytes(), nil
}

// widgetDefaultFontSizes maps an element type to the fontSize used when the
// element carries no explicit fontSize property.
//
// This table is duplicated in the frontend designer
// (static/js/widgets.js, getPreviewFontSize). Both sides must agree, otherwise
// the canvas preview and the rendered panel disagree on glyph size for the same
// element. TestWidgetDefaultFontSizesMatchFrontend pins them together — extend
// BOTH tables when adding a widget type.
var widgetDefaultFontSizes = map[string]int{
	"text":            24,
	"widget_clock":    48,
	"widget_weather":  18,
	"widget_forecast": 13,
	"widget_calendar": 13,
	"widget_news":     13,
	"widget_system":   12,
	"widget_custom":   24,
	"widget_timer":    24,
	"widget_hass":     18,
	"widget_progress": 18,
	"widget_holidays": 13,
}

// widgetFallbackFontSize applies to element types absent from the table above.
const widgetFallbackFontSize = 18

// defaultFontSizeFor returns the default fontSize for an element type.
func defaultFontSizeFor(elemType string) int {
	if size, ok := widgetDefaultFontSizes[elemType]; ok {
		return size
	}
	return widgetFallbackFontSize
}

// WidgetTextContent returns the text content for a text/widget element type
// using the same fill*Content logic drawElement draws. It is the single,
// exported dispatch entry point shared by the panel renderer (drawElement) and
// the /api/widget_content endpoint, so both derive content from one source.
// ok is false for element types without server-side text content (image,
// shape, unknown), letting callers distinguish an empty content string from a
// type that has no text content at all.
func (s *PreviewService) WidgetTextContent(elemType string, props map[string]any) (content string, ok bool) {
	switch elemType {
	case "text", "i-text", "textbox":
		return s.fillTextContent(props), true
	case "widget_clock":
		return s.fillClockContent(props), true
	case "widget_weather":
		return s.fillWeatherContent(props), true
	case "widget_forecast":
		return s.fillForecastContent(props), true
	case "widget_calendar":
		return s.fillCalendarContent(props), true
	case "widget_news":
		return s.fillNewsContent(props), true
	case "widget_timer":
		return s.fillTimerContent(props), true
	case "widget_custom":
		return s.fillCustomContent(props), true
	case "widget_system":
		return s.fillSystemContent(props), true
	case "widget_hass":
		return s.fillHassContent(props), true
	case "widget_progress":
		return s.fillProgressContent(props), true
	case "widget_holidays":
		return s.fillHolidaysContent(props), true
	default:
		return "", false
	}
}

// drawElement draws a single element of the given type into dst with its
// top-left anchor at (x, y). w and h are the scaled, unrotated element
// dimensions; px and py the scaled widget padding. scale is the supersample
// factor so scalar geometry (corner radii, stroke width) can be scaled to the
// same coordinate space as x/y/w/h.
//
// Text content comes exclusively from WidgetTextContent (the shared dispatch),
// so the panel and the /api/widget_content endpoint can never fork. image and
// shape have no text content and are drawn directly.
func (s *PreviewService) drawElement(dst *image.RGBA, elemType string, props map[string]any, x, y, w, h, px, py int, face font.Face, fontSize int, bold, italic, strike bool, align, vAlign string, textColor color.RGBA, scale float64) {
	switch elemType {
	case "image":
		s.renderImageElement(dst, x, y, w, h, props)
		return
	case "shape":
		s.renderShapeElement(dst, x, y, w, h, props, textColor, scale)
		return
	}

	content, ok := s.WidgetTextContent(elemType, props)
	if !ok {
		return
	}

	// Plain text elements draw flush to the element box; widget_* elements
	// inset by the widget padding (px, py) to match the designer.
	if elemType == "text" || elemType == "i-text" || elemType == "textbox" {
		s.renderTextV(dst, x, y, w, h, content, face, fontSize, bold, italic, strike, align, vAlign, textColor)
		return
	}
	s.renderTextV(dst, x+px, y+py, w-2*px, h-2*py, content, face, fontSize, bold, italic, strike, align, vAlign, textColor)
}

// --- Rotation helpers ---
//
// Fabric.js semantics (verified in the designer frontend): Element.Rotation is
// in degrees, clockwise, around the element's unrotated top-left anchor (x, y).
// width/height stay unrotated. In image coordinates (y pointing down) a
// clockwise rotation is the standard rotation matrix, mapping an offscreen
// point (u, v) to canvas coordinates:
//
//	dstX = x + u*cos(θ) - v*sin(θ)
//	dstY = y + u*sin(θ) + v*cos(θ)

// normalizeRotation maps a rotation in degrees to the range [0, 360).
func normalizeRotation(deg float64) float64 {
	m := math.Mod(deg, 360)
	if m < 0 {
		m += 360
	}
	return m
}

// rotationCoeffs returns (cos θ, sin θ) for a clockwise rotation of deg
// degrees (deg must be normalized to [0, 360)). Exact multiples of 90° use
// exact {-1, 0, 1} coefficients because math.Cos/Sin leave float dust there
// (e.g. cos(π/2) ≈ 6.1e-17), which would break exact axis parallelism.
func rotationCoeffs(deg float64) (cos, sin float64) {
	switch deg {
	case 90:
		return 0, 1
	case 180:
		return -1, 0
	case 270:
		return 0, -1
	}
	rad := deg * math.Pi / 180
	return math.Cos(rad), math.Sin(rad)
}

// rotatedAABB returns the axis-aligned bounding box of a w×h element rotated
// by deg degrees (normalized) around its top-left anchor (x, y).
func rotatedAABB(x, y, w, h int, deg float64) (minX, minY, maxX, maxY float64) {
	cos, sin := rotationCoeffs(deg)
	fx, fy := float64(x), float64(y)
	corners := [4][2]float64{{0, 0}, {float64(w), 0}, {0, float64(h)}, {float64(w), float64(h)}}
	for i, c := range corners {
		cx := fx + c[0]*cos - c[1]*sin
		cy := fy + c[0]*sin + c[1]*cos
		if i == 0 {
			minX, maxX = cx, cx
			minY, maxY = cy, cy
			continue
		}
		minX = math.Min(minX, cx)
		maxX = math.Max(maxX, cx)
		minY = math.Min(minY, cy)
		maxY = math.Max(maxY, cy)
	}
	return minX, minY, maxX, maxY
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
	return fmt.Sprintf("%s, %d. %s %d", germanWeekdaysFull[t.Weekday()], t.Day(), germanMonths[t.Month()], t.Year())
}

func applyClockPlaceholders(template string, t time.Time) string {
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
		"%WEEKDAY%", germanWeekdaysFull[t.Weekday()],
		"%WEEKDAY_SHORT%", germanWeekdaysShort[t.Weekday()],
		"%MONTH_NAME%", germanMonths[t.Month()],
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
			lines = append(lines, fmt.Sprintf("%s %d/%d°", germanShortWeekdayFromName(day.Weekday), int(day.Min), int(day.Max)))
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

	// Negative cache hit: same return value as a live fetch failure.
	negKey := "url:" + calURL
	if failCache.blocked(negKey) {
		return "No events"
	}

	client := &defaultHTTPClient
	resp, err := client.Get(calURL)
	if err != nil {
		slog.Error("failed to fetch calendar", "error", err)
		failCache.markFailure(negKey)
		return "No events"
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		failCache.markFailure(negKey)
		return "No events"
	}
	failCache.markSuccess(negKey)
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

// fillHassContent renders a widget_hass element's live text from Home-Assistant
// (specs/B5-home-assistant.md, sub-task B5b). It reads the dedicated hassMode
// prop (temperature/alarm/presence — never layout), fetches each configured
// entity read-only via HassService, and maps state to German text sourced
// entirely from locale.go (AC-HA7). It is nil-tolerant (no hass wired →
// germanHassNotConfigured) and never returns the token nor panics.
func (s *PreviewService) fillHassContent(props map[string]any) string {
	entities := splitHassEntities(GetPropString(props, "entityId", ""))
	if len(entities) == 0 {
		return germanHassNoEntity
	}
	if s.hass == nil {
		return germanHassNotConfigured
	}

	label := GetPropString(props, "label", "")
	switch GetPropString(props, "hassMode", "temperature") {
	case "alarm":
		return s.fillHassAlarm(entities, label)
	case "presence":
		return s.fillHassPresence(entities, label)
	default:
		return s.fillHassTemperature(entities, label)
	}
}

// splitHassEntities parses a comma-separated entity list, trimming whitespace
// and dropping empty items.
func splitHassEntities(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if id := strings.TrimSpace(part); id != "" {
			out = append(out, id)
		}
	}
	return out
}

// fetchHassEntity fetches one entity and maps the service error to the German
// graceful string from locale.go. On success text is "" and the entity is
// returned; on failure the entity is nil and text holds the placeholder.
func (s *PreviewService) fetchHassEntity(entityID string) (*HassEntity, string) {
	ent, err := s.hass.FetchEntity(context.Background(), entityID)
	if err == nil {
		return ent, ""
	}
	switch {
	case errors.Is(err, ErrHassNotConfigured):
		return nil, germanHassNotConfigured
	case errors.Is(err, ErrHassEntityUnknown):
		return nil, germanHassUnknownPrefix + entityID
	default:
		return nil, germanHassUnavailable
	}
}

// fillHassTemperature renders sensor.* readings. One entity yields "<state><unit>"
// (or germanHassNoValue for a non-numeric reading), optionally prefixed by label.
// Several entities yield one "<name>: <state><unit>" line each.
func (s *PreviewService) fillHassTemperature(entities []string, label string) string {
	if len(entities) == 1 {
		ent, text := s.fetchHassEntity(entities[0])
		if ent == nil {
			return text
		}
		return withHassLabel(label, formatHassTemp(ent))
	}

	var lines []string
	okCount := 0
	lastFail := ""
	for _, id := range entities {
		ent, text := s.fetchHassEntity(id)
		if ent == nil {
			lastFail = text
			continue
		}
		okCount++
		lines = append(lines, hassDisplayName(ent, id)+": "+formatHassTemp(ent))
	}
	if okCount == 0 {
		return lastFail
	}
	return strings.Join(lines, "\n")
}

// fillHassAlarm renders an alarm_control_panel state as German text, optionally
// prefixed by label. Only the first entity is used (alarm is single-panel).
func (s *PreviewService) fillHassAlarm(entities []string, label string) string {
	ent, text := s.fetchHassEntity(entities[0])
	if ent == nil {
		return text
	}
	return withHassLabel(label, germanAlarmText(ent.State))
}

// fillHassPresence renders person.*/device_tracker.* presence. One entity yields
// the German presence text (optionally label-prefixed). Several entities yield a
// count summary (germanHassHomeCount) followed by each present person's name.
func (s *PreviewService) fillHassPresence(entities []string, label string) string {
	if len(entities) == 1 {
		ent, text := s.fetchHassEntity(entities[0])
		if ent == nil {
			return text
		}
		return withHassLabel(label, germanPresenceText(ent.State))
	}

	homeCount := 0
	okCount := 0
	lastFail := ""
	var names []string
	for _, id := range entities {
		ent, text := s.fetchHassEntity(id)
		if ent == nil {
			lastFail = text
			continue
		}
		okCount++
		if ent.State == "home" {
			homeCount++
			names = append(names, hassDisplayName(ent, id))
		}
	}
	if okCount == 0 {
		return lastFail
	}
	lines := []string{germanHassHomeCount(homeCount)}
	lines = append(lines, names...)
	return strings.Join(lines, "\n")
}

// formatHassTemp renders "<state><unit>" for a numeric reading, or
// germanHassNoValue when the state is not a number (unavailable/unknown).
func formatHassTemp(ent *HassEntity) string {
	if _, err := strconv.ParseFloat(strings.TrimSpace(ent.State), 64); err != nil {
		return germanHassNoValue
	}
	return ent.State + ent.Unit
}

// hassDisplayName returns the entity friendly name, falling back to its id.
func hassDisplayName(ent *HassEntity, id string) string {
	if ent.FriendlyName != "" {
		return ent.FriendlyName
	}
	return id
}

// withHassLabel prefixes text with "<label>: " when label is non-empty.
func withHassLabel(label, text string) string {
	if label == "" {
		return text
	}
	return label + ": " + text
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

// renderShapeElement renders a shape element with fill, stroke, and rounded
// corners, matching Fabric's geometry (B2): corner radii and stroke width are
// scaled by the supersample factor so they live in the same coordinate space
// as x/y/w/h, corners are elliptical with independent per-axis clamps, and the
// stroke is a centered ring band straddling the path edge.
func (s *PreviewService) renderShapeElement(img *image.RGBA, x, y, w, h int, props map[string]any, defaultColor color.RGBA, scale float64) {
	// Read fill color: try "fill" (v2 frontend), then "fillColor" (legacy).
	// An explicit "transparent" fill leaves the interior as background (ring),
	// which differs from an unset fill (falls back to defaultColor).
	fillStr := GetPropString(props, "fill", "")
	if fillStr == "" {
		fillStr = GetPropString(props, "fillColor", "")
	}
	var fillColor color.RGBA
	hasFill := false
	switch {
	case fillStr == "transparent":
		hasFill = false
	case fillStr != "":
		fillColor = parseHexColor(fillStr)
		hasFill = true
	default:
		fillColor = defaultColor
		hasFill = true
	}

	// Scale corner radii and stroke width by the supersample factor so they
	// match the already-scaled x/y/w/h (D0). ry falls back to rx when unset.
	rx := int(float64(GetPropInt(props, "rx", 0)) * scale)
	ry := int(float64(GetPropInt(props, "ry", GetPropInt(props, "rx", 0))) * scale)
	sw := int(float64(GetPropInt(props, "strokeWidth", 0)) * scale)

	strokeStr := GetPropString(props, "stroke", "")
	hasStroke := strokeStr != "" && strokeStr != "transparent" && sw > 0

	// Fill the full rounded path first, then paint the centered stroke ring on
	// top. The stroke overpaints the outer sw/2 of the fill and adds sw/2
	// outside, so opaque fill + stroke matches Fabric's centered stroke while a
	// transparent fill + stroke renders as a clean ring (interior untouched).
	if hasFill {
		drawRoundedRectFilled(img, x, y, w, h, rx, ry, fillColor)
	}
	if hasStroke {
		drawRoundedRectStroke(img, x, y, w, h, rx, ry, sw, parseHexColor(strokeStr))
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

// RenderActive renders the currently active design with palette quantization.
func (s *PreviewService) RenderActive(ctx context.Context) ([]byte, error) {
	return s.RenderActiveRaw(ctx, false)
}

// RenderActiveRaw renders the currently active design. If raw is true, no palette quantization is applied.
func (s *PreviewService) RenderActiveRaw(ctx context.Context, raw bool) ([]byte, error) {
	design, err := s.design.GetActive()
	if err != nil {
		return nil, err
	}
	if design == nil {
		return nil, fmt.Errorf("no active design")
	}
	return s.Render(ctx, design, raw)
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

	// Negative cache hit: same return value as a live fetch failure.
	negKey := "url:" + calURL
	if failCache.blocked(negKey) {
		return "No events"
	}

	client := &defaultHTTPClient
	resp, err := client.Get(calURL)
	if err != nil {
		slog.Error("failed to fetch calendar", "error", err)
		failCache.markFailure(negKey)
		return "No events"
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		failCache.markFailure(negKey)
		return "No events"
	}
	failCache.markSuccess(negKey)

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

// cachedFont returns the parsed font for key, loading and parsing the raw
// font bytes via load on a cache miss (hit reports which case occurred).
// The parsed *opentype.Font is read-only and safe to share across
// concurrent renders (unlike font.Face).
func (s *PreviewService) cachedFont(key string, load func() ([]byte, error)) (f *opentype.Font, hit bool, err error) {
	s.fontMu.RLock()
	f, ok := s.fontCache[key]
	s.fontMu.RUnlock()
	if ok {
		return f, true, nil
	}

	data, err := load()
	if err != nil {
		return nil, false, err
	}
	f, err = opentype.Parse(data)
	if err != nil {
		return nil, false, err
	}

	s.fontMu.Lock()
	if len(s.fontCache) >= maxFontCacheEntries {
		// Evict a random entry
		for k := range s.fontCache {
			delete(s.fontCache, k)
			break
		}
	}
	s.fontCache[key] = f
	s.fontMu.Unlock()

	return f, false, nil
}

// newFaceAt creates a fresh face for the given parsed font and size. Faces
// carry per-glyph rasterizer state and are NOT safe for concurrent use, so
// every call site gets its own instance; identical FaceOptions produce
// byte-identical glyph rendering.
func newFaceAt(f *opentype.Font, size int) (font.Face, error) {
	return opentype.NewFace(f, &opentype.FaceOptions{
		Size:    float64(size),
		DPI:     72,
		Hinting: font.HintingFull,
	})
}

// loadGoFont returns Go's built-in regular font at the given size.
func (s *PreviewService) loadGoFont(size int) font.Face {
	f, hit, err := s.cachedFont(goFontCacheKey, func() ([]byte, error) { return goregular.TTF, nil })
	if err != nil {
		slog.Error("failed to parse Go built-in font", "error", err)
		return nil
	}

	face, err := newFaceAt(f, size)
	if err != nil {
		slog.Error("failed to create Go font face", "error", err)
		return nil
	}

	if !hit {
		slog.Info("using Go built-in font", "size", size)
	}
	return face
}

// loadTTFFace loads a TrueType font file and returns a font.Face. The parsed
// font is cached by path; the returned face is a fresh per-call instance.
func (s *PreviewService) loadTTFFace(path string, size int) font.Face {
	f, _, err := s.cachedFont(path, func() ([]byte, error) { return os.ReadFile(path) })
	if err != nil {
		return nil
	}

	face, err := newFaceAt(f, size)
	if err != nil {
		return nil
	}
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
	if si, ok := src.(interface {
		SubImage(image.Rectangle) image.Image
	}); ok {
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

// clampCornerRadii clamps rx to w/2 and ry to h/2 independently (Fabric's
// per-axis clamp), flooring negatives to 0. A wide-flat box with a large
// radius becomes a pill/ellipse rather than collapsing to a single circle.
func clampCornerRadii(w, h, rx, ry int) (int, int) {
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
	return rx, ry
}

// setPixelRGBA sets a pixel only if it lies within img's bounds.
func setPixelRGBA(img *image.RGBA, bounds image.Rectangle, ix, iy int, c color.RGBA) {
	if ix >= bounds.Min.X && ix < bounds.Max.X && iy >= bounds.Min.Y && iy < bounds.Max.Y {
		img.SetRGBA(ix, iy, c)
	}
}

// insideRoundedRect reports whether the center of pixel (px, py) lies inside
// the rounded rectangle at (x, y) of size (w, h) with elliptical corner radii
// (rx, ry). Radii are clamped per-axis. The elliptical corner test uses the
// same pixel-center convention as drawRoundedRectFilled so fill and stroke
// share one boundary definition.
func insideRoundedRect(px, py, x, y, w, h, rx, ry int) bool {
	if w <= 0 || h <= 0 {
		return false
	}
	if px < x || px >= x+w || py < y || py >= y+h {
		return false
	}
	rx, ry = clampCornerRadii(w, h, rx, ry)
	if rx <= 0 || ry <= 0 {
		return true
	}

	cx := float64(px) + 0.5
	cy := float64(py) + 0.5

	// Nearest corner-ellipse center on each axis; the straight bands (center
	// column/row) are trivially inside.
	var ex, ey float64
	switch {
	case cx < float64(x+rx):
		ex = float64(x + rx)
	case cx > float64(x+w-rx):
		ex = float64(x + w - rx)
	default:
		return true
	}
	switch {
	case cy < float64(y+ry):
		ey = float64(y + ry)
	case cy > float64(y+h-ry):
		ey = float64(y + h - ry)
	default:
		return true
	}

	dx := (cx - ex) / float64(rx)
	dy := (cy - ey) / float64(ry)
	return dx*dx+dy*dy <= 1.0
}

// drawRoundedRectFilled draws a filled rectangle with elliptical rounded
// corners. rx and ry are clamped independently (see clampCornerRadii); rx<=0
// or ry<=0 degenerates to a plain filled rectangle (the byte-identical fast
// path for non-rounded shapes).
func drawRoundedRectFilled(img *image.RGBA, x, y, w, h, rx, ry int, c color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	rx, ry = clampCornerRadii(w, h, rx, ry)
	if rx <= 0 || ry <= 0 {
		rect := image.Rect(x, y, x+w, y+h).Intersect(img.Bounds())
		draw.Draw(img, rect, image.NewUniform(c), image.Point{}, draw.Src)
		return
	}

	uni := image.NewUniform(c)
	bounds := img.Bounds()

	// Center horizontal strip (full height between the corner columns)
	draw.Draw(img, image.Rect(x+rx, y, x+w-rx, y+h).Intersect(bounds), uni, image.Point{}, draw.Src)
	// Left vertical strip (between corners)
	draw.Draw(img, image.Rect(x, y+ry, x+rx, y+h-ry).Intersect(bounds), uni, image.Point{}, draw.Src)
	// Right vertical strip (between corners)
	draw.Draw(img, image.Rect(x+w-rx, y+ry, x+w, y+h-ry).Intersect(bounds), uni, image.Point{}, draw.Src)

	// Four elliptical corner arcs. dx/dy are normalized to the corner radii so
	// the unit-circle test (dx²+dy² <= 1) describes an ellipse in pixel space.
	frx, fry := float64(rx), float64(ry)
	for py := 0; py < ry; py++ {
		for px := 0; px < rx; px++ {
			dx := (float64(rx-1-px) + 0.5) / frx
			dy := (float64(ry-1-py) + 0.5) / fry
			if dx*dx+dy*dy <= 1.0 {
				setPixelRGBA(img, bounds, x+px, y+py, c)         // top-left
				setPixelRGBA(img, bounds, x+w-1-px, y+py, c)     // top-right
				setPixelRGBA(img, bounds, x+px, y+h-1-py, c)     // bottom-left
				setPixelRGBA(img, bounds, x+w-1-px, y+h-1-py, c) // bottom-right
			}
		}
	}
}

// drawRoundedRectStroke paints a centered stroke ring of width sw straddling
// the path edge of the rounded rectangle at (x, y, w, h, rx, ry): the band
// covers sw/2 outside and sw/2 inside the edge, matching Fabric's centered
// stroke (strokeUniform). It fills only the band (outer rounded rect minus
// inner rounded rect), so a transparent-fill shape renders as a true ring.
func drawRoundedRectStroke(img *image.RGBA, x, y, w, h, rx, ry, sw int, c color.RGBA) {
	if w <= 0 || h <= 0 || sw <= 0 {
		return
	}
	rx, ry = clampCornerRadii(w, h, rx, ry)

	// Split the stroke width across the edge. For even sw (always the case at
	// render_quality=high, scale 2.0) this is exactly sw/2 each side.
	outerOff := sw / 2
	innerOff := sw - outerOff

	ox, oy := x-outerOff, y-outerOff
	ow, oh := w+2*outerOff, h+2*outerOff
	orx, ory := rx+outerOff, ry+outerOff

	inx, iny := x+innerOff, y+innerOff
	iw, ih := w-2*innerOff, h-2*innerOff
	irx, iry := rx-innerOff, ry-innerOff
	if irx < 0 {
		irx = 0
	}
	if iry < 0 {
		iry = 0
	}

	bounds := img.Bounds()
	startX, endX := clampRange(ox, ox+ow, bounds.Min.X, bounds.Max.X)
	startY, endY := clampRange(oy, oy+oh, bounds.Min.Y, bounds.Max.Y)

	for py := startY; py < endY; py++ {
		for px := startX; px < endX; px++ {
			if insideRoundedRect(px, py, ox, oy, ow, oh, orx, ory) &&
				!insideRoundedRect(px, py, inx, iny, iw, ih, irx, iry) {
				img.SetRGBA(px, py, c)
			}
		}
	}
}

// clampRange clamps the half-open interval [lo, hi) to [min, max).
func clampRange(lo, hi, min, max int) (int, int) {
	if lo < min {
		lo = min
	}
	if hi > max {
		hi = max
	}
	return lo, hi
}
