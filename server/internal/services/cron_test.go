package services

import (
	"testing"
	"time"
)

func TestParseCronErrors(t *testing.T) {
	cases := []string{
		"",            // empty
		"* * * *",     // 4 fields
		"* * * * * *", // 6 fields
		"60 * * * *",  // minute out of range
		"* 24 * * *",  // hour out of range
		"* * 0 * *",   // day-of-month below 1
		"* * 32 * *",  // day-of-month above 31
		"* * * 13 *",  // month above 12
		"* * * * 8",   // day-of-week above 7
		"*/0 * * * *", // zero step
		"5-1 * * * *", // inverted range
		"a * * * *",   // non-numeric
		"1-x * * * *", // bad range
		"1/b * * * *", // bad step
	}
	for _, spec := range cases {
		if _, err := ParseCron(spec); err == nil {
			t.Errorf("ParseCron(%q): expected error, got nil", spec)
		}
	}
}

func TestParseCronValid(t *testing.T) {
	cases := []string{
		"* * * * *",
		"1 0 * * *",
		"*/5 * * * *",
		"0 0,12 * * *",
		"0 9-17 * * 1-5",
		"15 10 1,15 * *",
		"0 0 * * 7", // Sunday as 7
		"0 0-23/2 * * *",
	}
	for _, spec := range cases {
		if _, err := ParseCron(spec); err != nil {
			t.Errorf("ParseCron(%q): unexpected error: %v", spec, err)
		}
	}
}

// mustParse is a test helper.
func mustParse(t *testing.T, spec string) *CronSchedule {
	t.Helper()
	s, err := ParseCron(spec)
	if err != nil {
		t.Fatalf("ParseCron(%q): %v", spec, err)
	}
	return s
}

func TestCronNextDaily(t *testing.T) {
	loc := time.FixedZone("CET", 3600)
	s := mustParse(t, "1 0 * * *") // 00:01 every day

	// From mid-day, the next tick is 00:01 tomorrow.
	from := time.Date(2026, 7, 25, 12, 30, 0, 0, loc)
	got := s.Next(from)
	want := time.Date(2026, 7, 26, 0, 1, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("Next(%v) = %v, want %v", from, got, want)
	}

	// From just before 00:01, the next tick is 00:01 same day.
	from = time.Date(2026, 7, 25, 0, 0, 30, 0, loc)
	got = s.Next(from)
	want = time.Date(2026, 7, 25, 0, 1, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("Next(%v) = %v, want %v", from, got, want)
	}

	// Next is strictly after t: exactly at 00:01 rolls to the following day.
	from = time.Date(2026, 7, 25, 0, 1, 0, 0, loc)
	got = s.Next(from)
	want = time.Date(2026, 7, 26, 0, 1, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("Next(%v) = %v, want %v", from, got, want)
	}
}

func TestCronNextEveryFiveMinutes(t *testing.T) {
	loc := time.UTC
	s := mustParse(t, "*/5 * * * *")
	from := time.Date(2026, 7, 25, 10, 2, 0, 0, loc)
	got := s.Next(from)
	want := time.Date(2026, 7, 25, 10, 5, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("Next(%v) = %v, want %v", from, got, want)
	}
}

func TestCronNextHourly(t *testing.T) {
	loc := time.UTC
	s := mustParse(t, "0 * * * *")
	from := time.Date(2026, 7, 25, 10, 30, 0, 0, loc)
	got := s.Next(from)
	want := time.Date(2026, 7, 25, 11, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("Next(%v) = %v, want %v", from, got, want)
	}
}

func TestCronNextMonthRollover(t *testing.T) {
	loc := time.UTC
	s := mustParse(t, "0 0 1 * *") // midnight on the 1st
	from := time.Date(2026, 7, 25, 10, 0, 0, 0, loc)
	got := s.Next(from)
	want := time.Date(2026, 8, 1, 0, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("Next(%v) = %v, want %v", from, got, want)
	}
}

func TestCronNextWeekday(t *testing.T) {
	loc := time.UTC
	s := mustParse(t, "0 9 * * 1") // 09:00 on Mondays
	// 2026-07-25 is a Saturday; next Monday is 2026-07-27.
	from := time.Date(2026, 7, 25, 12, 0, 0, 0, loc)
	got := s.Next(from)
	want := time.Date(2026, 7, 27, 9, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("Next(%v) = %v, want %v (weekday %v)", from, got, want, got.Weekday())
	}
}

func TestCronDayOfMonthOrDayOfWeek(t *testing.T) {
	loc := time.UTC
	// Both dom and dow restricted -> OR semantics: fires on the 1st OR on Mondays.
	s := mustParse(t, "0 0 1 * 1")
	// 2026-07-25 (Sat). Next Monday 2026-07-27 comes before the 1st (2026-08-01).
	from := time.Date(2026, 7, 25, 12, 0, 0, 0, loc)
	got := s.Next(from)
	want := time.Date(2026, 7, 27, 0, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("OR semantics: Next(%v) = %v, want %v", from, got, want)
	}
}

func TestCronUnsatisfiable(t *testing.T) {
	loc := time.UTC
	s := mustParse(t, "0 0 31 2 *") // Feb 31 never exists
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	got := s.Next(from)
	if !got.IsZero() {
		t.Errorf("unsatisfiable schedule: expected zero time, got %v", got)
	}
}

func TestCronNextRespectsLocation(t *testing.T) {
	// 00:01 "local" must be computed in the caller's location, not UTC.
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	s := mustParse(t, "1 0 * * *")
	from := time.Date(2026, 7, 25, 23, 0, 0, 0, berlin) // 23:00 local (summer, UTC+2)
	got := s.Next(from)
	want := time.Date(2026, 7, 26, 0, 1, 0, 0, berlin)
	if !got.Equal(want) {
		t.Errorf("Next(%v) = %v, want %v", from, got, want)
	}
	if got.Location().String() != berlin.String() {
		t.Errorf("Next lost location: got %v", got.Location())
	}
}
