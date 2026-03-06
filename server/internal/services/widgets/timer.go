package widgets

import (
	"context"
	"fmt"
	"image"
	"strconv"
	"strings"
	"time"
)

// TimerWidget renders a countdown to a target date.
type TimerWidget struct{}

func NewTimerWidget() *TimerWidget {
	return &TimerWidget{}
}

func (w *TimerWidget) Render(_ context.Context, _ map[string]any, _ image.Rectangle, _ *image.RGBA) error {
	return nil
}

// GetContent returns the countdown string.
func (w *TimerWidget) GetContent(props map[string]any) string {
	target := getString(props, "targetDate", "2025-01-01 00:00:00")
	format := getString(props, "format", "D days, HH:MM:SS")
	finishedText := getString(props, "finishedText", "Time's up!")
	label := getString(props, "label", "")

	targetDT, err := time.ParseInLocation("2006-01-02 15:04:05", target, time.Now().Location())
	if err != nil {
		return "Invalid timer target"
	}

	diff := targetDT.Sub(time.Now())
	if diff < 0 {
		return finishedText
	}

	totalSecs := int(diff.Seconds())
	days := totalSecs / 86400
	remainder := totalSecs % 86400
	hours := remainder / 3600
	minutes := (remainder % 3600) / 60
	seconds := remainder % 60

	display := strings.Replace(format, "D", strconv.Itoa(days), 1)
	display = strings.Replace(display, "HH", fmt.Sprintf("%02d", hours), 1)
	display = strings.Replace(display, "MM", fmt.Sprintf("%02d", minutes), 1)
	display = strings.Replace(display, "SS", fmt.Sprintf("%02d", seconds), 1)

	if label != "" {
		return label + "\n" + display
	}
	return display
}
