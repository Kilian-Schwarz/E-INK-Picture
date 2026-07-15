package services

// Template gallery tests (spec E3.5, AC4).
//
// Two layers guard the shipped templates under server/static/templates/:
//
//  1. Static lint (TestTemplateManifest, TestTemplateLint): the manifest and
//     every template JSON obey the mechanical design rules from spec
//     direction 2 — the loss-free element subset, exact driver-palette
//     colors, canvas geometry, token conventions and the renderer's
//     implemented widget layouts.
//  2. Render proof (TestTemplateRender): every template renders through the
//     real pipeline for both display profiles WITHOUT any network access and
//     the quantized output is driver-palette exact (assertPaletteExactness
//     from golden_test.go).
//
// Offline strategy: templates embed live widgets (weather, forecast, news)
// whose fetches would otherwise hit open-meteo.com and tagesschau.de on every
// test run. All network paths of the renderer go through two package-local
// clients — WeatherService.client and defaultHTTPClient (news RSS, iCal,
// custom API). forceOfflineRendering swaps both transports for one that fails
// every request, so the widgets deterministically degrade to their documented
// fallback strings ("No data", "No news", …) and the render must still
// succeed. This proves AC4's "runs green without network" on every run
// instead of only when the network happens to be down.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"e-ink-picture/server/internal/models"
)

// templateGalleryDir is the shipped template location relative to this package.
var templateGalleryDir = filepath.Join("..", "..", "static", "templates")

// nextNewYearToken is substituted by the frontend use-flow (spec direction 4b)
// with "<year+1>-01-01 00:00:00" before a template reaches the server.
const nextNewYearToken = "__NEXT_NEW_YEAR__"

// templateAllowedTypes is the loss-free element subset from spec direction 2:
// textbox/i-text degrade on the designer save roundtrip and widget_custom is
// forbidden in templates.
var templateAllowedTypes = map[string]bool{
	"text":            true,
	"image":           true,
	"shape":           true,
	"widget_clock":    true,
	"widget_weather":  true,
	"widget_forecast": true,
	"widget_calendar": true,
	"widget_news":     true,
	"widget_timer":    true,
	"widget_system":   true,
}

// templateWidgetLayouts lists the layout ids the renderer actually implements
// per widget type. Calendar "detailed" is declared in widgets/layouts.go but
// NOT implemented by fillCalendarContent — it is deliberately absent here.
var templateWidgetLayouts = map[string]map[string]bool{
	"widget_clock":    {"digital_large": true, "digital_with_seconds": true, "digital_with_date": true, "date_only": true, "full": true, "custom": true},
	"widget_weather":  {"compact": true, "standard": true, "detailed": true, "minimal": true, "custom": true},
	"widget_forecast": {"vertical": true, "compact_row": true, "detailed_list": true, "custom": true},
	"widget_calendar": {"list": true, "agenda": true, "compact": true},
	"widget_news":     {"headlines": true, "summary": true, "single": true, "custom": true},
	"widget_timer":    {"countdown_large": true, "countdown_compact": true, "label_above": true, "days_only": true, "custom": true},
	"widget_system":   {"vertical": true, "horizontal": true, "compact": true, "custom": true},
}

type templateManifestEntry struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	File        string    `json:"file"`
	Setup       *[]string `json:"setup"`
}

type templateManifest struct {
	Templates []templateManifestEntry `json:"templates"`
}

func loadTemplateManifest(t *testing.T) templateManifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(templateGalleryDir, "index.json"))
	if err != nil {
		t.Fatalf("read template manifest: %v", err)
	}
	var m templateManifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse template manifest: %v", err)
	}
	return m
}

func readTemplateJSON(t *testing.T, entry templateManifestEntry) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(templateGalleryDir, entry.File))
	if err != nil {
		t.Fatalf("template %s: read %s: %v", entry.ID, entry.File, err)
	}
	return data
}

func parseTemplateDesign(t *testing.T, entry templateManifestEntry, raw []byte) *models.DesignV2 {
	t.Helper()
	var design models.DesignV2
	if err := json.Unmarshal(raw, &design); err != nil {
		t.Fatalf("template %s: %s does not parse as DesignV2: %v", entry.ID, entry.File, err)
	}
	return &design
}

// instantiateTemplateJSON applies the frontend use-flow token substitution
// (spec direction 4b) on the raw template JSON, exactly like
// template-gallery.js does before POSTing the design.
func instantiateTemplateJSON(raw []byte) []byte {
	next := fmt.Sprintf("%d-01-01 00:00:00", time.Now().Year()+1)
	return bytes.ReplaceAll(raw, []byte(nextNewYearToken), []byte(next))
}

// parseTimerTarget mirrors the date formats accepted by fillTimerContent.
func parseTimerTarget(target string) (time.Time, error) {
	var firstErr error
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02T15:04"} {
		ts, err := time.ParseInLocation(layout, target, time.Local)
		if err == nil {
			return ts, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return time.Time{}, firstErr
}

// templatePaletteSet returns the exact 6-color driver palette (uppercase hex)
// from the display model — the single source of truth for template colors.
// The B/W panel palette is a subset, so linting against the 6-color palette
// covers both profiles.
func templatePaletteSet() map[string]bool {
	set := make(map[string]bool)
	for _, hex := range models.GetDisplayConfig(models.DisplayWaveshare73E).Colors {
		set[strings.ToUpper(hex)] = true
	}
	return set
}

func propFloat(props map[string]any, key string) (float64, bool) {
	v, ok := props[key]
	if !ok {
		return 0, false
	}
	f, ok := v.(float64)
	return f, ok
}

// blockedTransport fails every HTTP request. It forces the offline code path
// of all live widgets without touching production code.
type blockedTransport struct{}

func (blockedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("template tests run offline: blocked %s %s", req.Method, req.URL)
}

// forceOfflineRendering cuts every network path the renderer can take: the
// weather service client (open-meteo) and the package-level defaultHTTPClient
// (news RSS, iCal, custom API). Both are package-local, so the swap needs no
// production change; the global transport is restored on cleanup (no test in
// this package runs in parallel).
func forceOfflineRendering(t *testing.T, svc *PreviewService) {
	t.Helper()
	svc.weather.client = &http.Client{Transport: blockedTransport{}}
	orig := defaultHTTPClient.Transport
	defaultHTTPClient.Transport = blockedTransport{}
	t.Cleanup(func() { defaultHTTPClient.Transport = orig })
}

// TestTemplateManifest checks manifest <-> file consistency (AC1): exactly 8
// well-formed entries, every listed file exists, and no unlisted JSON hides
// in the templates directory.
func TestTemplateManifest(t *testing.T) {
	m := loadTemplateManifest(t)

	if len(m.Templates) != 8 {
		t.Errorf("manifest lists %d templates, want exactly 8 (AC1)", len(m.Templates))
	}

	ids := make(map[string]bool)
	names := make(map[string]bool)
	files := make(map[string]bool)
	for _, e := range m.Templates {
		if e.ID == "" || e.Name == "" || e.Description == "" || e.File == "" {
			t.Errorf("template %q: manifest entry must set id, name, description and file (got %+v)", e.ID, e)
			continue
		}
		if e.Setup == nil {
			t.Errorf("template %s: manifest entry is missing the setup array (use [] when no setup is needed)", e.ID)
		}
		if ids[e.ID] {
			t.Errorf("template %s: duplicate manifest id", e.ID)
		}
		ids[e.ID] = true
		if names[e.Name] {
			t.Errorf("template %s: duplicate manifest name %q (collision suffix logic is name-based)", e.ID, e.Name)
		}
		names[e.Name] = true
		if filepath.Base(e.File) != e.File || !strings.HasSuffix(e.File, ".json") {
			t.Errorf("template %s: file %q must be a plain *.json filename without path components", e.ID, e.File)
		}
		if files[e.File] {
			t.Errorf("template %s: file %q is referenced by more than one manifest entry", e.ID, e.File)
		}
		files[e.File] = true
		if _, err := os.Stat(filepath.Join(templateGalleryDir, e.File)); err != nil {
			t.Errorf("template %s: listed file does not exist: %v", e.ID, err)
		}
	}

	entries, err := os.ReadDir(templateGalleryDir)
	if err != nil {
		t.Fatalf("read templates dir: %v", err)
	}
	for _, de := range entries {
		name := de.Name()
		if de.IsDir() {
			t.Errorf("unexpected directory %s in %s", name, templateGalleryDir)
			continue
		}
		if name == "index.json" {
			continue
		}
		if !files[name] {
			t.Errorf("file %s in %s is not listed in index.json", name, templateGalleryDir)
		}
	}
}

// TestTemplateLint enforces the mechanical design rules from spec direction 2
// on every shipped template.
func TestTemplateLint(t *testing.T) {
	m := loadTemplateManifest(t)
	palette := templatePaletteSet()

	for _, entry := range m.Templates {
		t.Run(entry.ID, func(t *testing.T) {
			raw := readTemplateJSON(t, entry)
			design := parseTemplateDesign(t, entry, raw)

			if design.Version != 2 {
				t.Errorf("template %s: version = %d, want 2", entry.ID, design.Version)
			}
			if design.Name == "" {
				t.Errorf("template %s: design name must not be empty", entry.ID)
			}
			if design.Canvas.Width != 800 || design.Canvas.Height != 480 {
				t.Errorf("template %s: canvas %dx%d, want 800x480", entry.ID, design.Canvas.Width, design.Canvas.Height)
			}
			if strings.ToUpper(design.Canvas.Background) != "#FFFFFF" {
				t.Errorf("template %s: canvas background %q, want #FFFFFF", entry.ID, design.Canvas.Background)
			}
			if len(design.Elements) == 0 {
				t.Fatalf("template %s: has no elements", entry.ID)
			}

			idPattern := regexp.MustCompile(`^tpl_` + regexp.QuoteMeta(entry.ID) + `_[0-9]+$`)
			seenIDs := make(map[string]bool)
			fontSizes := make(map[float64]bool)
			accents := make(map[string]bool)
			tokenUses := 0
			calendarNeedsURL := false
			hasPhotoSlot := false

			for _, elem := range design.Elements {
				ref := fmt.Sprintf("template %s, element %s (%s)", entry.ID, elem.ID, elem.Type)

				if seenIDs[elem.ID] {
					t.Errorf("%s: duplicate element id", ref)
				}
				seenIDs[elem.ID] = true
				if !idPattern.MatchString(elem.ID) {
					t.Errorf("%s: element id must match tpl_%s_<n>", ref, entry.ID)
				}

				if !templateAllowedTypes[elem.Type] {
					t.Errorf("%s: element type is outside the loss-free template subset (no textbox/i-text/widget_custom)", ref)
					continue
				}

				if elem.Rotation != 0 {
					t.Errorf("%s: rotation must be 0, got %g", ref, elem.Rotation)
				}
				if elem.Width <= 0 || elem.Height <= 0 {
					t.Errorf("%s: width/height must be positive, got %gx%g", ref, elem.Width, elem.Height)
				}
				if elem.X < 0 || elem.Y < 0 || elem.X+elem.Width > 800 || elem.Y+elem.Height > 480 {
					t.Errorf("%s: box (x=%g y=%g %gx%g) exceeds the 800x480 canvas", ref, elem.X, elem.Y, elem.Width, elem.Height)
				}

				props := elem.Properties
				if props == nil {
					t.Errorf("%s: element has no properties object", ref)
					continue
				}

				for _, key := range []string{"fontFamily", "verticalAlign"} {
					if _, ok := props[key]; ok {
						t.Errorf("%s: property %q is forbidden (lost on the designer save roundtrip)", ref, key)
					}
				}

				for _, key := range []string{"color", "fill", "stroke"} {
					v, ok := props[key]
					if !ok {
						continue
					}
					s, isStr := v.(string)
					if !isStr {
						t.Errorf("%s: property %q must be a string, got %T", ref, key, v)
						continue
					}
					if s == "transparent" {
						if key == "color" {
							t.Errorf("%s: transparent is only allowed for fill/stroke, not for color", ref)
						}
						continue
					}
					up := strings.ToUpper(s)
					if !palette[up] {
						t.Errorf("%s: %s %q is not an exact 6-color driver palette color", ref, key, s)
						continue
					}
					if up != "#000000" && up != "#FFFFFF" {
						accents[up] = true
					}
				}

				if slot, ok := props["templateSlot"]; ok {
					isPhotoSlot := entry.ID == "photo-clock" && elem.Type == "image" && slot == "photo"
					isLocationSlot := entry.ID == "weather-dashboard" && elem.Type == "text" && slot == "location"
					if !isPhotoSlot && !isLocationSlot {
						t.Errorf("%s: templateSlot %v is only allowed as \"photo\" on the photo template's image element or \"location\" on a weather-dashboard text element", ref, slot)
					}
				}

				if elem.Type == "image" {
					if img := GetPropString(props, "image", ""); img != "" {
						t.Errorf("%s: templates must not ship a bundled image asset (image=%q)", ref, img)
					}
					if GetPropString(props, "templateSlot", "") == "photo" {
						hasPhotoSlot = true
					} else {
						t.Errorf("%s: image elements in templates must be marked templateSlot=photo", ref)
					}
				}

				if allowedLayouts, isWidget := templateWidgetLayouts[elem.Type]; isWidget {
					if layout := GetPropString(props, "layout", ""); layout != "" && !allowedLayouts[layout] {
						t.Errorf("%s: layout %q is not implemented by the renderer for this widget", ref, layout)
					}
				}

				isTextual := elem.Type == "text" || strings.HasPrefix(elem.Type, "widget_")
				if isTextual {
					fs, hasFS := propFloat(props, "fontSize")
					if hasFS {
						fontSizes[fs] = true
					}
					if c := strings.ToUpper(GetPropString(props, "color", "#000000")); c != "#000000" && c != "#FFFFFF" {
						if !hasFS || fs < 28 {
							t.Errorf("%s: colored text requires fontSize >= 28 (B/W dithering legibility), got %g", ref, fs)
						}
					}
				}

				switch elem.Type {
				case "widget_calendar":
					if GetPropString(props, "icalUrl", "") == "" {
						calendarNeedsURL = true
					}
				case "widget_news":
					if feed := GetPropString(props, "feedUrl", ""); feed != "" && !strings.HasPrefix(feed, "https://") {
						t.Errorf("%s: feedUrl must be empty or an https URL, got %q", ref, feed)
					}
				case "widget_system":
					tmpl := GetPropString(props, "customTemplate", "")
					for _, banned := range []string{"%uptime%", "%hostname%"} {
						if strings.Contains(tmpl, banned) {
							t.Errorf("%s: placeholder %s is not implemented by applySystemPlaceholders", ref, banned)
						}
					}
				case "widget_timer":
					target := GetPropString(props, "targetDate", "")
					if target == nextNewYearToken {
						tokenUses++
					} else if ts, err := parseTimerTarget(target); err != nil {
						t.Errorf("%s: targetDate %q is neither %s nor a parseable date: %v", ref, target, nextNewYearToken, err)
					} else if !ts.After(time.Now()) {
						t.Errorf("%s: targetDate %q must lie in the future", ref, target)
					}
				}
			}

			if len(fontSizes) > 3 {
				var sizes []float64
				for fs := range fontSizes {
					sizes = append(sizes, fs)
				}
				sort.Float64s(sizes)
				t.Errorf("template %s: %d distinct font sizes %v, want at most 3 (typography hierarchy rule)", entry.ID, len(sizes), sizes)
			}

			if len(accents) > 1 {
				var colors []string
				for c := range accents {
					colors = append(colors, c)
				}
				sort.Strings(colors)
				t.Errorf("template %s: %d accent colors %v, want at most 1 (single accent rule)", entry.ID, len(colors), colors)
			}

			if got := bytes.Count(raw, []byte(nextNewYearToken)); got != tokenUses {
				t.Errorf("template %s: %s appears %d times in the raw JSON but has only %d sanctioned uses (widget_timer targetDate)",
					entry.ID, nextNewYearToken, got, tokenUses)
			}

			setup := make(map[string]bool)
			if entry.Setup != nil {
				for _, s := range *entry.Setup {
					setup[s] = true
				}
			}
			if calendarNeedsURL != setup["Calendar ICS URL"] {
				t.Errorf("template %s: calendar widget without icalUrl = %v but manifest setup lists \"Calendar ICS URL\" = %v",
					entry.ID, calendarNeedsURL, setup["Calendar ICS URL"])
			}
			if hasPhotoSlot != setup["Your photo"] {
				t.Errorf("template %s: photo slot present = %v but manifest setup lists \"Your photo\" = %v",
					entry.ID, hasPhotoSlot, setup["Your photo"])
			}
		})
	}
}

// TestTemplateRender renders every gallery template for both display profiles
// through the real pipeline with all network paths blocked: the render must
// succeed on widget fallback strings alone, produce an 800x480 PNG and be
// driver-palette exact. No golden bytes — clock time and live data vary.
func TestTemplateRender(t *testing.T) {
	m := loadTemplateManifest(t)

	for _, entry := range m.Templates {
		raw := instantiateTemplateJSON(readTemplateJSON(t, entry))
		for _, displayType := range goldenDisplays {
			t.Run(fmt.Sprintf("%s__%s", entry.ID, displayType), func(t *testing.T) {
				previewSvc, _ := setupGoldenServices(t, displayType, models.RenderQualityHigh)
				forceOfflineRendering(t, previewSvc)

				design := parseTemplateDesign(t, entry, raw)

				// The use-flow substitution must yield a timer target the
				// renderer accepts — catches format drift between the token
				// substitution and fillTimerContent (spec risk table).
				for _, elem := range design.Elements {
					if elem.Type != "widget_timer" {
						continue
					}
					if content := previewSvc.fillTimerContent(elem.Properties); content == "Invalid timer target" {
						t.Errorf("template %s, element %s: substituted targetDate %q is rejected by the renderer",
							entry.ID, elem.ID, GetPropString(elem.Properties, "targetDate", ""))
					}
				}

				pngData, err := previewSvc.Render(context.Background(), design, false)
				if err != nil {
					t.Fatalf("template %s: offline render failed on %s: %v", entry.ID, displayType, err)
				}

				cfg, err := png.DecodeConfig(bytes.NewReader(pngData))
				if err != nil {
					t.Fatalf("template %s: rendered PNG not decodable: %v", entry.ID, err)
				}
				if cfg.Width != 800 || cfg.Height != 480 {
					t.Errorf("template %s: rendered %dx%d, want 800x480", entry.ID, cfg.Width, cfg.Height)
				}

				assertPaletteExactness(t, pngData, displayType, entry.ID)
			})
		}
	}
}
