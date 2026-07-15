package services

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// canonicalOpenMeteoJSON is a minimal but complete open-meteo response used
// as the fake success payload for the persistence tests (AC1) and the
// negative cache recovery phase (AC3).
const canonicalOpenMeteoJSON = `{
  "current_weather": {"temperature": 17.3, "weathercode": 2},
  "daily": {
    "time": ["2026-07-15", "2026-07-16", "2026-07-17", "2026-07-18"],
    "weathercode": [2, 3, 61, 0],
    "temperature_2m_max": [22.1, 20.4, 18.9, 24.0],
    "temperature_2m_min": [12.3, 11.8, 10.5, 13.1],
    "sunrise": ["2026-07-15T05:12", "2026-07-16T05:13", "2026-07-17T05:14", "2026-07-18T05:16"],
    "sunset": ["2026-07-15T21:24", "2026-07-16T21:23", "2026-07-17T21:22", "2026-07-18T21:20"]
  },
  "hourly": {
    "time": ["2026-07-15T00:00", "2026-07-15T01:00", "2026-07-15T02:00", "2026-07-15T03:00"],
    "temperature_2m": [13.0, 12.5, 12.1, 11.8],
    "weathercode": [1, 2, 2, 3],
    "precipitation": [0, 0, 0.1, 0]
  }
}`

// stubTransport returns the same fixed response for every request.
type stubTransport struct {
	status int
	body   string
}

func (tr stubTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: tr.status,
		Body:       io.NopCloser(strings.NewReader(tr.body)),
		Header:     make(http.Header),
	}, nil
}

// captureLogs redirects the default slog logger into a buffer for warning
// assertions (fail-open paths must warn, never fail).
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(orig) })
	return &buf
}

// backdatePersistedWeather rewrites cached_at of every entry in the persisted
// cache file so a restarted service sees the entries as stale.
func backdatePersistedWeather(t *testing.T, cacheFile string, age time.Duration) {
	t.Helper()
	raw, err := os.ReadFile(cacheFile)
	if err != nil {
		t.Fatalf("read cache file: %v", err)
	}
	var persisted map[string]persistedWeatherEntry
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("parse cache file: %v", err)
	}
	for key, entry := range persisted {
		entry.CachedAt = time.Now().Add(-age)
		persisted[key] = entry
	}
	updated, err := json.Marshal(persisted)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cacheFile, updated, 0644); err != nil {
		t.Fatal(err)
	}
}

// TestWeatherCachePersistenceRoundtrip proves AC1: a successful fetch writes
// data/cache/weather.json atomically, and a NEW service instance on the same
// data dir (simulated restart) serves the persisted values with the network
// fully blocked — both while the entry is fresh and after it went stale.
func TestWeatherCachePersistenceRoundtrip(t *testing.T) {
	failCache.reset()
	t.Cleanup(failCache.reset)
	dataDir := t.TempDir()

	svcA := NewWeatherService("", "", dataDir)
	svcA.client = &http.Client{Transport: stubTransport{status: http.StatusOK, body: canonicalOpenMeteoJSON}}

	dataA, err := svcA.FetchForLocation("52.52", "13.41")
	if err != nil {
		t.Fatalf("fetch via fake success transport: %v", err)
	}
	if dataA.CurrentTemp != 17.3 || dataA.CurrentDesc != "Teilweise bewölkt" {
		t.Fatalf("unexpected fetch result: temp=%g desc=%q", dataA.CurrentTemp, dataA.CurrentDesc)
	}

	// Cache file exists, is valid JSON and holds the location key.
	cacheFile := filepath.Join(dataDir, "cache", "weather.json")
	raw, err := os.ReadFile(cacheFile)
	if err != nil {
		t.Fatalf("cache file not written: %v", err)
	}
	var persisted map[string]persistedWeatherEntry
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("cache file is not valid JSON: %v", err)
	}
	if _, ok := persisted["52.52,13.41"]; !ok {
		t.Fatalf("cache file misses key 52.52,13.41, has %v", persisted)
	}

	// Atomic write: no *.tmp leftovers in the cache directory.
	entries, err := os.ReadDir(filepath.Join(dataDir, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file left behind after atomic write: %s", e.Name())
		}
	}

	// Restart 1: fresh persisted entry, transport blocked — values from A are
	// served straight from the restored cache.
	svcB := NewWeatherService("", "", dataDir)
	svcB.client = &http.Client{Transport: blockedTransport{}}
	dataB, err := svcB.FetchForLocation("52.52", "13.41")
	if err != nil {
		t.Fatalf("restart with fresh persisted cache returned error: %v", err)
	}
	if dataB.CurrentTemp != dataA.CurrentTemp || dataB.CurrentDesc != dataA.CurrentDesc {
		t.Errorf("restarted service returned temp=%g desc=%q, want temp=%g desc=%q",
			dataB.CurrentTemp, dataB.CurrentDesc, dataA.CurrentTemp, dataA.CurrentDesc)
	}

	// Restart 2: entry backdated beyond the 30 min freshness window — the
	// fetch attempt fails offline and the STALE values are returned ("stale
	// ok" now survives restarts).
	backdatePersistedWeather(t, cacheFile, 2*time.Hour)
	failCache.reset()
	svcC := NewWeatherService("", "", dataDir)
	svcC.client = &http.Client{Transport: blockedTransport{}}
	dataC, err := svcC.FetchForLocation("52.52", "13.41")
	if err != nil {
		t.Fatalf("stale persisted cache must be returned on fetch failure, got error: %v", err)
	}
	if dataC.CurrentTemp != dataA.CurrentTemp || dataC.CurrentDesc != dataA.CurrentDesc {
		t.Errorf("stale restart returned temp=%g desc=%q, want temp=%g desc=%q",
			dataC.CurrentTemp, dataC.CurrentDesc, dataA.CurrentTemp, dataA.CurrentDesc)
	}
}

// TestWeatherCacheFailOpenMissingFile proves AC2a: no cache directory at all
// is the normal first start — empty cache, no warning, offline behavior
// identical to today (fetch error, widget falls back to "No data").
func TestWeatherCacheFailOpenMissingFile(t *testing.T) {
	failCache.reset()
	t.Cleanup(failCache.reset)
	logs := captureLogs(t)

	svc := NewWeatherService("", "", t.TempDir())
	if len(svc.cache) != 0 {
		t.Errorf("cache must start empty, has %d entries", len(svc.cache))
	}
	if strings.Contains(logs.String(), "weather cache") {
		t.Errorf("missing cache file must not log a warning, got: %s", logs.String())
	}

	svc.client = &http.Client{Transport: blockedTransport{}}
	if _, err := svc.FetchForLocation("52.52", "13.41"); err == nil {
		t.Error("offline fetch without any cache must return the error (widget shows \"No data\")")
	}
}

// TestWeatherCacheFailOpenCorruptFile proves AC2b: corrupt JSON in the cache
// file yields a working service with an empty cache plus a warning — never a
// startup or render error.
func TestWeatherCacheFailOpenCorruptFile(t *testing.T) {
	failCache.reset()
	t.Cleanup(failCache.reset)
	dataDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dataDir, "cache"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "cache", "weather.json"), []byte("{corrupt json"), 0644); err != nil {
		t.Fatal(err)
	}
	logs := captureLogs(t)

	svc := NewWeatherService("", "", dataDir)
	if len(svc.cache) != 0 {
		t.Errorf("corrupt cache file must yield an empty cache, has %d entries", len(svc.cache))
	}
	if !strings.Contains(logs.String(), "weather cache corrupt") {
		t.Errorf("corrupt cache file must log a warning, got: %s", logs.String())
	}

	svc.client = &http.Client{Transport: blockedTransport{}}
	if _, err := svc.FetchForLocation("52.52", "13.41"); err == nil {
		t.Error("offline fetch after corrupt cache must behave like an empty cache (error)")
	}
}

// TestWeatherCacheFailOpenWriteError proves AC2c: when persisting fails (the
// cache path is not writable), the fetch result is still returned — only a
// warning is logged.
func TestWeatherCacheFailOpenWriteError(t *testing.T) {
	failCache.reset()
	t.Cleanup(failCache.reset)
	dataDir := t.TempDir()
	// A regular file where the cache DIRECTORY should be makes MkdirAll fail.
	if err := os.WriteFile(filepath.Join(dataDir, "cache"), []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}
	logs := captureLogs(t)

	svc := NewWeatherService("", "", dataDir)
	svc.client = &http.Client{Transport: stubTransport{status: http.StatusOK, body: canonicalOpenMeteoJSON}}

	data, err := svc.FetchForLocation("52.52", "13.41")
	if err != nil || data == nil {
		t.Fatalf("fetch result must be returned despite persist failure, got data=%v err=%v", data, err)
	}
	if data.CurrentTemp != 17.3 {
		t.Errorf("fetch result temp = %g, want 17.3", data.CurrentTemp)
	}
	if !strings.Contains(logs.String(), "weather cache not persisted") {
		t.Errorf("persist failure must log a warning, got: %s", logs.String())
	}
}

func TestWeatherService_CacheEviction(t *testing.T) {
	svc := NewWeatherService("", "", t.TempDir())

	// Fill cache beyond max
	svc.mu.Lock()
	for i := 0; i < maxWeatherCacheEntries+10; i++ {
		key := "key_" + time.Now().Add(time.Duration(i)*time.Millisecond).Format("150405.000")
		svc.cache[key] = &weatherCacheEntry{
			data:     &WeatherData{CurrentTemp: float64(i)},
			cachedAt: time.Now().Add(time.Duration(i) * time.Millisecond),
		}
		svc.evictOldestCache()
	}
	svc.mu.Unlock()

	svc.mu.RLock()
	defer svc.mu.RUnlock()
	if len(svc.cache) > maxWeatherCacheEntries {
		t.Errorf("cache size %d exceeds max %d", len(svc.cache), maxWeatherCacheEntries)
	}
}

func TestWeatherCodeToDescIcon(t *testing.T) {
	// German condition text: one representative daytime code and one nighttime
	// code, plus the shared wording that only differs by icon (AC2/AC7).
	desc, icon := WeatherCodeToDescIcon(0, false)
	if desc != "Klarer Himmel" {
		t.Errorf("expected 'Klarer Himmel', got %q", desc)
	}
	if icon != "clear_day.png" {
		t.Errorf("expected 'clear_day.png', got %q", icon)
	}

	desc, icon = WeatherCodeToDescIcon(0, true)
	if desc != "Klarer Himmel" {
		t.Errorf("expected 'Klarer Himmel' for night, got %q", desc)
	}
	if icon != "clear_night.png" {
		t.Errorf("expected 'clear_night.png', got %q", icon)
	}

	// A second daytime code proves the map is not a single-entry accident.
	desc, icon = WeatherCodeToDescIcon(61, false)
	if desc != "Leichter Regen" {
		t.Errorf("expected 'Leichter Regen' for code 61, got %q", desc)
	}
	if icon != "rain_day.png" {
		t.Errorf("expected 'rain_day.png' for code 61, got %q", icon)
	}

	// A nighttime code with its own icon variant.
	desc, icon = WeatherCodeToDescIcon(2, true)
	if desc != "Teilweise bewölkt" {
		t.Errorf("expected 'Teilweise bewölkt' for code 2 night, got %q", desc)
	}
	if icon != "cloudy_night.png" {
		t.Errorf("expected 'cloudy_night.png' for code 2 night, got %q", icon)
	}

	desc, icon = WeatherCodeToDescIcon(999, false)
	if desc != "Unbekannt" {
		t.Errorf("expected 'Unbekannt' for code 999, got %q", desc)
	}
	if icon != "cloudy_day.png" {
		t.Errorf("expected fallback 'cloudy_day.png' for code 999, got %q", icon)
	}
}

// sevenDayOpenMeteoJSON is a complete 7-day open-meteo response (dates start on
// a Wednesday, 2026-07-15) used to prove the forecast now parses 7 days (AC5).
const sevenDayOpenMeteoJSON = `{
  "current_weather": {"temperature": 19.0, "weathercode": 1},
  "daily": {
    "time": ["2026-07-15", "2026-07-16", "2026-07-17", "2026-07-18", "2026-07-19", "2026-07-20", "2026-07-21"],
    "weathercode": [2, 3, 61, 0, 45, 80, 65],
    "temperature_2m_max": [22.1, 20.4, 18.9, 24.0, 19.5, 17.2, 21.8],
    "temperature_2m_min": [12.3, 11.8, 10.5, 13.1, 9.9, 8.7, 12.0],
    "sunrise": ["2026-07-15T05:12", "2026-07-16T05:13", "2026-07-17T05:14", "2026-07-18T05:16", "2026-07-19T05:17", "2026-07-20T05:18", "2026-07-21T05:19"],
    "sunset": ["2026-07-15T21:24", "2026-07-16T21:23", "2026-07-17T21:22", "2026-07-18T21:20", "2026-07-19T21:19", "2026-07-20T21:18", "2026-07-21T21:16"]
  },
  "hourly": {
    "time": ["2026-07-15T00:00", "2026-07-15T01:00"],
    "temperature_2m": [13.0, 12.5],
    "weathercode": [1, 2],
    "precipitation": [0, 0]
  }
}`

// TestFetchSevenDayForecast proves AC5: a 7-day open-meteo response yields 7
// Daily entries, and the weekday/desc of the first entry are German (AC1/AC2).
func TestFetchSevenDayForecast(t *testing.T) {
	failCache.reset()
	t.Cleanup(failCache.reset)

	svc := NewWeatherService("", "", t.TempDir())
	svc.client = &http.Client{Transport: stubTransport{status: http.StatusOK, body: sevenDayOpenMeteoJSON}}

	data, err := svc.FetchForLocation("52.52", "13.41")
	if err != nil {
		t.Fatalf("fetch 7-day forecast: %v", err)
	}
	if len(data.Daily) != 7 {
		t.Fatalf("expected 7 daily forecast entries, got %d", len(data.Daily))
	}
	// 2026-07-15 is a Wednesday -> full German name.
	if data.Daily[0].Weekday != "Mittwoch" {
		t.Errorf("first forecast weekday = %q, want German %q", data.Daily[0].Weekday, "Mittwoch")
	}
	// weathercode 2 -> German condition text.
	if data.Daily[0].Desc != "Teilweise bewölkt" {
		t.Errorf("first forecast desc = %q, want German %q", data.Daily[0].Desc, "Teilweise bewölkt")
	}
}
