package services

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CronSchedule is a parsed 5-field cron expression
// (minute hour day-of-month month day-of-week). It is used to schedule
// display refreshes at fixed wall-clock times (specs: refresh_cron), as an
// alternative to the relative RefreshInterval.
//
// Supported per-field syntax: "*", a single number, ranges "a-b", steps
// "*/n" and "a-b/n" and "a/n", and comma-separated lists of any of these.
// Day-of-week accepts 0-6 (0=Sunday) and tolerates 7 as Sunday. When both
// day-of-month and day-of-week are restricted (not "*"), a day matches if
// EITHER field matches — the standard Vixie-cron OR semantics.
type CronSchedule struct {
	minute [60]bool
	hour   [24]bool
	dom    [32]bool // index 1-31
	month  [13]bool // index 1-12
	dow    [7]bool  // index 0-6 (Sunday=0)

	// domRestricted/dowRestricted record whether the day-of-month /
	// day-of-week field was something other than "*", which drives the OR
	// semantics in dayMatches.
	domRestricted bool
	dowRestricted bool
}

// ParseCron parses a standard 5-field cron expression. Fields are separated by
// runs of whitespace. It returns an error for any malformed field so callers
// can reject bad user input (the handler) or fall back to interval scheduling
// (the refresh decision).
func ParseCron(spec string) (*CronSchedule, error) {
	fields := strings.Fields(spec)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron: expected 5 fields, got %d", len(fields))
	}

	s := &CronSchedule{}

	minutes, _, err := parseCronField(fields[0], 0, 59, nil)
	if err != nil {
		return nil, fmt.Errorf("cron minute: %w", err)
	}
	for _, v := range minutes {
		s.minute[v] = true
	}

	hours, _, err := parseCronField(fields[1], 0, 23, nil)
	if err != nil {
		return nil, fmt.Errorf("cron hour: %w", err)
	}
	for _, v := range hours {
		s.hour[v] = true
	}

	doms, domRestricted, err := parseCronField(fields[2], 1, 31, nil)
	if err != nil {
		return nil, fmt.Errorf("cron day-of-month: %w", err)
	}
	for _, v := range doms {
		s.dom[v] = true
	}
	s.domRestricted = domRestricted

	months, _, err := parseCronField(fields[3], 1, 12, nil)
	if err != nil {
		return nil, fmt.Errorf("cron month: %w", err)
	}
	for _, v := range months {
		s.month[v] = true
	}

	// Normalize day-of-week 7 -> 0 (both mean Sunday) before recording set bits.
	dows, dowRestricted, err := parseCronField(fields[4], 0, 7, func(v int) int {
		if v == 7 {
			return 0
		}
		return v
	})
	if err != nil {
		return nil, fmt.Errorf("cron day-of-week: %w", err)
	}
	for _, v := range dows {
		s.dow[v] = true
	}
	s.dowRestricted = dowRestricted

	return s, nil
}

// parseCronField parses one comma-separated cron field into the set of concrete
// values it selects. min/max bound the legal range; normalize (may be nil)
// remaps a raw value after bounds checking (used to fold day-of-week 7 into 0).
// The returned bool reports whether the field was restricted (anything other
// than a bare "*").
func parseCronField(field string, min, max int, normalize func(int) int) ([]int, bool, error) {
	if field == "" {
		return nil, false, fmt.Errorf("empty field")
	}
	restricted := field != "*"
	set := make(map[int]struct{})

	for _, part := range strings.Split(field, ",") {
		rng := part
		step := 1

		if slash := strings.IndexByte(part, '/'); slash >= 0 {
			rng = part[:slash]
			stepStr := part[slash+1:]
			st, err := strconv.Atoi(stepStr)
			if err != nil || st <= 0 {
				return nil, false, fmt.Errorf("invalid step %q", stepStr)
			}
			step = st
		}

		lo, hi := min, max
		if rng != "*" {
			if dash := strings.IndexByte(rng, '-'); dash >= 0 {
				a, err1 := strconv.Atoi(rng[:dash])
				b, err2 := strconv.Atoi(rng[dash+1:])
				if err1 != nil || err2 != nil {
					return nil, false, fmt.Errorf("invalid range %q", rng)
				}
				lo, hi = a, b
			} else {
				v, err := strconv.Atoi(rng)
				if err != nil {
					return nil, false, fmt.Errorf("invalid value %q", rng)
				}
				lo, hi = v, v
			}
		}

		if lo < min || hi > max || lo > hi {
			return nil, false, fmt.Errorf("value %d-%d out of range %d-%d", lo, hi, min, max)
		}

		for v := lo; v <= hi; v += step {
			nv := v
			if normalize != nil {
				nv = normalize(v)
			}
			set[nv] = struct{}{}
		}
	}

	out := make([]int, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	return out, restricted, nil
}

// dayMatches reports whether t's calendar day is selected, applying the
// Vixie-cron OR semantics between day-of-month and day-of-week.
func (s *CronSchedule) dayMatches(t time.Time) bool {
	domOk := s.dom[t.Day()]
	dowOk := s.dow[int(t.Weekday())]
	switch {
	case s.domRestricted && s.dowRestricted:
		return domOk || dowOk
	case s.domRestricted:
		return domOk
	case s.dowRestricted:
		return dowOk
	default:
		return true
	}
}

// Next returns the earliest scheduled time strictly after t, evaluated in t's
// location (so callers control the "local time" the schedule refers to). It
// advances field by field — whole months/days/hours are skipped rather than
// stepping minute by minute — and gives up after 5 years, returning the zero
// time for an unsatisfiable expression (e.g. "0 0 31 2 *"). Wall-clock times
// skipped by a spring-forward DST transition simply do not fire that day,
// matching standard cron behaviour.
func (s *CronSchedule) Next(t time.Time) time.Time {
	loc := t.Location()
	// Start at the next whole minute strictly after t.
	t = t.Truncate(time.Minute).Add(time.Minute)
	yearLimit := t.Year() + 5

	for {
		if t.Year() > yearLimit {
			return time.Time{}
		}
		if !s.month[int(t.Month())] {
			// Jump to the first day of the next month at 00:00.
			t = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc).AddDate(0, 1, 0)
			continue
		}
		if !s.dayMatches(t) {
			// Jump to 00:00 of the next day.
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
			continue
		}
		if !s.hour[t.Hour()] {
			// Jump to the next hour at :00.
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, loc).Add(time.Hour)
			continue
		}
		if !s.minute[t.Minute()] {
			t = t.Add(time.Minute)
			continue
		}
		return t
	}
}
