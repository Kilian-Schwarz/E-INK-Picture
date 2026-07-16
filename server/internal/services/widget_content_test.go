package services

// Dispatcher unit test for PreviewService.WidgetTextContent (B4a).
//
// WidgetTextContent is the single content-dispatch entry point that both the
// panel renderer (drawElement) and POST /api/widget_content route through.
// This test proves, hermetically, that for every supported element type it
// routes to exactly the fill*Content method drawElement used to call inline,
// and that types without server-side text content report ok == false.
//
// Determinism follows the offline_render_test.go stubbing pattern:
// installRenderTransport swaps both renderer network paths for hostStubTransport
// and resets the negative cache, so calendar/news/custom/weather fetches are
// reproducible.

import (
	"testing"

	"e-ink-picture/server/internal/models"
)

func TestWidgetTextContent(t *testing.T) {
	svc, _ := setupGoldenServices(t, models.DisplayWaveshare73E, models.RenderQualityHigh)
	installRenderTransport(t, svc, hostStubTransport{
		"api.open-meteo.com": canonicalOpenMeteoJSON,
		"news.offline.test":  offlineStubRSS,
		"cal.offline.test":   offlineStubICal,
		"api.offline.test":   "42",
	})

	// Each case's want closure calls the fill*Content method drawElement used
	// to dispatch inline; WidgetTextContent must return exactly the same value.
	cases := []struct {
		name  string
		typ   string
		props map[string]any
		want  func() string
	}{
		{"text", "text", map[string]any{"text": "Hallo"},
			func() string { return svc.fillTextContent(map[string]any{"text": "Hallo"}) }},
		{"i-text", "i-text", map[string]any{"text": "Welt"},
			func() string { return svc.fillTextContent(map[string]any{"text": "Welt"}) }},
		{"textbox", "textbox", map[string]any{"content": "Box"},
			func() string { return svc.fillTextContent(map[string]any{"content": "Box"}) }},
		{"clock", "widget_clock", map[string]any{"layout": "date_only"},
			func() string { return svc.fillClockContent(map[string]any{"layout": "date_only"}) }},
		{"weather", "widget_weather", map[string]any{"latitude": "52.52", "longitude": "13.41"},
			func() string {
				return svc.fillWeatherContent(map[string]any{"latitude": "52.52", "longitude": "13.41"})
			}},
		{"forecast", "widget_forecast", map[string]any{"latitude": "52.52", "longitude": "13.41", "days": float64(3)},
			func() string {
				return svc.fillForecastContent(map[string]any{"latitude": "52.52", "longitude": "13.41", "days": float64(3)})
			}},
		{"calendar", "widget_calendar", map[string]any{"icalUrl": offlineCalendarURL, "title": "Termine", "layout": "agenda", "maxEvents": float64(3)},
			func() string {
				return svc.fillCalendarContent(map[string]any{"icalUrl": offlineCalendarURL, "title": "Termine", "layout": "agenda", "maxEvents": float64(3)})
			}},
		{"news", "widget_news", map[string]any{"feedUrl": offlineNewsURL, "title": "News", "showDescription": true, "layout": "summary", "maxItems": float64(3)},
			func() string {
				return svc.fillNewsContent(map[string]any{"feedUrl": offlineNewsURL, "title": "News", "showDescription": true, "layout": "summary", "maxItems": float64(3)})
			}},
		{"system", "widget_system", map[string]any{"showLabels": false, "layout": "horizontal"},
			func() string {
				return svc.fillSystemContent(map[string]any{"showLabels": false, "layout": "horizontal"})
			}},
		{"custom", "widget_custom", map[string]any{"url": offlineCustomURL, "prefix": "[", "suffix": "]"},
			func() string {
				return svc.fillCustomContent(map[string]any{"url": offlineCustomURL, "prefix": "[", "suffix": "]"})
			}},
		{"timer", "widget_timer", map[string]any{"targetDate": "2999-01-01 00:00:00", "layout": "days_only"},
			func() string {
				return svc.fillTimerContent(map[string]any{"targetDate": "2999-01-01 00:00:00", "layout": "days_only"})
			}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.want()
			got, ok := svc.WidgetTextContent(tc.typ, tc.props)
			if !ok {
				t.Fatalf("WidgetTextContent(%q) ok = false, want true", tc.typ)
			}
			if got != want {
				t.Errorf("WidgetTextContent(%q) = %q, want %q (must route to the same fill*Content drawElement draws)", tc.typ, got, want)
			}
		})
	}

	// Types without server-side text content: ok must be false with an empty
	// string, so drawElement and the endpoint treat them as "no text content".
	for _, typ := range []string{"image", "shape", "widget_unknown", "rect", ""} {
		if got, ok := svc.WidgetTextContent(typ, nil); ok || got != "" {
			t.Errorf("WidgetTextContent(%q) = (%q, %v), want (\"\", false)", typ, got, ok)
		}
	}
}
