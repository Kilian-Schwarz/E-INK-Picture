package handlers

// Content-equality proof for widget_hass (specs/B5-home-assistant.md, sub-task
// B5b, AC-PIPE2): POST /api/widget_content returns EXACTLY the string
// PreviewService.WidgetTextContent produces on the same service instance, for
// the three HA modes and the graceful cases (AC-HA4..HA6). The HA backend is a
// local httptest.Server the hardened allowlist client dials on 127.0.0.1, so no
// real Home-Assistant and no real token are involved.

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"e-ink-picture/server/internal/hass"
	"e-ink-picture/server/internal/services"
)

// hassCTToken is a sentinel Long-Lived-Token; no real value ever appears here.
const hassCTToken = "SENTINEL_TOKEN_DO_NOT_LOG"

// newHassBackend serves canned /api/states/<id> bodies (404 for unmapped ids)
// on localhost.
func newHassBackend(t *testing.T, bodies map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/states/")
		id, _ = url.PathUnescape(id)
		body, ok := bodies[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// configuredHassPreview wires a HassService (configured against baseURL with
// the sentinel token) into a fresh PreviewService.
func configuredHassPreview(t *testing.T, baseURL string) *services.PreviewService {
	t.Helper()
	svc := newContentTestService(t)
	mgr, err := hass.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := mgr.SetConfig(baseURL, hassCTToken); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	svc.SetHassService(services.NewHassService(mgr))
	return svc
}

// assertContentEquality posts props for a widget_hass element and asserts the
// endpoint content equals the direct dispatcher output and contains want.
func assertContentEquality(t *testing.T, svc *services.PreviewService, mux *http.ServeMux, props map[string]any, want string) {
	t.Helper()
	dispatch, ok := svc.WidgetTextContent("widget_hass", props)
	if !ok {
		t.Fatal(`WidgetTextContent("widget_hass") ok = false, want true`)
	}
	if dispatch != want {
		t.Fatalf("dispatcher content = %q, want %q", dispatch, want)
	}
	code, got := postContent(t, mux, "widget_hass", props)
	if code != http.StatusOK {
		t.Fatalf("POST /api/widget_content = %d, want 200", code)
	}
	if got != dispatch {
		t.Fatalf("endpoint content = %q, want %q (panel dispatcher); the two must be one source", got, dispatch)
	}
	if strings.Contains(got, hassCTToken) {
		t.Fatalf("content %q leaks the sentinel token", got)
	}
}

func newContentMux(svc *services.PreviewService) *http.ServeMux {
	h := NewWidgetHandler(nil, svc)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/widget_content", h.Content)
	return mux
}

// TestHassContentEndpointMatchesDispatcher is AC-PIPE2 for the three modes plus
// AC-HA6 (unknown entity).
func TestHassContentEndpointMatchesDispatcher(t *testing.T) {
	backend := newHassBackend(t, map[string]string{
		"sensor.wohnzimmer":        `{"state":"21.5","attributes":{"unit_of_measurement":"°C","friendly_name":"Wohnzimmer"}}`,
		"alarm_control_panel.home": `{"state":"armed_away"}`,
		"person.kilian":            `{"state":"home","attributes":{"friendly_name":"Kilian"}}`,
	})
	svc := configuredHassPreview(t, backend.URL)
	mux := newContentMux(svc)

	cases := []struct {
		name  string
		props map[string]any
		want  string
	}{
		{"temperature", map[string]any{"hassMode": "temperature", "entityId": "sensor.wohnzimmer"}, "21.5°C"},
		{"alarm", map[string]any{"hassMode": "alarm", "entityId": "alarm_control_panel.home"}, "Scharf (Abwesend)"},
		{"presence", map[string]any{"hassMode": "presence", "entityId": "person.kilian"}, "Zuhause"},
		{"unknown entity (AC-HA6)", map[string]any{"hassMode": "temperature", "entityId": "sensor.ghost"}, "Unbekannt: sensor.ghost"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertContentEquality(t, svc, mux, tc.props, tc.want)
		})
	}
}

// TestHassContentNotConfiguredEquality is AC-PIPE2 for AC-HA4: with no
// HassService wired the endpoint and the dispatcher both return
// "HA nicht konfiguriert".
func TestHassContentNotConfiguredEquality(t *testing.T) {
	svc := newContentTestService(t) // no hass wired → nil-tolerant path
	mux := newContentMux(svc)
	props := map[string]any{"hassMode": "temperature", "entityId": "sensor.x"}
	assertContentEquality(t, svc, mux, props, "HA nicht konfiguriert")
}

// TestHassContentUnavailableEquality is AC-PIPE2 for AC-HA5: a backend that is
// closed (connection refused) yields "Nicht verfügbar" from both paths.
func TestHassContentUnavailableEquality(t *testing.T) {
	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	baseURL := closed.URL
	closed.Close() // port now refuses connections

	svc := configuredHassPreview(t, baseURL)
	mux := newContentMux(svc)
	props := map[string]any{"hassMode": "temperature", "entityId": "sensor.x"}
	assertContentEquality(t, svc, mux, props, "Nicht verfügbar")
}
