package widgets

import (
	"context"
	"fmt"
	"image"
)

// WeatherFetcher abstracts weather data fetching.
type WeatherFetcher interface {
	FetchForLocation(lat, lon string) (WeatherResult, error)
}

// WeatherResult holds weather data needed by the widget.
type WeatherResult struct {
	CurrentTemp float64
	CurrentDesc string
	CurrentIcon string
	Daily       []ForecastDay
}

// ForecastDay holds one day of forecast.
type ForecastDay struct {
	Weekday string
	Min     float64
	Max     float64
	Desc    string
}

// WeatherWidget renders current weather information.
type WeatherWidget struct{}

func NewWeatherWidget() *WeatherWidget {
	return &WeatherWidget{}
}

func (w *WeatherWidget) Render(_ context.Context, _ map[string]any, _ image.Rectangle, _ *image.RGBA) error {
	return nil
}

// GetContent returns formatted weather text based on style.
func (w *WeatherWidget) GetContent(props map[string]any, data *WeatherResult) string {
	if data == nil {
		return "No data"
	}

	style := getString(props, "style", "compact")

	switch style {
	case "detailed":
		return fmt.Sprintf("%.0f°C %s\nHumidity: --%%\nWind: -- km/h", data.CurrentTemp, data.CurrentDesc)
	case "minimal":
		return fmt.Sprintf("%.0f°C", data.CurrentTemp)
	case "icon_only":
		return data.CurrentIcon
	default: // compact
		return fmt.Sprintf("%.0f°C %s", data.CurrentTemp, data.CurrentDesc)
	}
}
