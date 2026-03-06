package widgets

import (
	"context"
	"fmt"
	"image"
	"strings"
)

// ForecastWidget renders multi-day weather forecast.
type ForecastWidget struct{}

func NewForecastWidget() *ForecastWidget {
	return &ForecastWidget{}
}

func (w *ForecastWidget) Render(_ context.Context, _ map[string]any, _ image.Rectangle, _ *image.RGBA) error {
	return nil
}

// GetContent returns formatted forecast text.
func (w *ForecastWidget) GetContent(props map[string]any, data *WeatherResult) string {
	if data == nil || len(data.Daily) == 0 {
		return "No forecast data"
	}

	days := getInt(props, "days", 3)

	var lines []string
	for i, day := range data.Daily {
		if i >= days {
			break
		}
		lines = append(lines, fmt.Sprintf("%s: %d-%d°C %s",
			day.Weekday, int(day.Min), int(day.Max), day.Desc))
	}
	if len(lines) == 0 {
		return "No forecast data"
	}
	return strings.Join(lines, "\n")
}
