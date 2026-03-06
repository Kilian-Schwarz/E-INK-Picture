package widgets

import (
	"context"
	"image"
	"strings"
	"time"
)

// ClockWidget renders the current time in a configurable format.
type ClockWidget struct{}

func NewClockWidget() *ClockWidget {
	return &ClockWidget{}
}

func (w *ClockWidget) Render(_ context.Context, props map[string]any, _ image.Rectangle, _ *image.RGBA) error {
	return nil
}

// GetContent returns the formatted current time string.
func (w *ClockWidget) GetContent(props map[string]any) string {
	format := getString(props, "format", "YYYY-MM-DD HH:mm")
	tz := getString(props, "timezone", "")

	loc := time.Now().Location()
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}

	r := strings.NewReplacer(
		"YYYY", "2006",
		"MM", "01",
		"DD", "02",
		"HH", "15",
		"mm", "04",
		"ss", "05",
	)
	goFmt := r.Replace(format)
	return time.Now().In(loc).Format(goFmt)
}
