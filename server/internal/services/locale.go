package services

import "time"

// German localization tables for the panel renderer. These are the single
// source of truth for German weekday, month and weather-condition text: the
// clock/date placeholders and forecast layouts in preview.go and the forecast
// weekday, condition descriptions and ApplyStyle summary in weather.go all read
// from here — no layout keeps its own divergent copy.

// germanWeekdaysFull maps a time.Weekday (0 = Sunday) to its full German name.
var germanWeekdaysFull = [7]string{"Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"}

// germanWeekdaysShort maps a time.Weekday (0 = Sunday) to its two-letter German
// abbreviation (Mo/Di/Mi/Do/Fr/Sa/So).
var germanWeekdaysShort = [7]string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"}

// germanMonths maps a 1-based month number to its full German name (index 0 unused).
var germanMonths = [13]string{"", "Januar", "Februar", "März", "April", "Mai", "Juni",
	"Juli", "August", "September", "Oktober", "November", "Dezember"}

// germanWeekdayShortByName maps a full German weekday name to its two-letter
// abbreviation, so callers that only hold the localized full name (the forecast
// compact_row layout) get the proper short form without re-parsing the date.
// Derived from the canonical arrays above to keep a single source of truth.
var germanWeekdayShortByName = func() map[string]string {
	m := make(map[string]string, len(germanWeekdaysFull))
	for i := range germanWeekdaysFull {
		m[germanWeekdaysFull[i]] = germanWeekdaysShort[i]
	}
	return m
}()

// germanWeekday returns the full German weekday name for t.
func germanWeekday(t time.Time) string {
	return germanWeekdaysFull[t.Weekday()]
}

// germanShortWeekdayFromName returns the two-letter German abbreviation for a
// full German weekday name, falling back to the input unchanged if unknown.
func germanShortWeekdayFromName(full string) string {
	if s, ok := germanWeekdayShortByName[full]; ok {
		return s
	}
	return full
}

// germanWMODesc maps a WMO weather code to its German description. This is the
// single source of the localized condition text; day and night share the same
// wording and differ only in icon (weatherDayIcons/weatherNightIcons).
var germanWMODesc = map[int]string{
	0:  "Klarer Himmel",
	1:  "Überwiegend klar",
	2:  "Teilweise bewölkt",
	3:  "Bedeckt",
	45: "Nebel",
	48: "Reifnebel",
	51: "Leichter Nieselregen",
	61: "Leichter Regen",
	63: "Mäßiger Regen",
	65: "Starker Regen",
	80: "Regenschauer",
}

// germanWMOUnknown is the German fallback for an unmapped WMO code.
const germanWMOUnknown = "Unbekannt"
