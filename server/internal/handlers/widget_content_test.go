package handlers

// Content-equality proof for B4a: POST /api/widget_content returns EXACTLY the
// string PreviewService.WidgetTextContent produces on the same service
// instance, for a table of configs that exercises the props which currently
// drift between the editor canvas and the E-Ink panel (calendar title/layout/
// maxEvents, news title/showDescription/layout, system showLabels, custom
// prefix/suffix/jsonPath, weather/forecast). This proves the canvas and the
// panel share one content source once B4b points the canvas at this endpoint.
//
// Determinism is hermetic: calendar/news/custom fetch httptest.Servers on
// localhost, weather/forecast read a pre-seeded weather cache (no network),
// and system reads /proc (identical on both call paths regardless of OS).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"e-ink-picture/server/internal/models"
	"e-ink-picture/server/internal/services"
)

const testWidgetRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
<item><title>Erste Meldung</title><description>Beschreibung eins</description><link>https://x/1</link></item>
<item><title>Zweite Meldung</title><description>Beschreibung zwei</description><link>https://x/2</link></item>
<item><title>Dritte Meldung</title><description>Beschreibung drei</description><link>https://x/3</link></item>
</channel></rss>`

const testWidgetJSON = `{"temp": 21.5, "value": "ok"}`

// futureICal builds an iCal with n VEVENTs at now+1d .. now+nd (all within the
// 7-day default daysAhead window) so calendar formatting runs deterministically.
func futureICal(n int) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\n")
	base := time.Now()
	for i := 1; i <= n; i++ {
		start := base.Add(time.Duration(i) * 24 * time.Hour).UTC()
		fmt.Fprintf(&b, "BEGIN:VEVENT\r\nDTSTART:%s\r\nSUMMARY:Termin %d\r\nEND:VEVENT\r\n",
			start.Format("20060102T150405Z"), i)
	}
	b.WriteString("END:VCALENDAR\r\n")
	return b.String()
}

// seedWeatherCache writes a fresh weather cache entry for 52.52,13.41 so
// FetchForLocation serves it without any network call (loaded on construction).
func seedWeatherCache(t *testing.T, dataDir string) {
	t.Helper()
	cacheDir := filepath.Join(dataDir, "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	type persisted struct {
		Data     *services.WeatherData `json:"data"`
		CachedAt time.Time             `json:"cached_at"`
	}
	entry := persisted{
		Data: &services.WeatherData{
			CurrentTemp: 17.3,
			CurrentDesc: "Bewölkt",
			Daily: []services.DailyForecast{
				{Min: 12, Max: 22, Desc: "Bewölkt", Weekday: "Mo"},
				{Min: 11, Max: 20, Desc: "Regen", Weekday: "Di"},
				{Min: 10, Max: 19, Desc: "Sonnig", Weekday: "Mi"},
			},
		},
		CachedAt: time.Now(),
	}
	raw, err := json.Marshal(map[string]persisted{"52.52,13.41": entry})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "weather.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

// newContentTestService builds a PreviewService over an isolated data dir with
// a pre-seeded weather cache.
func newContentTestService(t *testing.T) *services.PreviewService {
	t.Helper()
	dir := t.TempDir()
	for _, sub := range []string{"designs", "uploaded_images", "fonts", "weather_styles", "cache"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	seedWeatherCache(t, dir)

	designSvc := services.NewDesignService(dir)
	imageSvc := services.NewImageService(dir)
	weatherSvc := services.NewWeatherService("", "", dir)
	settingsSvc := services.NewSettingsService(dir, models.DisplayWaveshare73E)
	return services.NewPreviewService(designSvc, weatherSvc, imageSvc, settingsSvc, dir)
}

// postContent posts {type, properties} through the wired route and returns the
// status code and decoded {content}.
func postContent(t *testing.T, mux *http.ServeMux, typ string, props map[string]any) (int, string) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"type": typ, "properties": props})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/widget_content", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		return rec.Code, ""
	}
	var resp struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode content response: %v", err)
	}
	return rec.Code, resp.Content
}

func TestWidgetContentEndpointMatchesDispatcher(t *testing.T) {
	svc := newContentTestService(t)
	// Content routes everything through the PreviewService dispatch; the legacy
	// WeatherService field and its E-path handlers were removed in B4c.
	h := NewWidgetHandler(svc)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/widget_content", h.Content)

	calSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(futureICal(4)))
	}))
	t.Cleanup(calSrv.Close)
	newsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(testWidgetRSS))
	}))
	t.Cleanup(newsSrv.Close)
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(testWidgetJSON))
	}))
	t.Cleanup(apiSrv.Close)

	cases := []struct {
		name  string
		typ   string
		props map[string]any
		// contains/omits assert the drifting prop actually shaped the content,
		// so the equality below is a meaningful (not vacuous) proof.
		contains []string
		omits    []string
	}{
		{name: "text", typ: "text", props: map[string]any{"text": "Hallo Welt"}, contains: []string{"Hallo Welt"}},

		{name: "weather compact", typ: "widget_weather",
			props:    map[string]any{"latitude": "52.52", "longitude": "13.41"},
			contains: []string{"17°C", "Bewölkt"}},
		{name: "weather custom template", typ: "widget_weather",
			props:    map[string]any{"latitude": "52.52", "longitude": "13.41", "layout": "custom", "customTemplate": "%temperature%°C / %description%"},
			contains: []string{"17°C / Bewölkt"}},

		{name: "forecast days 2", typ: "widget_forecast",
			props:    map[string]any{"latitude": "52.52", "longitude": "13.41", "days": float64(2)},
			contains: []string{"Mo: 12-22°C", "Di: 11-20°C"},
			omits:    []string{"Mi:"}},
		{name: "forecast default", typ: "widget_forecast",
			props:    map[string]any{"latitude": "52.52", "longitude": "13.41"},
			contains: []string{"Mo:", "Di:", "Mi:"}},

		{name: "calendar agenda title maxEvents", typ: "widget_calendar",
			props:    map[string]any{"icalUrl": calSrv.URL, "title": "Termine", "layout": "agenda", "maxEvents": float64(2)},
			contains: []string{"Termine", "Termin 1", "Termin 2"},
			omits:    []string{"Termin 3"}},
		{name: "calendar compact caps 3", typ: "widget_calendar",
			props:    map[string]any{"icalUrl": calSrv.URL, "layout": "compact", "maxEvents": float64(5)},
			contains: []string{"Termin 1", "Termin 2", "Termin 3"},
			omits:    []string{"Termin 4"}},
		{name: "calendar list default", typ: "widget_calendar",
			props:    map[string]any{"icalUrl": calSrv.URL},
			contains: []string{"Termin 1"}},

		{name: "news headlines with description", typ: "widget_news",
			props:    map[string]any{"feedUrl": newsSrv.URL, "title": "Schlagzeilen", "showDescription": true, "layout": "headlines", "maxItems": float64(2)},
			contains: []string{"Schlagzeilen", "Erste Meldung", "Beschreibung eins"},
			omits:    []string{"Dritte Meldung"}},
		{name: "news headlines without description", typ: "widget_news",
			props:    map[string]any{"feedUrl": newsSrv.URL, "showDescription": false, "layout": "headlines", "maxItems": float64(2)},
			contains: []string{"Erste Meldung"},
			omits:    []string{"Beschreibung eins"}},
		{name: "news summary", typ: "widget_news",
			props:    map[string]any{"feedUrl": newsSrv.URL, "layout": "summary", "maxItems": float64(2)},
			contains: []string{"Beschreibung eins"}},
		{name: "news single", typ: "widget_news",
			props:    map[string]any{"feedUrl": newsSrv.URL, "layout": "single"},
			contains: []string{"Erste Meldung"},
			omits:    []string{"Zweite Meldung"}},

		{name: "system labels off horizontal", typ: "widget_system",
			props: map[string]any{"showLabels": false, "layout": "horizontal"},
			omits: []string{"Load:", "\n"}},
		{name: "system labels on vertical", typ: "widget_system",
			props:    map[string]any{"showLabels": true, "layout": "vertical"},
			contains: []string{"Load:"}},

		{name: "custom jsonPath prefix suffix", typ: "widget_custom",
			props:    map[string]any{"url": apiSrv.URL, "jsonPath": "temp", "prefix": "T=", "suffix": "°"},
			contains: []string{"T=21.5°"}},
		{name: "custom raw", typ: "widget_custom",
			props:    map[string]any{"url": apiSrv.URL},
			contains: []string{"21.5"}},

		// widget_progress needs no fixture: it is computed purely locally.
		// period=year is used so the two calls cannot straddle a period
		// boundary in any realistic run.
		{name: "progress bar_percent", typ: "widget_progress",
			props:    map[string]any{"period": "year", "layout": "bar_percent", "barWidth": float64(20), "timezone": "UTC"},
			contains: []string{"[", "]", "%"}},
		{name: "progress count", typ: "widget_progress",
			props:    map[string]any{"period": "year", "layout": "count", "timezone": "UTC"},
			contains: []string{"Tag ", " von "},
			omits:    []string{"["}},
		{name: "progress custom template", typ: "widget_progress",
			props:    map[string]any{"period": "year", "layout": "custom", "customTemplate": "%period%=%percent%", "timezone": "UTC"},
			contains: []string{"Jahr="},
			omits:    []string{"["}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Direct dispatcher call: the exact string the panel renderer draws.
			want, ok := svc.WidgetTextContent(tc.typ, tc.props)
			if !ok {
				t.Fatalf("WidgetTextContent(%q) ok = false, want true", tc.typ)
			}

			// Endpoint call: full properties map round-tripped through JSON.
			code, got := postContent(t, mux, tc.typ, tc.props)
			if code != http.StatusOK {
				t.Fatalf("POST /api/widget_content = %d, want 200", code)
			}
			if got != want {
				t.Fatalf("endpoint content = %q, want %q (panel dispatcher); the two must be one source", got, want)
			}

			for _, sub := range tc.contains {
				if !strings.Contains(got, sub) {
					t.Errorf("content %q does not contain %q (prop did not shape output)", got, sub)
				}
			}
			for _, sub := range tc.omits {
				if strings.Contains(got, sub) {
					t.Errorf("content %q unexpectedly contains %q (prop was ignored)", got, sub)
				}
			}
		})
	}
}

func TestWidgetContentUnsupportedType(t *testing.T) {
	svc := newContentTestService(t)
	h := NewWidgetHandler(svc)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/widget_content", h.Content)

	// image/shape have no text content; unknown types are rejected too.
	for _, typ := range []string{"image", "shape", "widget_bogus", "rect"} {
		code, _ := postContent(t, mux, typ, map[string]any{})
		if code != http.StatusBadRequest {
			t.Errorf("POST type=%q = %d, want 400", typ, code)
		}
	}

	// Malformed JSON body → 400.
	req := httptest.NewRequest(http.MethodPost, "/api/widget_content", strings.NewReader("not json"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("malformed body = %d, want 400", rec.Code)
	}
}
