package services

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// F4 — widget_holidays: the next German public holiday (with countdown) or a
// list of the next N, for a selectable Bundesland.
//
// Computed purely locally: Gauss/Meeus Easter algorithm plus a rule table. No
// network, no provider, no cache. The domain is pure arithmetic over
// (year, state), so a fetch would only add latency, an outage mode and
// non-determinism to something that needs none of them.
//
// Output flows through WidgetTextContent like every other widget and is drawn
// with the embedded goregular fallback font. German umlauts and ß are fine
// (precedent: germanMonths / "März" in locale.go), but every rune stays below
// U+0100 — an en dash, em dash or ellipsis renders as blank space or tofu on
// the panel while looking correct on the browser canvas.

// holidayRulesAsOf is the legal cutoff date of the rule table below.
//
// The table cannot update itself, so this constant makes its age visible. It
// reflects the Feiertagsgesetze of the 16 Länder as published for 2026
// (last substantive changes: Reformationstag as a permanent holiday in
// HB/HH/NI/SH from 2018, Internationaler Frauentag in BE from 2019 and in MV
// from 2023). When a Land changes its law, update the table AND this date.
// Pinned by TestHolidayRuleTableStamp.
const holidayRulesAsOf = "2026-01-01"

const (
	holidayDefaultState    = "DE"
	holidayDefaultLayout   = "next_countdown"
	holidayDefaultCount    = 3
	holidayMinCount        = 1
	holidayMaxCount        = 10
	holidayDefaultTemplate = "%name% (%date%)"
	// holidayYearSpan is the number of consecutive years generated as
	// candidates. Two years are not enough: on 31 December with count = 10 the
	// remainder of the current year is empty and a single following year yields
	// only 9..12 entries depending on the state. Three years always yield >= 18.
	holidayYearSpan = 3
)

// holidayStateNames maps a state code to its written-out name (%state%).
// "DE" is the sentinel for "nationwide holidays only" and is the default: it is
// a strict subset of every Land, so a misconfiguration can only ever omit a
// holiday, never claim that a working day is a holiday.
var holidayStateNames = map[string]string{
	"DE": "Deutschland",
	"BW": "Baden-Württemberg",
	"BY": "Bayern",
	"BE": "Berlin",
	"BB": "Brandenburg",
	"HB": "Bremen",
	"HH": "Hamburg",
	"HE": "Hessen",
	"MV": "Mecklenburg-Vorpommern",
	"NI": "Niedersachsen",
	"NW": "Nordrhein-Westfalen",
	"RP": "Rheinland-Pfalz",
	"SL": "Saarland",
	"SN": "Sachsen",
	"ST": "Sachsen-Anhalt",
	"SH": "Schleswig-Holstein",
	"TH": "Thüringen",
}

// holidayStates lists every accepted state code in display order (the
// nationwide sentinel first). Used by tests to enumerate the full matrix.
var holidayStates = []string{
	"DE",
	"BW", "BY", "BE", "BB", "HB", "HH", "HE", "MV",
	"NI", "NW", "RP", "SL", "SN", "ST", "SH", "TH",
}

// holiday is a single dated public holiday in the requested location.
type holiday struct {
	date time.Time
	name string
}

// holidayRule is one entry of the rule table. states == nil means "nationwide"
// — those nine holidays apply in all 16 Länder and to the DE sentinel. A
// non-nil states list applies to exactly the named Länder and never to DE.
type holidayRule struct {
	name   string
	states []string
}

// applies reports whether the rule holds in the given normalized state code.
func (r holidayRule) applies(state string) bool {
	if r.states == nil {
		return true
	}
	for _, s := range r.states {
		if s == state {
			return true
		}
	}
	return false
}

// holidayFixedRules are the holidays on a fixed calendar date.
//
// Rechtsstand: see holidayRulesAsOf.
//
// Deliberately NOT included, because the widget has no municipal resolution and
// must never grow one:
//   - Augsburger Friedensfest (08.08.) — city of Augsburg only, not Bavaria.
//   - Mariä Himmelfahrt in Bayern — only in predominantly Catholic
//     municipalities (~1700 of 2056). In SL it applies state-wide, so SL is in.
//   - Fronleichnam in Sachsen/Thüringen — individual municipalities only.
var holidayFixedRules = []struct {
	month time.Month
	day   int
	holidayRule
}{
	{time.January, 1, holidayRule{name: "Neujahr"}},
	{time.January, 6, holidayRule{name: "Heilige Drei Könige", states: []string{"BW", "BY", "ST"}}},
	{time.March, 8, holidayRule{name: "Internationaler Frauentag", states: []string{"BE", "MV"}}},
	{time.May, 1, holidayRule{name: "Tag der Arbeit"}},
	{time.August, 15, holidayRule{name: "Mariä Himmelfahrt", states: []string{"SL"}}},
	{time.September, 20, holidayRule{name: "Weltkindertag", states: []string{"TH"}}},
	{time.October, 3, holidayRule{name: "Tag der Deutschen Einheit"}},
	{time.October, 31, holidayRule{name: "Reformationstag", states: []string{"BB", "HB", "HH", "MV", "NI", "SH", "SN", "ST", "TH"}}},
	{time.November, 1, holidayRule{name: "Allerheiligen", states: []string{"BW", "BY", "NW", "RP", "SL"}}},
	{time.December, 25, holidayRule{name: "1. Weihnachtstag"}},
	{time.December, 26, holidayRule{name: "2. Weihnachtstag"}},
}

// holidayEasterRules are the holidays at a fixed day offset from Easter Sunday.
//
// Ostersonntag (offset 0) and Pfingstsonntag (offset +49) are deliberately
// ABSENT from this table: their legal treatment differs between the Länder and
// is contested in the literature (BB/HE), and both always fall on a Sunday, so
// for a "next day off" countdown they are pure noise. This omission is a
// decision, not an oversight.
var holidayEasterRules = []struct {
	offset int
	holidayRule
}{
	{-2, holidayRule{name: "Karfreitag"}},
	{1, holidayRule{name: "Ostermontag"}},
	{39, holidayRule{name: "Christi Himmelfahrt"}},
	{50, holidayRule{name: "Pfingstmontag"}},
	{60, holidayRule{name: "Fronleichnam", states: []string{"BW", "BY", "HE", "NW", "RP", "SL"}}},
}

// bussUndBettagStates limits the movable Buß- und Bettag to Saxony.
var bussUndBettagStates = holidayRule{name: "Buß- und Bettag", states: []string{"SN"}}

// fillHolidaysContent renders the widget_holidays text content.
//
// Timezone semantics follow the widget_clock/widget_progress precedent: an
// explicit IANA "timezone" property wins, an invalid name falls back silently,
// an empty value uses server-local time. That matters here because "today"
// decides which holiday is next: a UTC container serving a panel in
// Europe/Berlin would flip the countdown up to an hour late.
func (s *PreviewService) fillHolidaysContent(props map[string]any) string {
	state := normalizeHolidayState(GetPropString(props, "state", holidayDefaultState))
	layout := normalizeHolidayLayout(GetPropString(props, "layout", holidayDefaultLayout))
	count := clampInt(GetPropInt(props, "count", holidayDefaultCount), holidayMinCount, holidayMaxCount)

	// Read the clock ONCE. Location, "today" and every candidate year are
	// derived from this single instant; a second read could tear across
	// midnight and produce a date from one day with a countdown from another.
	serverNow := s.nowOrDefault()

	loc := serverNow.Location()
	if tz := GetPropString(props, "timezone", ""); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	now := serverNow.In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	upcoming := upcomingHolidays(today, state, loc)
	if len(upcoming) == 0 {
		// Unreachable: the candidate window always contains at least next
		// year's Neujahr. Guarded so the function can never index out of range
		// nor return an empty string.
		return holidayStateNames[state]
	}

	switch layout {
	case "next":
		return holidayNextText(upcoming[0], today, false)
	case "list":
		return holidayListText(upcoming, count)
	case "custom":
		template := GetPropString(props, "customTemplate", "")
		if template == "" {
			template = holidayDefaultTemplate
		}
		return applyHolidayPlaceholders(template, upcoming[0], today, state)
	default: // next_countdown
		return holidayNextText(upcoming[0], today, true)
	}
}

// normalizeHolidayState maps an unknown or empty code to the DE sentinel.
func normalizeHolidayState(state string) string {
	if _, ok := holidayStateNames[state]; ok {
		return state
	}
	return holidayDefaultState
}

// normalizeHolidayLayout maps an unknown layout to the default.
func normalizeHolidayLayout(layout string) string {
	switch layout {
	case "next", "next_countdown", "list", "custom":
		return layout
	default:
		return holidayDefaultLayout
	}
}

// easterSunday returns Easter Sunday of year y in the Gregorian calendar using
// the anonymous Gregorian algorithm (Meeus/Jones/Butcher). Pure integer math.
func easterSunday(y int, loc *time.Location) time.Time {
	a := y % 19
	b := y / 100
	c := y % 100
	d := b / 4
	e := b % 4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i := c / 4
	k := c % 4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	n := h + l - 7*m + 114
	month := n / 31
	day := n%31 + 1
	return time.Date(y, time.Month(month), day, 0, 0, 0, 0, loc)
}

// bussUndBettag returns the Wednesday strictly before 23 November of year y.
// "Before" is strict: if 23 November is itself a Wednesday, the holiday is the
// 16th. The result always falls in [16.11., 22.11.].
func bussUndBettag(y int, loc *time.Location) time.Time {
	nov23 := time.Date(y, time.November, 23, 0, 0, 0, 0, loc)
	back := (int(nov23.Weekday()) - int(time.Wednesday) + 7) % 7
	if back == 0 {
		back = 7
	}
	return nov23.AddDate(0, 0, -back)
}

// holidaysForYear returns every public holiday of year y valid in state,
// sorted ascending by date and, on a tie, by name (so the order is total).
// An unknown state code behaves exactly like the DE sentinel.
func holidaysForYear(y int, state string, loc *time.Location) []holiday {
	state = normalizeHolidayState(state)
	out := make([]holiday, 0, 12)

	for _, r := range holidayFixedRules {
		if r.applies(state) {
			out = append(out, holiday{date: time.Date(y, r.month, r.day, 0, 0, 0, 0, loc), name: r.name})
		}
	}

	// Easter offsets are applied with AddDate, never by adding a
	// time.Duration: a duration would shift the wall-clock date across the
	// March DST switch, which lies between Easter and Christi Himmelfahrt.
	easter := easterSunday(y, loc)
	for _, r := range holidayEasterRules {
		if r.applies(state) {
			out = append(out, holiday{date: easter.AddDate(0, 0, r.offset), name: r.name})
		}
	}

	if bussUndBettagStates.applies(state) {
		out = append(out, holiday{date: bussUndBettag(y, loc), name: bussUndBettagStates.name})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].date.Equal(out[j].date) {
			return out[i].name < out[j].name
		}
		return out[i].date.Before(out[j].date)
	})
	return out
}

// upcomingHolidays returns the holidays of the three-year candidate window that
// are not before today, sorted ascending. A holiday that IS today stays in and
// counts as 0 days.
func upcomingHolidays(today time.Time, state string, loc *time.Location) []holiday {
	var out []holiday
	for i := 0; i < holidayYearSpan; i++ {
		for _, h := range holidaysForYear(today.Year()+i, state, loc) {
			if daysBetween(today, h.date) >= 0 {
				out = append(out, h)
			}
		}
	}
	return out
}

// daysBetween returns the number of CALENDAR days from a to b (negative if b is
// before a). Both instants are reduced to their civil (y, m, d) triple and
// rebuilt as UTC midnights; UTC has no DST jumps by definition, so the quotient
// is exact. Computing b.Sub(a)/(24*time.Hour) instead would be off by one day
// on the two DST switch days of each year.
func daysBetween(a, b time.Time) int {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	au := time.Date(ay, am, ad, 0, 0, 0, 0, time.UTC)
	bu := time.Date(by, bm, bd, 0, 0, 0, 0, time.UTC)
	return int(bu.Sub(au) / (24 * time.Hour))
}

// formatHolidayDate renders "Sa, 03.10.2026".
func formatHolidayDate(d time.Time) string {
	return fmt.Sprintf("%s, %02d.%02d.%04d", germanWeekdaysShort[d.Weekday()], d.Day(), int(d.Month()), d.Year())
}

// holidayCountdownText renders the countdown wording. "in 1 Tag" never occurs:
// the singular case is covered by "morgen".
func holidayCountdownText(days int) string {
	switch days {
	case 0:
		return "heute"
	case 1:
		return "morgen"
	default:
		return "in " + strconv.Itoa(days) + " Tagen"
	}
}

// holidayNextText renders the "next" and "next_countdown" layouts: the name on
// the first line, the date (optionally with the countdown in parentheses) on
// the second.
func holidayNextText(h holiday, today time.Time, withCountdown bool) string {
	line := formatHolidayDate(h.date)
	if withCountdown {
		line += " (" + holidayCountdownText(daysBetween(today, h.date)) + ")"
	}
	return h.name + "\n" + line
}

// holidayListText renders up to count lines of "<date>  <name>" (two spaces),
// joined by "\n" with no trailing newline.
func holidayListText(upcoming []holiday, count int) string {
	if count > len(upcoming) {
		count = len(upcoming)
	}
	lines := make([]string, 0, count)
	for _, h := range upcoming[:count] {
		lines = append(lines, formatHolidayDate(h.date)+"  "+h.name)
	}
	return strings.Join(lines, "\n")
}

// applyHolidayPlaceholders substitutes all five registered placeholders.
// %days% is the raw number so a user can write "noch %days% Tage".
func applyHolidayPlaceholders(template string, h holiday, today time.Time, state string) string {
	r := strings.NewReplacer(
		"%name%", h.name,
		"%date%", fmt.Sprintf("%02d.%02d.%04d", h.date.Day(), int(h.date.Month()), h.date.Year()),
		"%weekday%", germanWeekdaysShort[h.date.Weekday()],
		"%days%", strconv.Itoa(daysBetween(today, h.date)),
		"%state%", holidayStateNames[state],
	)
	return r.Replace(template)
}
