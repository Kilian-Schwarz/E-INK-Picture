package services

import (
	"testing"
	"time"
)

// TestGermanWeekdayFull proves the full German weekday name is returned for
// each day of the week (AC1/AC3).
func TestGermanWeekdayFull(t *testing.T) {
	// 2026-07-13 is a Monday; walk a full week from there.
	base := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	want := []string{"Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag", "Sonntag"}
	for i, w := range want {
		got := germanWeekday(base.AddDate(0, 0, i))
		if got != w {
			t.Errorf("germanWeekday(+%dd) = %q, want %q", i, got, w)
		}
	}
}

// TestGermanShortWeekdayFromName proves the full German name maps to the proper
// two-letter abbreviation (Mo/Di/Mi/Do/Fr/Sa/So), not a naive 3-byte slice, and
// that an unknown input passes through unchanged (AC1).
func TestGermanShortWeekdayFromName(t *testing.T) {
	cases := map[string]string{
		"Montag":     "Mo",
		"Dienstag":   "Di",
		"Mittwoch":   "Mi",
		"Donnerstag": "Do",
		"Freitag":    "Fr",
		"Samstag":    "Sa",
		"Sonntag":    "So",
	}
	for full, short := range cases {
		if got := germanShortWeekdayFromName(full); got != short {
			t.Errorf("germanShortWeekdayFromName(%q) = %q, want %q", full, got, short)
		}
	}
	if got := germanShortWeekdayFromName("Monday"); got != "Monday" {
		t.Errorf("unknown weekday should pass through, got %q", got)
	}
}

// TestFillForecastGerman proves the forecast layouts render German full and
// short weekday names plus German condition text end-to-end (AC1/AC2/AC4).
func TestFillForecastGerman(t *testing.T) {
	previewSvc := newGoldenPreviewService(t.TempDir())

	previewSvc.weather.mu.Lock()
	previewSvc.weather.cache["52.52,13.41"] = &weatherCacheEntry{
		data: &WeatherData{
			Daily: []DailyForecast{
				{Weekday: "Montag", Min: 10, Max: 20, Desc: "Teilweise bewölkt", Date: "2026-07-13"},
				{Weekday: "Dienstag", Min: 11, Max: 21, Desc: "Bedeckt", Date: "2026-07-14"},
			},
		},
		cachedAt: time.Now(),
	}
	previewSvc.weather.mu.Unlock()

	base := map[string]any{"latitude": "52.52", "longitude": "13.41", "days": float64(2)}

	compactProps := map[string]any{}
	for k, v := range base {
		compactProps[k] = v
	}
	compactProps["layout"] = "compact_row"
	if got := previewSvc.fillForecastContent(compactProps); got != "Mo 10/20°\nDi 11/21°" {
		t.Errorf("compact_row = %q, want %q", got, "Mo 10/20°\nDi 11/21°")
	}

	verticalProps := map[string]any{}
	for k, v := range base {
		verticalProps[k] = v
	}
	verticalProps["layout"] = "vertical"
	want := "Montag: 10-20°C Teilweise bewölkt\nDienstag: 11-21°C Bedeckt"
	if got := previewSvc.fillForecastContent(verticalProps); got != want {
		t.Errorf("vertical = %q, want %q", got, want)
	}
}
