package services

// Content tests for fillHassContent (specs/B5-home-assistant.md, sub-task B5b).
// They drive the shared WidgetTextContent dispatcher against MOCKED HA
// responses (the B5a recordingTransport/stateTransport stubs) and assert the
// German output for every mode and graceful case (AC-HA1..HA7). The token is a
// sentinel throughout (safeFetchSentinelToken) and every content assertion
// confirms it never leaks (AC-SEC3). All expected German strings are compared
// against the locale.go constants/tables, never hardcoded duplicates (AC-HA7).

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"e-ink-picture/server/internal/hass"
)

// hassPreview builds a PreviewService whose HassService is wired to transport
// and configured with the sentinel token against hassTestBaseURL.
func hassPreview(t *testing.T, transport http.RoundTripper) *PreviewService {
	t.Helper()
	p := newGoldenPreviewService(t.TempDir())
	p.SetHassService(newConfiguredHassService(t, transport))
	return p
}

// hassContent renders a widget_hass element through the shared dispatcher.
func hassContent(t *testing.T, p *PreviewService, props map[string]any) string {
	t.Helper()
	content, ok := p.WidgetTextContent("widget_hass", props)
	if !ok {
		t.Fatal(`WidgetTextContent("widget_hass") ok = false, want true`)
	}
	if strings.Contains(content, safeFetchSentinelToken) {
		t.Fatalf("content %q leaks the sentinel token (AC-SEC3)", content)
	}
	return content
}

// stateTransport serves a canned 200 JSON body per entity id (matched by the
// /api/states/<id> path suffix); an unmapped id yields 404. It records calls so
// multi-entity fan-out can be observed.
type stateTransport struct {
	mu     sync.Mutex
	bodies map[string]string
	calls  int
}

func (st *stateTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	st.mu.Lock()
	st.calls++
	st.mu.Unlock()
	for id, body := range st.bodies {
		if strings.HasSuffix(req.URL.EscapedPath(), "/api/states/"+url.PathEscape(id)) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
			}, nil
		}
	}
	return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody, Header: make(http.Header)}, nil
}

// TestFillHassTemperatureSingle is AC-HA1 (single entity): state+unit with no
// space, and the token never appears in the content (AC-SEC3).
func TestFillHassTemperatureSingle(t *testing.T) {
	rt := &recordingTransport{respond: respondOK(
		`{"state":"21.5","attributes":{"unit_of_measurement":"°C","friendly_name":"Wohnzimmer"}}`)}
	p := hassPreview(t, rt)

	got := hassContent(t, p, map[string]any{"hassMode": "temperature", "entityId": "sensor.wohnzimmer"})
	if got != "21.5°C" {
		t.Errorf("content = %q, want %q", got, "21.5°C")
	}
}

// TestFillHassTemperatureMulti is AC-HA1 (several entities): one
// "<friendly_name>: <state><unit>" line each.
func TestFillHassTemperatureMulti(t *testing.T) {
	st := &stateTransport{bodies: map[string]string{
		"sensor.a": `{"state":"21.5","attributes":{"unit_of_measurement":"°C","friendly_name":"A"}}`,
		"sensor.b": `{"state":"19.0","attributes":{"unit_of_measurement":"°C","friendly_name":"B"}}`,
	}}
	p := hassPreview(t, st)

	got := hassContent(t, p, map[string]any{"hassMode": "temperature", "entityId": "sensor.a, sensor.b"})
	want := "A: 21.5°C\nB: 19.0°C"
	if got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

// TestFillHassTemperatureNonNumeric is AC-HA1 (graceful): a non-numeric state
// (unavailable/unknown) renders germanHassNoValue for that entity.
func TestFillHassTemperatureNonNumeric(t *testing.T) {
	for _, state := range []string{"unavailable", "unknown"} {
		rt := &recordingTransport{respond: respondOK(`{"state":"` + state + `"}`)}
		p := hassPreview(t, rt)
		got := hassContent(t, p, map[string]any{"hassMode": "temperature", "entityId": "sensor.x"})
		if got != germanHassNoValue {
			t.Errorf("state %q → %q, want germanHassNoValue %q", state, got, germanHassNoValue)
		}
	}
}

// TestFillHassAlarm is AC-HA2: every mapped state renders its germanHassAlarm
// label, an unmapped state falls back to the raw state, and a label is
// prepended.
func TestFillHassAlarm(t *testing.T) {
	for state, want := range germanHassAlarm {
		rt := &recordingTransport{respond: respondOK(`{"state":"` + state + `"}`)}
		p := hassPreview(t, rt)
		got := hassContent(t, p, map[string]any{"hassMode": "alarm", "entityId": "alarm_control_panel.home"})
		if got != want {
			t.Errorf("alarm state %q → %q, want %q (germanHassAlarm)", state, got, want)
		}
	}

	rt := &recordingTransport{respond: respondOK(`{"state":"weird_mode"}`)}
	p := hassPreview(t, rt)
	if got := hassContent(t, p, map[string]any{"hassMode": "alarm", "entityId": "alarm_control_panel.home"}); got != "weird_mode" {
		t.Errorf("unmapped alarm state → %q, want raw %q", got, "weird_mode")
	}

	rtL := &recordingTransport{respond: respondOK(`{"state":"disarmed"}`)}
	pL := hassPreview(t, rtL)
	want := "Haus: " + germanHassAlarm["disarmed"]
	if got := hassContent(t, pL, map[string]any{"hassMode": "alarm", "entityId": "alarm_control_panel.home", "label": "Haus"}); got != want {
		t.Errorf("labeled alarm = %q, want %q", got, want)
	}
}

// TestFillHassPresenceSingle is AC-HA3 (single entity): home/not_home map to
// German, a zone name passes through, and a label is prepended.
func TestFillHassPresenceSingle(t *testing.T) {
	cases := []struct{ state, want string }{
		{"home", germanHassHome},
		{"not_home", germanHassAway},
		{"Arbeit", "Arbeit"},
	}
	for _, tc := range cases {
		rt := &recordingTransport{respond: respondOK(`{"state":"` + tc.state + `"}`)}
		p := hassPreview(t, rt)
		got := hassContent(t, p, map[string]any{"hassMode": "presence", "entityId": "person.kilian"})
		if got != tc.want {
			t.Errorf("presence state %q → %q, want %q", tc.state, got, tc.want)
		}
	}

	rt := &recordingTransport{respond: respondOK(`{"state":"home"}`)}
	p := hassPreview(t, rt)
	want := "Kilian: " + germanHassHome
	if got := hassContent(t, p, map[string]any{"hassMode": "presence", "entityId": "person.kilian", "label": "Kilian"}); got != want {
		t.Errorf("labeled presence = %q, want %q", got, want)
	}
}

// TestFillHassPresenceMulti is AC-HA3 (several entities): a "<n> zuhause"
// summary (germanHassHomeCount) followed by each present person's name, and
// germanHassNobodyHome when nobody is home.
func TestFillHassPresenceMulti(t *testing.T) {
	st := &stateTransport{bodies: map[string]string{
		"person.a": `{"state":"home","attributes":{"friendly_name":"Anna"}}`,
		"person.b": `{"state":"not_home","attributes":{"friendly_name":"Ben"}}`,
		"person.c": `{"state":"home","attributes":{"friendly_name":"Carla"}}`,
	}}
	p := hassPreview(t, st)
	got := hassContent(t, p, map[string]any{"hassMode": "presence", "entityId": "person.a,person.b,person.c"})
	want := germanHassHomeCount(2) + "\nAnna\nCarla"
	if got != want {
		t.Errorf("multi presence = %q, want %q", got, want)
	}

	st2 := &stateTransport{bodies: map[string]string{
		"person.a": `{"state":"not_home"}`,
		"person.b": `{"state":"not_home"}`,
	}}
	p2 := hassPreview(t, st2)
	if got := hassContent(t, p2, map[string]any{"hassMode": "presence", "entityId": "person.a,person.b"}); got != germanHassNobodyHome {
		t.Errorf("nobody home = %q, want %q", got, germanHassNobodyHome)
	}
}

// TestFillHassNotConfiguredNilService is AC-HA4 (a): with no HassService wired,
// fillHassContent is nil-tolerant and returns germanHassNotConfigured.
func TestFillHassNotConfiguredNilService(t *testing.T) {
	p := newGoldenPreviewService(t.TempDir())
	got := hassContent(t, p, map[string]any{"hassMode": "temperature", "entityId": "sensor.x"})
	if got != germanHassNotConfigured {
		t.Errorf("nil hass → %q, want %q", got, germanHassNotConfigured)
	}
}

// TestFillHassNotConfiguredManager is AC-HA4 (b): the HassService is wired but
// the manager holds no config → germanHassNotConfigured, without any network
// call.
func TestFillHassNotConfiguredManager(t *testing.T) {
	failCache.reset()
	t.Cleanup(failCache.reset)

	mgr, err := hass.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	rt := &recordingTransport{respond: respondOK("{}")}
	svc := NewHassService(mgr)
	svc.testTransport = rt

	p := newGoldenPreviewService(t.TempDir())
	p.SetHassService(svc)

	got := hassContent(t, p, map[string]any{"hassMode": "temperature", "entityId": "sensor.x"})
	if got != germanHassNotConfigured {
		t.Errorf("unconfigured manager → %q, want %q", got, germanHassNotConfigured)
	}
	if rt.count() != 0 {
		t.Errorf("transport called %d times, want 0 when not configured", rt.count())
	}
}

// TestFillHassUnavailable is AC-HA5: a transport failure renders
// germanHassUnavailable without leaking the host, and the negative cache
// suppresses a second network attempt inside the TTL.
func TestFillHassUnavailable(t *testing.T) {
	rt := &recordingTransport{respond: func() (*http.Response, error) {
		return nil, errors.New("dial tcp 10.0.0.5:8123: connection refused")
	}}
	p := hassPreview(t, rt)
	props := map[string]any{"hassMode": "temperature", "entityId": "sensor.x"}

	got := hassContent(t, p, props)
	if got != germanHassUnavailable {
		t.Errorf("content = %q, want %q", got, germanHassUnavailable)
	}
	if strings.Contains(got, "10.0.0.5") {
		t.Errorf("content %q leaks the internal host", got)
	}

	if got2 := hassContent(t, p, props); got2 != germanHassUnavailable {
		t.Errorf("cached content = %q, want %q", got2, germanHassUnavailable)
	}
	if rt.count() != 1 {
		t.Errorf("transport calls = %d, want 1 (negative cache suppresses the retry)", rt.count())
	}
}

// TestFillHassEntityUnknown is AC-HA6: a 404 renders "Unbekannt: <entity_id>"
// (germanHassUnknownPrefix), distinct from the generic unavailable text so a
// typo stays visible.
func TestFillHassEntityUnknown(t *testing.T) {
	rt := &recordingTransport{respond: func() (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody, Header: make(http.Header)}, nil
	}}
	p := hassPreview(t, rt)

	got := hassContent(t, p, map[string]any{"hassMode": "temperature", "entityId": "sensor.tippfehler"})
	want := germanHassUnknownPrefix + "sensor.tippfehler"
	if got != want {
		t.Errorf("404 content = %q, want %q", got, want)
	}
}

// TestFillHassNoEntity: an empty/whitespace entity list renders germanHassNoEntity
// without any network call.
func TestFillHassNoEntity(t *testing.T) {
	rt := &recordingTransport{respond: respondOK("{}")}
	p := hassPreview(t, rt)

	got := hassContent(t, p, map[string]any{"hassMode": "temperature", "entityId": "  ,  "})
	if got != germanHassNoEntity {
		t.Errorf("empty entity → %q, want %q", got, germanHassNoEntity)
	}
	if rt.count() != 0 {
		t.Errorf("transport called %d times, want 0 for an empty entity list", rt.count())
	}
}

// TestSetHassService covers the exported injection point: before wiring the
// widget is not configured; after wiring a mocked reading it renders live.
func TestSetHassService(t *testing.T) {
	p := newGoldenPreviewService(t.TempDir())
	props := map[string]any{"hassMode": "temperature", "entityId": "sensor.x"}

	if got := hassContent(t, p, props); got != germanHassNotConfigured {
		t.Fatalf("before SetHassService = %q, want %q", got, germanHassNotConfigured)
	}

	rt := &recordingTransport{respond: respondOK(`{"state":"21.5","attributes":{"unit_of_measurement":"°C"}}`)}
	p.SetHassService(newConfiguredHassService(t, rt))
	if got := hassContent(t, p, props); got != "21.5°C" {
		t.Fatalf("after SetHassService = %q, want %q", got, "21.5°C")
	}
}

// TestHassStringsOnlyInLocale is the static half of AC-HA7: the German HA
// state/error texts appear only in locale.go, never as literals in preview.go
// or hass.go — fillHassContent must reference the locale constants/tables.
func TestHassStringsOnlyInLocale(t *testing.T) {
	texts := []string{
		germanHassNotConfigured, germanHassUnavailable, germanHassNoEntity,
		germanHassHome, germanHassAway, germanHassNobodyHome,
		germanHassAlarm["disarmed"], germanHassAlarm["armed_home"],
		germanHassAlarm["armed_away"], germanHassAlarm["triggered"],
		germanHassAlarm["arming"], germanHassAlarm["disarming"],
		"zuhause",
	}
	for _, file := range []string{"preview.go", "hass.go"} {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, txt := range texts {
			if strings.Contains(string(src), txt) {
				t.Errorf("%s contains HA text %q as a literal — it must live only in locale.go (AC-HA7)", file, txt)
			}
		}
	}
}
