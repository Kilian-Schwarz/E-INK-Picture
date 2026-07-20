package services

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
	"time"

	"e-ink-picture/server/internal/services/widgets"
)

// newHolidaysService builds a bare PreviewService with a frozen clock. Nothing
// else is wired: fillHolidaysContent must be purely local — no network, no
// weather/hass dependency, no data seam (F4 AC20).
func newHolidaysService(frozen time.Time) *PreviewService {
	s := &PreviewService{}
	s.now = func() time.Time { return frozen }
	return s
}

func holidaysContent(t *testing.T, at time.Time, props map[string]any) string {
	t.Helper()
	content, ok := newHolidaysService(at).WidgetTextContent("widget_holidays", props)
	if !ok {
		t.Fatal("WidgetTextContent(widget_holidays) returned ok == false")
	}
	return content
}

// --- Domain: Easter -----------------------------------------------------

// TestEasterSunday covers AC5: the pinned reference dates plus a 201-year
// invariant. The invariant is the part that catches a sign error in a
// copy-pasted formula, which the reference dates alone can let through.
func TestEasterSunday(t *testing.T) {
	tests := []struct {
		year  int
		month time.Month
		day   int
	}{
		{2024, time.March, 31},
		{2025, time.April, 20},
		{2026, time.April, 5},
		{2027, time.March, 28},
		{2028, time.April, 16},
		{2030, time.April, 21},
		{2038, time.April, 25}, // latest possible date
	}
	for _, tc := range tests {
		got := easterSunday(tc.year, time.UTC)
		want := time.Date(tc.year, tc.month, tc.day, 0, 0, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("easterSunday(%d) = %s, want %s", tc.year, got.Format("2006-01-02"), want.Format("2006-01-02"))
		}
	}

	// Invariant over 201 years: always a Sunday, always within [22.03., 25.04.].
	for y := 1900; y <= 2100; y++ {
		e := easterSunday(y, time.UTC)
		if e.Weekday() != time.Sunday {
			t.Errorf("easterSunday(%d) = %s is a %s, want Sunday", y, e.Format("2006-01-02"), e.Weekday())
		}
		lo := time.Date(y, time.March, 22, 0, 0, 0, 0, time.UTC)
		hi := time.Date(y, time.April, 25, 0, 0, 0, 0, time.UTC)
		if e.Before(lo) || e.After(hi) {
			t.Errorf("easterSunday(%d) = %s outside [22.03., 25.04.]", y, e.Format("2006-01-02"))
		}
	}
}

// TestBussUndBettag covers AC8, including the strict edge: when 23 November is
// itself a Wednesday the holiday is the 16th, not the 23rd.
func TestBussUndBettag(t *testing.T) {
	tests := []struct {
		year int
		day  int
	}{
		{2022, 16}, // 23.11.2022 is a Wednesday -> strict edge
		{2025, 19},
		{2026, 18},
		{2027, 17},
		{2028, 22},
	}
	for _, tc := range tests {
		got := bussUndBettag(tc.year, time.UTC)
		want := time.Date(tc.year, time.November, tc.day, 0, 0, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("bussUndBettag(%d) = %s, want %s", tc.year, got.Format("2006-01-02"), want.Format("2006-01-02"))
		}
	}

	for y := 2000; y <= 2100; y++ {
		d := bussUndBettag(y, time.UTC)
		if d.Weekday() != time.Wednesday {
			t.Errorf("bussUndBettag(%d) = %s is a %s, want Wednesday", y, d.Format("2006-01-02"), d.Weekday())
		}
		if d.Month() != time.November || d.Day() < 16 || d.Day() > 22 {
			t.Errorf("bussUndBettag(%d) = %s outside [16.11., 22.11.]", y, d.Format("2006-01-02"))
		}
	}
}

// --- Domain: rule table --------------------------------------------------

func holidayNames(list []holiday) map[string]time.Time {
	out := make(map[string]time.Time, len(list))
	for _, h := range list {
		out[h.name] = h.date
	}
	return out
}

// TestHolidaysPerState covers AC6 and AC7: the per-state counts, the exact
// membership of the rules that are easy to get wrong, the subset invariant that
// justifies the DE default, and the absence of municipal holidays.
func TestHolidaysPerState(t *testing.T) {
	const year = 2026

	counts := map[string]int{
		"DE": 9,
		"BE": 10, "BB": 10, "HB": 10, "HH": 10, "HE": 10, "NI": 10, "SH": 10,
		"MV": 11, "NW": 11, "RP": 11, "SN": 11, "ST": 11, "TH": 11,
		"BW": 12, "BY": 12, "SL": 12,
	}
	// holidayStates (the slice this matrix enumerates) and holidayStateNames
	// (the map normalizeHolidayState validates against) are two sources for the
	// same set. Comparing only their LENGTHS would let a code that exists in the
	// map but not the slice be accepted at runtime while every test below
	// silently skips it. Compare membership in both directions.
	for _, state := range holidayStates {
		if _, ok := holidayStateNames[state]; !ok {
			t.Errorf("holidayStates lists %q but holidayStateNames has no name for it — normalizeHolidayState would reject a code this test matrix believes is valid", state)
		}
		if _, ok := counts[state]; !ok {
			t.Errorf("holidayStates lists %q but the count table has no expectation for it", state)
		}
	}
	listed := make(map[string]bool, len(holidayStates))
	for _, state := range holidayStates {
		listed[state] = true
	}
	for code := range holidayStateNames {
		if !listed[code] {
			t.Errorf("holidayStateNames accepts %q but holidayStates does not list it — the whole per-state matrix would skip that code", code)
		}
	}
	if len(counts) != len(holidayStates) {
		t.Fatalf("count table has %d entries, holidayStates has %d", len(counts), len(holidayStates))
	}
	for _, state := range holidayStates {
		got := holidaysForYear(year, state, time.UTC)
		if len(got) != counts[state] {
			t.Errorf("%s: %d holidays in %d, want %d (%v)", state, len(got), year, counts[state], holidayNames(got))
		}
	}

	// Unknown code behaves exactly like the DE sentinel, no error, no panic.
	if a, b := holidaysForYear(year, "XX", time.UTC), holidaysForYear(year, "DE", time.UTC); len(a) != len(b) {
		t.Errorf("unknown state code yielded %d holidays, want the DE set (%d)", len(a), len(b))
	}

	// Subset invariant: every nationwide holiday exists in every Land. This is
	// what makes the DE default safe — it can only ever omit, never overclaim.
	nationwide := holidayNames(holidaysForYear(year, "DE", time.UTC))
	for _, state := range holidayStates[1:] {
		inState := holidayNames(holidaysForYear(year, state, time.UTC))
		for name, date := range nationwide {
			if d, ok := inState[name]; !ok || !d.Equal(date) {
				t.Errorf("%s is missing the nationwide holiday %q on %s", state, name, date.Format("2006-01-02"))
			}
		}
	}

	// Exact membership per rule. Easter 2026 = 05.04., so Fronleichnam = 04.06.
	membership := []struct {
		name   string
		date   time.Time
		states []string
	}{
		{"Fronleichnam", time.Date(2026, time.June, 4, 0, 0, 0, 0, time.UTC), []string{"BW", "BY", "HE", "NW", "RP", "SL"}},
		{"Allerheiligen", time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC), []string{"BW", "BY", "NW", "RP", "SL"}},
		{"Reformationstag", time.Date(2026, time.October, 31, 0, 0, 0, 0, time.UTC), []string{"BB", "HB", "HH", "MV", "NI", "SH", "SN", "ST", "TH"}},
		{"Buß- und Bettag", time.Date(2026, time.November, 18, 0, 0, 0, 0, time.UTC), []string{"SN"}},
		{"Mariä Himmelfahrt", time.Date(2026, time.August, 15, 0, 0, 0, 0, time.UTC), []string{"SL"}},
		{"Weltkindertag", time.Date(2026, time.September, 20, 0, 0, 0, 0, time.UTC), []string{"TH"}},
		{"Internationaler Frauentag", time.Date(2026, time.March, 8, 0, 0, 0, 0, time.UTC), []string{"BE", "MV"}},
		{"Heilige Drei Könige", time.Date(2026, time.January, 6, 0, 0, 0, 0, time.UTC), []string{"BW", "BY", "ST"}},
	}
	for _, m := range membership {
		want := map[string]bool{}
		for _, s := range m.states {
			want[s] = true
		}
		for _, state := range holidayStates {
			date, present := holidayNames(holidaysForYear(year, state, time.UTC))[m.name]
			if present != want[state] {
				t.Errorf("%q present in %s = %v, want %v", m.name, state, present, want[state])
			}
			if present && !date.Equal(m.date) {
				t.Errorf("%q in %s on %s, want %s", m.name, state, date.Format("2006-01-02"), m.date.Format("2006-01-02"))
			}
		}
	}

	// Neither Ostersonntag nor Pfingstsonntag is listed anywhere: contested
	// between the Länder and always a Sunday anyway.
	for _, state := range holidayStates {
		for name := range holidayNames(holidaysForYear(year, state, time.UTC)) {
			if name == "Ostersonntag" || name == "Pfingstsonntag" {
				t.Errorf("%s lists %q — deliberately excluded", state, name)
			}
		}
	}

	// The Augsburger Friedensfest (08.08.) is municipal and must appear nowhere.
	for y := 2024; y <= 2030; y++ {
		for _, state := range holidayStates {
			for _, h := range holidaysForYear(y, state, time.UTC) {
				if h.date.Month() == time.August && h.date.Day() == 8 {
					t.Errorf("%s %d contains a holiday on 08.08. (%q) — municipal, must be out of scope", state, y, h.name)
				}
			}
		}
	}
}

// TestDaysBetweenCalendarDays covers AC10 (leap year) and the AC11 DST control:
// daysBetween counts CALENDAR days via UTC midnights, so the two DST switch
// days of the year cannot shift it.
func TestDaysBetweenCalendarDays(t *testing.T) {
	berlin := mustLoadLocation(t, "Europe/Berlin")

	tests := []struct {
		name string
		from time.Time
		to   time.Time
		want int
	}{
		{"leap year counts 29.02.", time.Date(2028, 2, 28, 0, 0, 0, 0, time.UTC), time.Date(2028, 4, 14, 0, 0, 0, 0, time.UTC), 46},
		{"common year", time.Date(2027, 2, 28, 0, 0, 0, 0, time.UTC), time.Date(2027, 4, 14, 0, 0, 0, 0, time.UTC), 45},
		{"same day", time.Date(2026, 7, 20, 23, 59, 0, 0, berlin), time.Date(2026, 7, 20, 0, 0, 0, 0, berlin), 0},
		{"tomorrow", time.Date(2026, 7, 20, 23, 59, 0, 0, berlin), time.Date(2026, 7, 21, 0, 0, 0, 0, berlin), 1},
		{"past is negative", time.Date(2026, 7, 20, 0, 0, 0, 0, berlin), time.Date(2026, 7, 19, 0, 0, 0, 0, berlin), -1},
		// Spring forward (2026-03-29, a 23h day) and fall back (2026-10-25, a
		// 25h day). A 24*time.Hour quotient would be off by one across these.
		{"across spring forward", time.Date(2026, 3, 28, 12, 0, 0, 0, berlin), time.Date(2026, 3, 30, 12, 0, 0, 0, berlin), 2},
		{"across fall back", time.Date(2026, 10, 24, 12, 0, 0, 0, berlin), time.Date(2026, 10, 26, 12, 0, 0, 0, berlin), 2},
		{"spring forward whole march", time.Date(2026, 3, 1, 0, 0, 0, 0, berlin), time.Date(2026, 4, 1, 0, 0, 0, 0, berlin), 31},
		{"fall back whole october", time.Date(2026, 10, 1, 0, 0, 0, 0, berlin), time.Date(2026, 11, 1, 0, 0, 0, 0, berlin), 31},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := daysBetween(tc.from, tc.to); got != tc.want {
				t.Errorf("daysBetween = %d, want %d", got, tc.want)
			}
		})
	}
}

// --- Content ------------------------------------------------------------

// TestHolidaysNextLayouts pins the reference output of the layouts at the
// golden instant (2026-07-20 12:00 Europe/Berlin, a Monday). 03.10.2026 is 75
// days later and a Saturday.
func TestHolidaysNextLayouts(t *testing.T) {
	mustLoadLocation(t, "Europe/Berlin")
	at := goldenNow

	tests := []struct {
		name  string
		props map[string]any
		want  string
	}{
		{
			"next",
			map[string]any{"state": "DE", "layout": "next", "timezone": "Europe/Berlin"},
			"Tag der Deutschen Einheit\nSa, 03.10.2026",
		},
		{
			"next_countdown",
			map[string]any{"state": "DE", "layout": "next_countdown", "timezone": "Europe/Berlin"},
			"Tag der Deutschen Einheit\nSa, 03.10.2026 (in 75 Tagen)",
		},
		{
			"list saxony",
			map[string]any{"state": "SN", "layout": "list", "count": float64(3), "timezone": "Europe/Berlin"},
			"Sa, 03.10.2026  Tag der Deutschen Einheit\n" +
				"Sa, 31.10.2026  Reformationstag\n" +
				"Mi, 18.11.2026  Buß- und Bettag",
		},
		{
			"custom default template",
			map[string]any{"state": "DE", "layout": "custom", "customTemplate": "", "timezone": "Europe/Berlin"},
			"Tag der Deutschen Einheit (03.10.2026)",
		},
		{
			"custom all placeholders",
			map[string]any{"state": "BY", "layout": "custom", "timezone": "Europe/Berlin",
				"customTemplate": "%state%: %name% am %weekday%, %date% (noch %days% Tage)"},
			"Bayern: Tag der Deutschen Einheit am Sa, 03.10.2026 (noch 75 Tage)",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := holidaysContent(t, at, tc.props); got != tc.want {
				t.Errorf("\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}

// TestHolidaysYearRollover covers AC9, the off-by-one trap: in late December
// the next holiday lies in the FOLLOWING year.
func TestHolidaysYearRollover(t *testing.T) {
	berlin := mustLoadLocation(t, "Europe/Berlin")

	t.Run("after christmas the next holiday is next new year", func(t *testing.T) {
		at := time.Date(2026, 12, 27, 9, 0, 0, 0, berlin)
		props := map[string]any{"state": "DE", "layout": "next_countdown", "timezone": "Europe/Berlin"}
		want := "Neujahr\nFr, 01.01.2027 (in 5 Tagen)"
		if got := holidaysContent(t, at, props); got != want {
			t.Errorf("\n got: %q\nwant: %q", got, want)
		}
	})

	t.Run("a holiday that is today stays in", func(t *testing.T) {
		at := time.Date(2026, 12, 25, 9, 0, 0, 0, berlin)
		props := map[string]any{"state": "DE", "layout": "next_countdown", "timezone": "Europe/Berlin"}
		want := "1. Weihnachtstag\nFr, 25.12.2026 (heute)"
		if got := holidaysContent(t, at, props); got != want {
			t.Errorf("\n got: %q\nwant: %q", got, want)
		}
	})

	t.Run("list crosses the year boundary", func(t *testing.T) {
		at := time.Date(2026, 12, 25, 9, 0, 0, 0, berlin)
		props := map[string]any{"state": "DE", "layout": "list", "count": float64(3), "timezone": "Europe/Berlin"}
		want := "Fr, 25.12.2026  1. Weihnachtstag\n" +
			"Sa, 26.12.2026  2. Weihnachtstag\n" +
			"Fr, 01.01.2027  Neujahr"
		if got := holidaysContent(t, at, props); got != want {
			t.Errorf("\n got: %q\nwant: %q", got, want)
		}
	})

	// The three-year candidate window: on 31 December with count = 10 there
	// must still be ten distinct, non-empty lines.
	t.Run("ten entries on new years eve", func(t *testing.T) {
		at := time.Date(2026, 12, 31, 23, 0, 0, 0, berlin)
		props := map[string]any{"state": "DE", "layout": "list", "count": float64(10), "timezone": "Europe/Berlin"}
		lines := strings.Split(holidaysContent(t, at, props), "\n")
		if len(lines) != 10 {
			t.Fatalf("got %d lines, want 10:\n%s", len(lines), strings.Join(lines, "\n"))
		}
		seen := map[string]bool{}
		for i, line := range lines {
			if strings.TrimSpace(line) == "" {
				t.Errorf("line %d is empty", i)
			}
			if seen[line] {
				t.Errorf("duplicate line %d: %q", i, line)
			}
			seen[line] = true
		}
	})
}

// TestHolidaysLeapYear covers AC10 end to end: Karfreitag 2028 is 14.04. and
// 29.02.2028 is counted.
func TestHolidaysLeapYear(t *testing.T) {
	berlin := mustLoadLocation(t, "Europe/Berlin")
	props := map[string]any{"state": "DE", "layout": "next_countdown", "timezone": "Europe/Berlin"}

	at := time.Date(2028, 2, 28, 12, 0, 0, 0, berlin)
	want := "Karfreitag\nFr, 14.04.2028 (in 46 Tagen)"
	if got := holidaysContent(t, at, props); got != want {
		t.Errorf("\n got: %q\nwant: %q", got, want)
	}

	// 29 February is a valid input instant and must not panic.
	leapDay := time.Date(2028, 2, 29, 8, 0, 0, 0, berlin)
	if got := holidaysContent(t, leapDay, props); got == "" {
		t.Error("content on 29.02.2028 must not be empty")
	}
}

// TestHolidaysCountdownWording covers AC11: heute / morgen / in N Tagen, with
// the transition pinned to local midnight rather than 24-hour blocks.
func TestHolidaysCountdownWording(t *testing.T) {
	berlin := mustLoadLocation(t, "Europe/Berlin")

	if got := holidayCountdownText(0); got != "heute" {
		t.Errorf("countdown(0) = %q, want %q", got, "heute")
	}
	if got := holidayCountdownText(1); got != "morgen" {
		t.Errorf("countdown(1) = %q, want %q", got, "morgen")
	}
	if got := holidayCountdownText(2); got != "in 2 Tagen" {
		t.Errorf("countdown(2) = %q, want %q", got, "in 2 Tagen")
	}

	props := map[string]any{"state": "DE", "layout": "next_countdown", "timezone": "Europe/Berlin"}

	// 23:59 the day before -> morgen; 00:01 on the day -> heute.
	eve := holidaysContent(t, time.Date(2026, 10, 2, 23, 59, 0, 0, berlin), props)
	if !strings.Contains(eve, "(morgen)") {
		t.Errorf("23:59 on 02.10.2026 = %q, want the morgen wording", eve)
	}
	justAfterMidnight := holidaysContent(t, time.Date(2026, 10, 3, 0, 1, 0, 0, berlin), props)
	if !strings.Contains(justAfterMidnight, "(heute)") {
		t.Errorf("00:01 on 03.10.2026 = %q, want the heute wording", justAfterMidnight)
	}

	// "in 1 Tag" must never be produced — the singular is covered by "morgen".
	for _, state := range holidayStates {
		for day := 1; day <= 365; day++ {
			at := time.Date(2026, 1, day, 12, 0, 0, 0, berlin)
			out := holidaysContent(t, at, map[string]any{"state": state, "layout": "next_countdown", "timezone": "Europe/Berlin"})
			if strings.Contains(out, "in 1 Tag)") {
				t.Fatalf("%s at %s produced the singular form: %q", state, at.Format("2006-01-02"), out)
			}
		}
	}
}

// TestHolidaysDSTCountdown covers the AC11 DST control end to end: a countdown
// spanning either DST switch is not shifted by a day.
func TestHolidaysDSTCountdown(t *testing.T) {
	berlin := mustLoadLocation(t, "Europe/Berlin")
	props := map[string]any{"state": "DE", "layout": "custom", "customTemplate": "%days%", "timezone": "Europe/Berlin"}

	// From 2026-03-01 the next nationwide holiday is Karfreitag, 03.04.2026
	// (Easter 05.04.). 31 (March) + 3 = 33 days, across the 29.03. switch.
	if got := holidaysContent(t, time.Date(2026, 3, 1, 12, 0, 0, 0, berlin), props); got != "33" {
		t.Errorf("days across spring forward = %q, want %q", got, "33")
	}
	// From 2026-10-01 the next nationwide holiday is 03.10.2026: 2 days.
	// From 2026-10-26 (after fall back) the next is 25.12.2026: 5 + 30 + 25 = 60.
	if got := holidaysContent(t, time.Date(2026, 10, 26, 12, 0, 0, 0, berlin), props); got != "60" {
		t.Errorf("days after fall back = %q, want %q", got, "60")
	}
}

// TestHolidaysTimezoneHandling covers AC12: the zone decides which day "today"
// is, and an invalid zone falls back silently.
func TestHolidaysTimezoneHandling(t *testing.T) {
	mustLoadLocation(t, "Europe/Berlin")
	at := time.Date(2026, 12, 31, 23, 30, 0, 0, time.UTC)

	berlin := holidaysContent(t, at, map[string]any{"state": "DE", "layout": "next_countdown", "timezone": "Europe/Berlin"})
	if !strings.Contains(berlin, "(heute)") || !strings.Contains(berlin, "01.01.2027") {
		t.Errorf("Europe/Berlin = %q, want Neujahr 01.01.2027 (heute) — locally it is already 2027", berlin)
	}

	utc := holidaysContent(t, at, map[string]any{"state": "DE", "layout": "next_countdown", "timezone": "UTC"})
	if !strings.Contains(utc, "(morgen)") || !strings.Contains(utc, "01.01.2027") {
		t.Errorf("UTC = %q, want Neujahr 01.01.2027 (morgen) — it is still 31.12.2026", utc)
	}

	invalid := holidaysContent(t, at, map[string]any{"state": "DE", "layout": "next_countdown", "timezone": "Mars/Olympus"})
	if invalid == "" {
		t.Error("an invalid timezone must fall back silently, got empty content")
	}
}

// TestHolidaysDefaultsAndInvalidInput covers AC14.
func TestHolidaysDefaultsAndInvalidInput(t *testing.T) {
	at := goldenNow
	defaultOut := "Tag der Deutschen Einheit\nSa, 03.10.2026 (in 75 Tagen)"

	tests := []struct {
		name  string
		props map[string]any
		want  string
	}{
		{"nil props", nil, defaultOut},
		{"empty props", map[string]any{}, defaultOut},
		{"unknown state word", map[string]any{"state": "Bayern"}, defaultOut},
		{"unknown state code", map[string]any{"state": "xx"}, defaultOut},
		{"empty state", map[string]any{"state": ""}, defaultOut},
		{"unknown layout", map[string]any{"layout": "wolkig"}, defaultOut},
		{"empty custom template", map[string]any{"layout": "custom", "customTemplate": ""}, "Tag der Deutschen Einheit (03.10.2026)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := holidaysContent(t, at, tc.props)
			if got == "" {
				t.Fatal("content must never be empty")
			}
			if got != tc.want {
				t.Errorf("\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}

	// count is clamped to [1, 10]. Numbers are float64 because properties
	// always arrive JSON-decoded — see TestHolidaysTestsUseFloat64Props.
	clamped := []struct {
		name  string
		count float64
		lines int
	}{
		{"zero", float64(0), 1},
		{"negative", float64(-5), 1},
		{"huge", float64(9999), 10},
		{"in range", float64(4), 4},
	}
	for _, tc := range clamped {
		t.Run("count "+tc.name, func(t *testing.T) {
			props := map[string]any{"state": "DE", "layout": "list", "count": tc.count}
			got := strings.Split(holidaysContent(t, at, props), "\n")
			if len(got) != tc.lines {
				t.Errorf("count=%v produced %d lines, want %d", tc.count, len(got), tc.lines)
			}
		})
	}
}

// TestHolidaysZeroValueServiceDoesNotPanic guards the clock seam: a
// PreviewService built without NewPreviewService has a nil now func.
func TestHolidaysZeroValueServiceDoesNotPanic(t *testing.T) {
	s := &PreviewService{}
	for _, layout := range []string{"next", "next_countdown", "list", "custom"} {
		content, ok := s.WidgetTextContent("widget_holidays", map[string]any{"layout": layout})
		if !ok {
			t.Fatalf("layout %s: ok == false", layout)
		}
		if content == "" {
			t.Errorf("layout %s: zero-value service produced empty content", layout)
		}
	}
}

// TestHolidaysCharsetSafe covers AC16: every rune below U+0100, so German
// umlauts and ß are fine but an en dash, em dash, ellipsis or block character
// — the things one types by accident when formatting "Datum - Name" — cannot
// slip in. goregular lacks those glyphs; the panel would render tofu while the
// browser canvas looks correct.
func TestHolidaysCharsetSafe(t *testing.T) {
	at := goldenNow
	layouts := []string{"next", "next_countdown", "list", "custom"}

	// The en dash, em dash, ellipsis, curly quotes and block characters one
	// might list here are ALL >= U+0100, so the single range check below already
	// rejects them; an explicit list of them would be dead code. What the range
	// check does NOT cover is the other end: control characters are below
	// U+0100 and would pass it while drawing as tofu or silently breaking the
	// line layout. "\n" is the one legitimate control character — it separates
	// the lines of the next/list layouts.
	for _, layout := range layouts {
		for _, state := range holidayStates {
			props := map[string]any{
				"state": state, "layout": layout, "count": float64(10),
				"customTemplate": "%state% %name% %weekday% %date% %days%",
			}
			content := holidaysContent(t, at, props)
			for i, r := range content {
				if r >= 0x100 {
					t.Errorf("layout=%s state=%s: rune %#U at byte %d is >= U+0100 (content %q)", layout, state, r, i, content)
				}
				if r != '\n' && (r < 0x20 || r == 0x7f) {
					t.Errorf("layout=%s state=%s: control character %#U at byte %d — only \\n is allowed (content %q)", layout, state, r, i, content)
				}
			}
		}
	}

	// The state names themselves must be Latin-1 too, even though only the
	// custom layout surfaces them.
	for code, name := range holidayStateNames {
		for _, r := range name {
			if r >= 0x100 {
				t.Errorf("state name %s = %q contains %#U >= U+0100", code, name, r)
			}
		}
	}
}

// TestHolidaysLayoutsRegistered covers registration point 8 directly.
func TestHolidaysLayoutsRegistered(t *testing.T) {
	layouts := widgets.GetLayouts("widget_holidays")
	want := []string{"next", "next_countdown", "list", "custom"}
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

	placeholders := widgets.Placeholders("widget_holidays")
	for _, p := range []string{"%name%", "%date%", "%weekday%", "%days%", "%state%"} {
		if !containsString(placeholders, p) {
			t.Errorf("placeholder %q missing", p)
		}
	}

	// Every registered placeholder is really substituted, otherwise
	// TestDeadPlaceholderRegistry would gain a new entry.
	props := map[string]any{"state": "SN", "layout": "custom", "timezone": "Europe/Berlin",
		"customTemplate": strings.Join(placeholders, "|")}
	got := holidaysContent(t, goldenNow, props)
	for _, p := range placeholders {
		if strings.Contains(got, p) {
			t.Errorf("placeholder %q was not substituted: %q", p, got)
		}
	}
}

// TestHolidayRuleTableStamp covers AC22: the rule table carries a machine
// readable legal cutoff date, so its age is visible even though it cannot
// update itself.
func TestHolidayRuleTableStamp(t *testing.T) {
	stamp, err := time.Parse("2006-01-02", holidayRulesAsOf)
	if err != nil {
		t.Fatalf("holidayRulesAsOf = %q is not a parsable date: %v", holidayRulesAsOf, err)
	}
	if stamp.IsZero() {
		t.Error("holidayRulesAsOf must name a real date")
	}

	src := readWidgetHolidaysSource(t)
	if !strings.Contains(src, "holidayRulesAsOf") {
		t.Error("holidayRulesAsOf is not documented in widget_holidays.go")
	}
	// The constant must be referenced from the rule table documentation, not
	// merely declared in isolation.
	if strings.Count(src, "holidayRulesAsOf") < 2 {
		t.Error("holidayRulesAsOf is declared but never referenced from the rule table comment")
	}
}

// --- Mechanical guards ---------------------------------------------------

func readWidgetHolidaysSource(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("widget_holidays.go")
	if err != nil {
		t.Fatalf("read widget_holidays.go: %v", err)
	}
	return string(data)
}

// TestHolidaysClockReadOnce covers AC13: fillHolidaysContent reads the clock
// seam exactly once and never calls time.Now() directly. Two reads could tear
// across midnight and yield a date from one day with a countdown from another.
func TestHolidaysClockReadOnce(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "widget_holidays.go", nil, 0)
	if err != nil {
		t.Fatalf("parse widget_holidays.go: %v", err)
	}

	var target *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "fillHolidaysContent" {
			target = fn
		}
	}
	if target == nil {
		t.Fatal("fillHolidaysContent not found in widget_holidays.go")
	}

	calls := 0
	ast.Inspect(target, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "nowOrDefault" {
			calls++
		}
		return true
	})
	if calls != 1 {
		t.Errorf("fillHolidaysContent calls nowOrDefault() %d times, want exactly 1", calls)
	}

	// No direct time.Now() anywhere in the file — it would bypass the seam and
	// make the golden render non-deterministic.
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if ok && pkg.Name == "time" && sel.Sel.Name == "Now" {
			t.Errorf("widget_holidays.go calls time.Now() at %s — use s.nowOrDefault()", fset.Position(call.Pos()))
		}
		return true
	})
}

// TestHolidaysTestsUseFloat64Props covers AC15: GetPropInt decodes only float64
// and string, so a bare int literal in a property map is silently discarded and
// the default applies. Such a test can be green while asserting nothing — that
// was a real bug in the F7 tests, so it is guarded mechanically, not by
// convention.
func TestHolidaysTestsUseFloat64Props(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "widget_holidays_test.go", nil, 0)
	if err != nil {
		t.Fatalf("parse widget_holidays_test.go: %v", err)
	}

	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok || !isStringAnyMapType(lit.Type) {
			return true
		}
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			if v, ok := kv.Value.(*ast.BasicLit); ok && v.Kind == token.INT {
				t.Errorf("%s: property value %s is a bare int; GetPropInt decodes only float64 and string — wrap it as float64(%s)",
					fset.Position(v.Pos()), v.Value, v.Value)
			}
		}
		return true
	})
}

// isStringAnyMapType reports whether expr is map[string]any /
// map[string]interface{}.
func isStringAnyMapType(expr ast.Expr) bool {
	m, ok := expr.(*ast.MapType)
	if !ok {
		return false
	}
	key, ok := m.Key.(*ast.Ident)
	if !ok || key.Name != "string" {
		return false
	}
	switch v := m.Value.(type) {
	case *ast.Ident:
		return v.Name == "any"
	case *ast.InterfaceType:
		return v.Methods == nil || len(v.Methods.List) == 0
	}
	return false
}
