package services

// End-to-end offline hardening tests (spec E5.5, AC1/AC3/AC4).
//
// Pattern: forceOfflineRendering from template_render_test.go — both renderer
// network paths (WeatherService.client and defaultHTTPClient) are swapped for
// test transports. Additions here:
//
//   - countingTransport counts every HTTP attempt per URL and fails
//     immediately (optionally after a delay, optionally delegating to a
//     success stub) — the hard metric for the negative cache assertions,
//   - newDelayTransport is the time-scaled blackhole substitute (real
//     blackhole = 10 s client timeout; the test scales that to 100 ms per
//     attempt — never real seconds),
//   - hostStubTransport serves canned success responses per host for the
//     recovery phase.
//
// The negative cache is package-global, so every test resets it via
// installRenderTransport (failCache.reset in t.Cleanup) for a defined
// counter baseline.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"e-ink-picture/server/internal/models"
)

// countingTransport counts HTTP attempts per URL. Without an inner transport
// every attempt fails immediately (after an optional delay).
type countingTransport struct {
	mu    sync.Mutex
	delay time.Duration
	inner http.RoundTripper
	seen  map[string]int
}

func newDelayTransport(delay time.Duration) *countingTransport {
	return &countingTransport{delay: delay}
}

func (c *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	if c.seen == nil {
		c.seen = make(map[string]int)
	}
	c.seen[req.URL.String()]++
	delay := c.delay
	inner := c.inner
	c.mu.Unlock()

	if delay > 0 {
		time.Sleep(delay)
	}
	if inner != nil {
		return inner.RoundTrip(req)
	}
	return nil, fmt.Errorf("offline render test: refused %s %s", req.Method, req.URL)
}

// total returns the number of HTTP attempts across all URLs.
func (c *countingTransport) total() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, v := range c.seen {
		n += v
	}
	return n
}

// count returns the attempts for one exact URL.
func (c *countingTransport) count(url string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.seen[url]
}

// countByHost returns the attempts against one host (open-meteo URLs carry
// the full query string, so exact-URL counting is impractical there).
func (c *countingTransport) countByHost(host string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for u, v := range c.seen {
		if strings.Contains(u, "://"+host+"/") {
			n += v
		}
	}
	return n
}

// setInner switches the transport to a success delegate (recovery phase).
func (c *countingTransport) setInner(rt http.RoundTripper) {
	c.mu.Lock()
	c.inner = rt
	c.mu.Unlock()
}

// hostStubTransport serves a canned 200 body per host.
type hostStubTransport map[string]string

func (h hostStubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, ok := h[req.URL.Host]
	if !ok {
		return nil, fmt.Errorf("offline render test: no stub for host %s", req.URL.Host)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}, nil
}

// installRenderTransport swaps BOTH renderer network paths for rt and resets
// the negative cache so counter assertions start from a defined state
// (forceOfflineRendering pattern from template_render_test.go).
func installRenderTransport(t *testing.T, svc *PreviewService, rt http.RoundTripper) {
	t.Helper()
	failCache.reset()
	t.Cleanup(failCache.reset)
	svc.weather.client = &http.Client{Transport: rt}
	orig := defaultHTTPClient.Transport
	defaultHTTPClient.Transport = rt
	t.Cleanup(func() { defaultHTTPClient.Transport = orig })
}

const (
	offlineNewsURL     = "https://news.offline.test/feed.xml"
	offlineCalendarURL = "https://cal.offline.test/calendar.ics"
	offlineCustomURL   = "https://api.offline.test/value"
)

const offlineStubRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
<item><title>Local headline</title><description>Recovered feed</description><link>https://news.offline.test/1</link><pubDate>Wed, 15 Jul 2026 06:00:00 GMT</pubDate></item>
</channel></rss>`

const offlineStubICal = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nEND:VCALENDAR\r\n"

// offlineWidgetDesign builds the AC3 design: weather + forecast on the SAME
// coordinates plus one news, one calendar and one custom API source — five
// fetch widgets, four distinct sources.
func offlineWidgetDesign() *models.DesignV2 {
	mk := func(id, typ string, y float64, z int, props map[string]any) models.Element {
		props["fontSize"] = float64(18)
		props["fontFamily"] = "testfont.ttf"
		props["color"] = "#000000"
		return models.Element{ID: id, Type: typ, X: 20, Y: y, Width: 760, Height: 80, ZIndex: z, Properties: props}
	}
	return &models.DesignV2{
		Name:    "offline-widgets",
		Version: 2,
		Canvas:  models.CanvasConfig{Width: 800, Height: 480, Background: "#FFFFFF"},
		Elements: []models.Element{
			mk("w", "widget_weather", 10, 1, map[string]any{"latitude": "52.52", "longitude": "13.41"}),
			mk("f", "widget_forecast", 100, 2, map[string]any{"latitude": "52.52", "longitude": "13.41"}),
			mk("n", "widget_news", 190, 3, map[string]any{"feedUrl": offlineNewsURL}),
			mk("c", "widget_calendar", 280, 4, map[string]any{"icalUrl": offlineCalendarURL}),
			mk("a", "widget_custom", 370, 5, map[string]any{"url": offlineCustomURL}),
		},
	}
}

// TestOfflineRenderNegativeCache proves AC3: render 1 pays exactly ONE
// attempt per source (weather + forecast share the open-meteo attempt),
// render 2 inside the TTL window makes ZERO attempts and produces a
// byte-identical PNG, after the TTL every source is retried once, and a
// healthy transport clears the negative entries and yields fresh data.
func TestOfflineRenderNegativeCache(t *testing.T) {
	previewSvc, _ := setupGoldenServices(t, models.DisplayWaveshare73E, models.RenderQualityHigh)
	ct := &countingTransport{}
	installRenderTransport(t, previewSvc, ct)

	clock := newTestClock(time.Now())
	failCache.setNow(clock.Now)

	design := offlineWidgetDesign()

	// Render 1: exactly one attempt per source.
	png1, err := previewSvc.Render(context.Background(), design, false)
	if err != nil {
		t.Fatalf("render 1: %v", err)
	}
	if got := ct.countByHost("api.open-meteo.com"); got != 1 {
		t.Errorf("render 1: open-meteo attempts = %d, want exactly 1 (forecast must hit the weather fill's negative entry)", got)
	}
	for _, u := range []string{offlineNewsURL, offlineCalendarURL, offlineCustomURL} {
		if got := ct.count(u); got != 1 {
			t.Errorf("render 1: attempts for %s = %d, want 1", u, got)
		}
	}
	if got := ct.total(); got != 4 {
		t.Errorf("render 1: total attempts = %d, want 4", got)
	}

	// Render 2 inside the TTL window: zero new attempts, byte-identical PNG.
	png2, err := previewSvc.Render(context.Background(), design, false)
	if err != nil {
		t.Fatalf("render 2: %v", err)
	}
	if got := ct.total(); got != 4 {
		t.Errorf("render 2 made %d new attempts, want 0 (negative cache must suppress retries)", got-4)
	}
	if !bytes.Equal(png1, png2) {
		t.Error("render 2 (negative cache hits) must be byte-identical to render 1 (fallback strings by construction)")
	}

	// TTL elapsed: every source is retried once (recovery proof).
	clock.Advance(negativeCacheTTL + time.Second)
	if _, err := previewSvc.Render(context.Background(), design, false); err != nil {
		t.Fatalf("render 3: %v", err)
	}
	if got := ct.total(); got != 8 {
		t.Errorf("render 3 after TTL: total attempts = %d, want 8 (one retry per source)", got)
	}

	// Transport healthy again: fetches succeed, negative entries are cleared,
	// the following render uses fresh data.
	ct.setInner(hostStubTransport{
		"api.open-meteo.com": canonicalOpenMeteoJSON,
		"news.offline.test":  offlineStubRSS,
		"cal.offline.test":   offlineStubICal,
		"api.offline.test":   "42",
	})
	clock.Advance(negativeCacheTTL + time.Second)
	png4, err := previewSvc.Render(context.Background(), design, false)
	if err != nil {
		t.Fatalf("render 4 (recovered transport): %v", err)
	}
	if got := ct.total(); got != 12 {
		t.Errorf("render 4: total attempts = %d, want 12", got)
	}
	failCache.mu.Lock()
	remaining := len(failCache.entries)
	failCache.mu.Unlock()
	if remaining != 0 {
		t.Errorf("negative cache entries after successful fetches = %d, want 0 (success must clear entries)", remaining)
	}
	if bytes.Equal(png1, png4) {
		t.Error("render with recovered transport must differ from the offline fallback render (fresh data visible)")
	}
}

// TestCustomAPINegativeCacheHitPreservesHTTPStatus guards the spec guardrail
// "hit == EXACT live failure value" for the one fetch site whose failure
// value varies: fetchCustomAPI returns "HTTP <code>" on a non-200 response.
// A negative cache hit inside the TTL window must reproduce that exact string
// (not flip to "Error") while making zero new HTTP attempts.
func TestCustomAPINegativeCacheHitPreservesHTTPStatus(t *testing.T) {
	failCache.reset()
	t.Cleanup(failCache.reset)
	ct := &countingTransport{inner: stubTransport{status: http.StatusServiceUnavailable, body: "boom"}}
	orig := defaultHTTPClient.Transport
	defaultHTTPClient.Transport = ct
	t.Cleanup(func() { defaultHTTPClient.Transport = orig })

	const url = "https://api.offline.test/http503"
	live := fetchCustomAPI(url, map[string]any{})
	if live != "HTTP 503" {
		t.Fatalf("live non-200 fetch = %q, want %q", live, "HTTP 503")
	}
	hit := fetchCustomAPI(url, map[string]any{})
	if hit != live {
		t.Errorf("negative cache hit = %q, want exactly the live failure value %q", hit, live)
	}
	if got := ct.total(); got != 1 {
		t.Errorf("cache hit must not retry: %d HTTP attempts, want 1", got)
	}
}

// TestOfflineRenderStaleWeatherAfterRestart proves AC1 at the fill level: a
// restarted service stack over the same data dir renders the persisted stale
// temperature instead of "No data" while fully offline.
func TestOfflineRenderStaleWeatherAfterRestart(t *testing.T) {
	previewSvc, tmpDir := setupGoldenServices(t, models.DisplayWaveshare73E, models.RenderQualityHigh)

	// One successful fetch populates and persists the weather cache.
	previewSvc.weather.client = &http.Client{Transport: stubTransport{status: http.StatusOK, body: canonicalOpenMeteoJSON}}
	if _, err := previewSvc.weather.FetchForLocation("52.52", "13.41"); err != nil {
		t.Fatalf("populate weather cache: %v", err)
	}
	backdatePersistedWeather(t, filepath.Join(tmpDir, "cache", "weather.json"), 2*time.Hour)

	// Simulated restart: a NEW service stack over the same data dir, offline.
	restarted := newGoldenPreviewService(tmpDir)
	installRenderTransport(t, restarted, blockedTransport{})

	props := map[string]any{"latitude": "52.52", "longitude": "13.41"}
	got := restarted.fillWeatherContent(props)
	if got == "No data" {
		t.Fatal("stale persisted weather must survive the restart, got \"No data\"")
	}
	if got != "17°C Partly cloudy" {
		t.Errorf("fillWeatherContent after offline restart = %q, want %q", got, "17°C Partly cloudy")
	}
}

// TestOfflineRenderBlockedFast proves AC4a: the fetch-heaviest gallery
// template (weather-dashboard: 3 weather + 1 forecast widgets) renders in
// under 5 s wall time when every request fails instantly. The generous bound
// tolerates CI noise but catches any accidental sync-retry or sleep
// regression in the offline path.
func TestOfflineRenderBlockedFast(t *testing.T) {
	m := loadTemplateManifest(t)
	var entry templateManifestEntry
	for _, e := range m.Templates {
		if e.ID == "weather-dashboard" {
			entry = e
		}
	}
	if entry.ID == "" {
		t.Fatal("weather-dashboard template not found in manifest")
	}

	previewSvc, _ := setupGoldenServices(t, models.DisplayWaveshare73E, models.RenderQualityHigh)
	installRenderTransport(t, previewSvc, blockedTransport{})

	design := parseTemplateDesign(t, entry, instantiateTemplateJSON(readTemplateJSON(t, entry)))

	start := time.Now()
	if _, err := previewSvc.Render(context.Background(), design, false); err != nil {
		t.Fatalf("offline render failed: %v", err)
	}
	dur := time.Since(start)
	t.Logf("offline render of %s with blocked transport took %s", entry.ID, dur)
	if dur >= 5*time.Second {
		t.Errorf("offline render took %s, want < 5s (AC4)", dur)
	}
}

// TestOfflineRenderDelayedTransport proves AC4b with the time-scaled
// blackhole substitute: render 1 pays at most sources x 100 ms fetch share
// (hard metric: exactly 4 attempts), render 2 inside the TTL window makes 0
// attempts. Wall times are logged as plausibility values.
func TestOfflineRenderDelayedTransport(t *testing.T) {
	previewSvc, _ := setupGoldenServices(t, models.DisplayWaveshare73E, models.RenderQualityHigh)
	dt := newDelayTransport(100 * time.Millisecond)
	installRenderTransport(t, previewSvc, dt)

	design := offlineWidgetDesign()

	start := time.Now()
	if _, err := previewSvc.Render(context.Background(), design, false); err != nil {
		t.Fatalf("render 1: %v", err)
	}
	dur1 := time.Since(start)
	if got := dt.total(); got != 4 {
		t.Errorf("render 1: %d HTTP attempts, want 4 (fetch share bounded by sources x delay)", got)
	}

	start = time.Now()
	if _, err := previewSvc.Render(context.Background(), design, false); err != nil {
		t.Fatalf("render 2: %v", err)
	}
	dur2 := time.Since(start)
	if got := dt.total(); got != 4 {
		t.Errorf("render 2 inside TTL made %d new attempts, want 0", got-4)
	}
	t.Logf("delayed-transport renders: first %s (4 attempts x 100 ms), second %s (0 attempts)", dur1, dur2)
	if dur1 >= 5*time.Second || dur2 >= 5*time.Second {
		t.Errorf("delayed-transport renders took %s / %s, want each < 5s", dur1, dur2)
	}
}
