package widgets

import (
	"context"
	"fmt"
	"image"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"
)

// CalendarWidget fetches and displays iCal events.
type CalendarWidget struct {
	client *http.Client
}

func NewCalendarWidget() *CalendarWidget {
	return &CalendarWidget{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (w *CalendarWidget) Render(_ context.Context, _ map[string]any, _ image.Rectangle, _ *image.RGBA) error {
	return nil
}

// GetContent fetches iCal events and returns formatted text.
func (w *CalendarWidget) GetContent(props map[string]any) string {
	calURL := getString(props, "icalUrl", "")
	maxEvents := getInt(props, "maxEvents", 5)
	daysAhead := getInt(props, "daysAhead", 30)
	title := getString(props, "title", "")
	showTime := getBool(props, "showTime", true)

	if calURL == "" {
		return "No calendar URL"
	}

	if strings.HasPrefix(calURL, "webcal://") {
		calURL = "https://" + calURL[len("webcal://"):]
	}

	resp, err := w.client.Get(calURL)
	if err != nil {
		slog.Error("failed to fetch calendar", "error", err)
		return "No events"
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "No events"
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "No events"
	}

	events := parseICalEvents(string(body), maxEvents, daysAhead)
	if len(events) == 0 {
		return "No events"
	}

	var lines []string
	if title != "" {
		lines = append(lines, title)
	}
	for _, e := range events {
		if showTime {
			lines = append(lines, fmt.Sprintf("%s - %s", e.Start.Format("2006-01-02 15:04"), e.Summary))
		} else {
			lines = append(lines, fmt.Sprintf("%s - %s", e.Start.Format("2006-01-02"), e.Summary))
		}
	}
	return strings.Join(lines, "\n")
}

type calEvent struct {
	Start   time.Time
	Summary string
}

func parseICalEvents(ical string, maxEvents, daysAhead int) []calEvent {
	var events []calEvent
	now := time.Now()
	cutoff := now.Add(time.Duration(daysAhead) * 24 * time.Hour)

	lines := strings.Split(strings.ReplaceAll(ical, "\r\n", "\n"), "\n")
	var unfolded []string
	for _, line := range lines {
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') && len(unfolded) > 0 {
			unfolded[len(unfolded)-1] += line[1:]
		} else {
			unfolded = append(unfolded, line)
		}
	}

	inEvent := false
	var currentStart time.Time
	var currentSummary string

	for _, line := range unfolded {
		trimmed := strings.TrimSpace(line)
		if trimmed == "BEGIN:VEVENT" {
			inEvent = true
			currentStart = time.Time{}
			currentSummary = ""
			continue
		}
		if trimmed == "END:VEVENT" {
			if inEvent && !currentStart.IsZero() && currentStart.After(now) && currentStart.Before(cutoff) {
				events = append(events, calEvent{Start: currentStart, Summary: currentSummary})
			}
			inEvent = false
			continue
		}
		if !inEvent {
			continue
		}

		if strings.HasPrefix(trimmed, "DTSTART") {
			currentStart = parseICalDate(trimmed)
		} else if strings.HasPrefix(trimmed, "SUMMARY:") {
			currentSummary = strings.TrimPrefix(trimmed, "SUMMARY:")
		}
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].Start.Before(events[j].Start)
	})

	if len(events) > maxEvents {
		events = events[:maxEvents]
	}
	return events
}

func parseICalDate(line string) time.Time {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return time.Time{}
	}
	val := strings.TrimSpace(parts[1])

	formats := []string{
		"20060102T150405Z",
		"20060102T150405",
		"20060102",
	}

	for _, f := range formats {
		if t, err := time.ParseInLocation(f, val, time.Now().Location()); err == nil {
			return t
		}
	}
	return time.Time{}
}
