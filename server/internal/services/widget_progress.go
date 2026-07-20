package services

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// F7 — widget_progress: how much of the current period (year/month/week/day)
// has elapsed. Computed purely locally, no network, no dependency.
//
// The progress bar is ASCII ('[', '#', '-', ']') on purpose: the content flows
// through WidgetTextContent and is drawn by renderTextV with the embedded
// goregular fallback font, which has no U+2588/U+2591 block glyphs — those
// would render as blank space or tofu on the panel while looking fine on the
// browser canvas.

const (
	progressDefaultBarWidth = 20
	progressMinBarWidth     = 5
	progressMaxBarWidth     = 60
	progressDefaultTemplate = "%bar% %percent%"
)

// progressPeriodNames maps a period id to its German label (%period%).
var progressPeriodNames = map[string]string{
	"year":  "Jahr",
	"month": "Monat",
	"week":  "Woche",
	"day":   "Tag",
}

// fillProgressContent renders the widget_progress text content.
//
// Timezone semantics follow the widget_clock precedent: the optional
// "timezone" property (IANA name) wins, an invalid name falls back silently,
// and an empty value uses server-local time. That fallback matters in
// practice: the server usually runs as a UTC container while the panel stands
// in Europe/Berlin, so without an explicit timezone the year/day rollover is
// off by the UTC offset. All period boundaries are built in the SAME location.
func (s *PreviewService) fillProgressContent(props map[string]any) string {
	period := normalizeProgressPeriod(GetPropString(props, "period", "year"))
	layout := GetPropString(props, "layout", "bar_percent")
	barWidth := clampInt(GetPropInt(props, "barWidth", progressDefaultBarWidth), progressMinBarWidth, progressMaxBarWidth)

	// Read the clock ONCE and derive everything from that instant. Calling
	// nowOrDefault() a second time would risk a torn read across a period
	// boundary — here only the Location is taken from the first call, but this
	// function is the template for further time-dependent widgets where both
	// values matter.
	serverNow := s.nowOrDefault()

	loc := serverNow.Location()
	if tz := GetPropString(props, "timezone", ""); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	now := serverNow.In(loc)

	ratio := progressRatio(now, period)
	current, total := progressCounts(now, period)

	bar := progressBar(ratio, barWidth)
	percent := fmt.Sprintf("%d%%", int(ratio*100))
	count := progressCountText(period, current, total)

	switch layout {
	case "bar":
		return bar
	case "percent":
		return percent
	case "count":
		return count
	case "full":
		return count + "\n" + bar + " " + percent
	case "custom":
		template := GetPropString(props, "customTemplate", "")
		if template == "" {
			template = progressDefaultTemplate
		}
		return applyProgressPlaceholders(template, bar, percent, period, current, total)
	default: // bar_percent
		return bar + " " + percent
	}
}

// normalizeProgressPeriod maps an unknown period to the "year" default.
func normalizeProgressPeriod(period string) string {
	switch period {
	case "year", "month", "week", "day":
		return period
	default:
		return "year"
	}
}

// progressPeriodBounds returns the half-open interval [start, end) of the
// period containing t, in t's location.
//
// Boundaries are constructed with time.Date and subtracted from each other —
// never via 24*time.Hour arithmetic. Only that way do DST days come out as 23h
// or 25h and February as 28 or 29 days.
func progressPeriodBounds(t time.Time, period string) (start, end time.Time) {
	y, m, d := t.Date()
	loc := t.Location()

	switch period {
	case "month":
		return time.Date(y, m, 1, 0, 0, 0, 0, loc), time.Date(y, m+1, 1, 0, 0, 0, 0, loc)
	case "week":
		// ISO-8601: the week starts on Monday. time.Weekday has Sunday = 0.
		offset := (int(t.Weekday()) + 6) % 7
		monday := time.Date(y, m, d-offset, 0, 0, 0, 0, loc)
		return monday, time.Date(y, m, d-offset+7, 0, 0, 0, 0, loc)
	case "day":
		return time.Date(y, m, d, 0, 0, 0, 0, loc), time.Date(y, m, d+1, 0, 0, 0, 0, loc)
	default: // year
		return time.Date(y, 1, 1, 0, 0, 0, 0, loc), time.Date(y+1, 1, 1, 0, 0, 0, 0, loc)
	}
}

// progressRatio returns the elapsed fraction of the period, clamped to [0, 1).
// It never reaches exactly 1 so the widget cannot show 100% before the period
// has actually ended.
func progressRatio(t time.Time, period string) float64 {
	start, end := progressPeriodBounds(t, period)
	total := end.Sub(start)
	if total <= 0 {
		return 0
	}
	ratio := float64(t.Sub(start)) / float64(total)
	if ratio < 0 {
		return 0
	}
	if maxRatio := math.Nextafter(1, 0); ratio > maxRatio {
		return maxRatio
	}
	return ratio
}

// progressCounts returns the 1-based position within the period and its total
// number of units (days for year/month/week, hours for day).
func progressCounts(t time.Time, period string) (current, total int) {
	switch period {
	case "month":
		// Day 0 of the following month is the last day of this month.
		last := time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location())
		return t.Day(), last.Day()
	case "week":
		return (int(t.Weekday())+6)%7 + 1, 7
	case "day":
		return t.Hour() + 1, 24
	default: // year
		last := time.Date(t.Year(), 12, 31, 0, 0, 0, 0, t.Location())
		return t.YearDay(), last.YearDay()
	}
}

// progressCountText formats the count layout in German, consistent with
// formatGermanDate.
func progressCountText(period string, current, total int) string {
	if period == "day" {
		return fmt.Sprintf("Stunde %d von %d", current, total)
	}
	return fmt.Sprintf("Tag %d von %d", current, total)
}

// progressBar renders the ASCII bar: '[' + filled '#' + remaining '-' + ']'.
// filled truncates (never rounds up), so a partially elapsed unit never fills
// an extra cell.
func progressBar(ratio float64, barWidth int) string {
	filled := clampInt(int(ratio*float64(barWidth)), 0, barWidth)
	var b strings.Builder
	b.Grow(barWidth + 2)
	b.WriteByte('[')
	b.WriteString(strings.Repeat("#", filled))
	b.WriteString(strings.Repeat("-", barWidth-filled))
	b.WriteByte(']')
	return b.String()
}

func applyProgressPlaceholders(template, bar, percent, period string, current, total int) string {
	r := strings.NewReplacer(
		"%bar%", bar,
		"%percent%", percent,
		"%current%", fmt.Sprintf("%d", current),
		"%total%", fmt.Sprintf("%d", total),
		"%remaining%", fmt.Sprintf("%d", total-current),
		"%period%", progressPeriodNames[period],
	)
	return r.Replace(template)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
