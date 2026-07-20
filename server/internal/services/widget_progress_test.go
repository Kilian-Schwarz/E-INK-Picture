package services

import (
	"strings"
	"testing"
	"time"

	"e-ink-picture/server/internal/services/widgets"
)

// newProgressService builds a bare PreviewService with a frozen clock. No
// other service is wired: fillProgressContent must be purely local (no
// network, no weather/hass dependency).
func newProgressService(frozen time.Time) *PreviewService {
	s := &PreviewService{}
	s.now = func() time.Time { return frozen }
	return s
}

func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("timezone database unavailable: %v", err)
	}
	return loc
}

func progressContent(t *testing.T, at time.Time, props map[string]any) string {
	t.Helper()
	content, ok := newProgressService(at).WidgetTextContent("widget_progress", props)
	if !ok {
		t.Fatal("WidgetTextContent(widget_progress) returned ok == false")
	}
	return content
}

// TestProgressPeriodMath pins the exact output strings for year/month/week/day
// including leap years, the ISO-8601 week boundary and both DST transitions.
func TestProgressPeriodMath(t *testing.T) {
	berlin := mustLoadLocation(t, "Europe/Berlin")

	tests := []struct {
		name   string
		at     time.Time
		period string
		layout string
		want   string
	}{
		// Year boundaries.
		{"year start", time.Date(2026, 1, 1, 0, 0, 0, 0, berlin), "year", "bar_percent", "[--------------------] 0%"},
		{"year start count", time.Date(2026, 1, 1, 0, 0, 0, 0, berlin), "year", "count", "Tag 1 von 365"},
		{"year end", time.Date(2026, 12, 31, 23, 59, 59, 0, berlin), "year", "bar_percent", "[###################-] 99%"},
		{"year end count", time.Date(2026, 12, 31, 23, 59, 59, 0, berlin), "year", "count", "Tag 365 von 365"},
		// 200.5 of 365 days = 0.5492 -> 54%, filled = int(0.5492*20) = 10.
		// (The F7 spec's example showed 11 '#', which contradicts its own
		// truncating filled rule; the rule wins.)
		{"year mid full", time.Date(2026, 7, 20, 12, 0, 0, 0, berlin), "year", "full", "Tag 201 von 365\n[##########----------] 54%"},

		// Leap year.
		{"leap year end", time.Date(2028, 12, 31, 12, 0, 0, 0, berlin), "year", "count", "Tag 366 von 366"},
		{"leap day", time.Date(2028, 2, 29, 0, 0, 0, 0, berlin), "year", "count", "Tag 60 von 366"},
		{"leap february", time.Date(2028, 2, 15, 0, 0, 0, 0, berlin), "month", "count", "Tag 15 von 29"},
		{"common february", time.Date(2026, 2, 15, 0, 0, 0, 0, berlin), "month", "count", "Tag 15 von 28"},

		// ISO-8601 week: Monday is day 1, Sunday is day 7.
		{"monday start", time.Date(2026, 7, 20, 0, 0, 0, 0, berlin), "week", "count", "Tag 1 von 7"},
		{"monday start pct", time.Date(2026, 7, 20, 0, 0, 0, 0, berlin), "week", "percent", "0%"},
		{"sunday end", time.Date(2026, 7, 19, 23, 59, 0, 0, berlin), "week", "count", "Tag 7 von 7"},
		{"sunday end pct", time.Date(2026, 7, 19, 23, 59, 0, 0, berlin), "week", "percent", "99%"},

		// DST: 2026-03-29 is a 23h day, 2026-10-25 a 25h day in Europe/Berlin.
		// Note both the total AND the elapsed time shift: 12:00 local is 11h
		// after local midnight on the short day and 13h on the long day, so
		// the ratios are 11/23 = 47% and 13/25 = 52% (not 12/23 and 12/25 as
		// the F7 spec's AC7 example assumed).
		{"dst short day", time.Date(2026, 3, 29, 12, 0, 0, 0, berlin), "day", "percent", "47%"},
		{"dst long day", time.Date(2026, 10, 25, 12, 0, 0, 0, berlin), "day", "percent", "52%"},
		{"utc same day", time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC), "day", "percent", "50%"},
		{"day count", time.Date(2026, 7, 20, 12, 0, 0, 0, berlin), "day", "count", "Stunde 13 von 24"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			props := map[string]any{"period": tc.period, "layout": tc.layout}
			if got := progressContent(t, tc.at, props); got != tc.want {
				t.Errorf("period=%s layout=%s\n got: %q\nwant: %q", tc.period, tc.layout, got, tc.want)
			}
		})
	}
}

// TestProgressPeriodDurations verifies the boundaries are built via time.Date
// (not 24h arithmetic), so DST days are 23h/25h long.
func TestProgressPeriodDurations(t *testing.T) {
	berlin := mustLoadLocation(t, "Europe/Berlin")

	tests := []struct {
		name string
		at   time.Time
		want time.Duration
	}{
		{"spring forward", time.Date(2026, 3, 29, 12, 0, 0, 0, berlin), 23 * time.Hour},
		{"fall back", time.Date(2026, 10, 25, 12, 0, 0, 0, berlin), 25 * time.Hour},
		{"utc spring", time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC), 24 * time.Hour},
		{"utc fall", time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC), 24 * time.Hour},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			start, end := progressPeriodBounds(tc.at, "day")
			if got := end.Sub(start); got != tc.want {
				t.Errorf("day duration = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestProgressNeverCompletesWithinPeriod covers AC4: no instant inside 2026
// yields 100% or a day count beyond the year length.
func TestProgressNeverCompletesWithinPeriod(t *testing.T) {
	berlin := mustLoadLocation(t, "Europe/Berlin")
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, berlin)
	end := time.Date(2027, 1, 1, 0, 0, 0, 0, berlin)

	for at.Before(end) {
		ratio := progressRatio(at, "year")
		if ratio < 0 || ratio >= 1 {
			t.Fatalf("ratio at %v = %v, want [0,1)", at, ratio)
		}
		current, total := progressCounts(at, "year")
		if current < 1 || current > total {
			t.Fatalf("count at %v = %d von %d", at, current, total)
		}
		at = at.Add(7 * time.Hour)
	}
}

// TestProgressTimezoneHandling covers AC8: an explicit timezone shifts the
// period, an invalid one falls back silently without an error.
func TestProgressTimezoneHandling(t *testing.T) {
	mustLoadLocation(t, "Europe/Berlin")
	at := time.Date(2025, 12, 31, 23, 30, 0, 0, time.UTC)

	berlinCount := progressContent(t, at, map[string]any{"period": "year", "layout": "count", "timezone": "Europe/Berlin"})
	if berlinCount != "Tag 1 von 365" {
		t.Errorf("Europe/Berlin count = %q, want %q (already 2026)", berlinCount, "Tag 1 von 365")
	}

	utcCount := progressContent(t, at, map[string]any{"period": "year", "layout": "count", "timezone": "UTC"})
	if utcCount != "Tag 365 von 365" {
		t.Errorf("UTC count = %q, want %q (still 2025)", utcCount, "Tag 365 von 365")
	}

	invalid := progressContent(t, at, map[string]any{"period": "year", "layout": "count", "timezone": "Mars/Olympus"})
	if invalid == "" {
		t.Error("invalid timezone must fall back silently, got empty content")
	}
}

// TestProgressDefaultsAndInvalidInput covers AC9: nil/empty props, unknown
// period, out-of-range barWidth and an empty custom template.
func TestProgressDefaultsAndInvalidInput(t *testing.T) {
	at := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	defaultOut := "[##########----------] 54%"

	// Numbers are float64 because properties always arrive JSON-decoded
	// (GetPropInt handles float64 and string, matching Element.Properties).
	tests := []struct {
		name  string
		props map[string]any
		want  string
	}{
		{"nil props", nil, defaultOut},
		{"empty props", map[string]any{}, defaultOut},
		{"unknown period", map[string]any{"period": "fortnight"}, defaultOut},
		{"unknown layout", map[string]any{"layout": "hologram"}, defaultOut},
		{"barWidth zero", map[string]any{"barWidth": float64(0), "layout": "bar"}, "[##---]"},
		{"barWidth negative", map[string]any{"barWidth": float64(-5), "layout": "bar"}, "[##---]"},
		{"barWidth huge", map[string]any{"barWidth": float64(9999), "layout": "bar"}, "[" + strings.Repeat("#", 32) + strings.Repeat("-", 28) + "]"},
		{"empty custom template", map[string]any{"layout": "custom", "customTemplate": ""}, defaultOut},
		{"custom placeholders", map[string]any{"layout": "custom", "customTemplate": "%period%: %current%/%total%, noch %remaining% (%percent%)"}, "Jahr: 201/365, noch 164 (54%)"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := progressContent(t, at, tc.props)
			if got == "" {
				t.Fatal("content must never be empty")
			}
			if got != tc.want {
				t.Errorf("\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}

// TestProgressZeroValueServiceDoesNotPanic guards the clock seam: a
// PreviewService built without NewPreviewService has a nil now func.
func TestProgressZeroValueServiceDoesNotPanic(t *testing.T) {
	s := &PreviewService{}
	content, ok := s.WidgetTextContent("widget_progress", nil)
	if !ok {
		t.Fatal("ok == false")
	}
	if content == "" {
		t.Error("zero-value service produced empty content")
	}
}

// TestProgressASCIIOnly covers AC12: the bar layouts contain no byte >= 0x80,
// in particular no Unicode block characters the fallback font lacks.
func TestProgressASCIIOnly(t *testing.T) {
	at := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	for _, layout := range []string{"bar", "percent", "bar_percent"} {
		content := progressContent(t, at, map[string]any{"layout": layout})
		for i := 0; i < len(content); i++ {
			if content[i] >= 0x80 {
				t.Errorf("layout %s: non-ASCII byte %#x at %d in %q", layout, content[i], i, content)
			}
		}
	}
}

// TestProgressLayoutsRegistered covers registration point 8: without these
// entries the properties panel's layout dropdown stays empty.
func TestProgressLayoutsRegistered(t *testing.T) {
	layouts := widgets.GetLayouts("widget_progress")
	want := []string{"bar", "percent", "bar_percent", "count", "full", "custom"}
	if len(layouts) != len(want) {
		t.Fatalf("got %d layouts, want %d", len(layouts), len(want))
	}
	for i, id := range want {
		if layouts[i].ID != id {
			t.Errorf("layout[%d].ID = %q, want %q", i, layouts[i].ID, id)
		}
		if layouts[i].Name == "" {
			t.Errorf("layout %q has no name", id)
		}
	}

	placeholders := widgets.Placeholders("widget_progress")
	for _, p := range []string{"%bar%", "%percent%", "%current%", "%total%", "%remaining%", "%period%"} {
		if !containsString(placeholders, p) {
			t.Errorf("placeholder %q missing", p)
		}
	}
}

func containsString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
