package handlers

import (
	"net/http"
	"strconv"

	"e-ink-picture/server/internal/services"
	"e-ink-picture/server/internal/services/widgets"
)

// WidgetHandler provides widget data API endpoints.
type WidgetHandler struct {
	weather  *services.WeatherService
	calendar *widgets.CalendarWidget
	news     *widgets.NewsWidget
	custom   *widgets.CustomWidget
	system   *widgets.SystemWidget
}

// NewWidgetHandler creates a new WidgetHandler.
func NewWidgetHandler(weather *services.WeatherService) *WidgetHandler {
	return &WidgetHandler{
		weather:  weather,
		calendar: widgets.NewCalendarWidget(),
		news:     widgets.NewNewsWidget(),
		custom:   widgets.NewCustomWidget(),
		system:   widgets.NewSystemWidget(),
	}
}

// Weather returns current weather data as JSON.
func (h *WidgetHandler) Weather(w http.ResponseWriter, r *http.Request) {
	lat := r.URL.Query().Get("lat")
	lon := r.URL.Query().Get("lon")
	if lat == "" {
		lat = "52.52"
	}
	if lon == "" {
		lon = "13.41"
	}

	data, err := h.weather.FetchForLocation(lat, lon)
	if err != nil || data == nil {
		jsonError(w, "Failed to fetch weather", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, data)
}

// Forecast returns multi-day forecast data as JSON.
func (h *WidgetHandler) Forecast(w http.ResponseWriter, r *http.Request) {
	lat := r.URL.Query().Get("lat")
	lon := r.URL.Query().Get("lon")
	if lat == "" {
		lat = "52.52"
	}
	if lon == "" {
		lon = "13.41"
	}
	days := 3
	if d := r.URL.Query().Get("days"); d != "" {
		if v, err := strconv.Atoi(d); err == nil && v > 0 {
			days = v
		}
	}

	data, err := h.weather.FetchForLocation(lat, lon)
	if err != nil || data == nil {
		jsonError(w, "Failed to fetch forecast", http.StatusInternalServerError)
		return
	}

	// Limit daily to requested days
	daily := data.Daily
	if len(daily) > days {
		daily = daily[:days]
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"daily": daily,
	})
}

// Calendar returns parsed iCal events as JSON.
func (h *WidgetHandler) Calendar(w http.ResponseWriter, r *http.Request) {
	calURL := r.URL.Query().Get("url")
	if calURL == "" {
		jsonError(w, "Missing url parameter", http.StatusBadRequest)
		return
	}

	daysAhead := 7
	if d := r.URL.Query().Get("days"); d != "" {
		if v, err := strconv.Atoi(d); err == nil && v > 0 {
			daysAhead = v
		}
	}

	props := map[string]any{
		"icalUrl":   calURL,
		"maxEvents": float64(10),
		"daysAhead": float64(daysAhead),
		"showTime":  true,
	}

	content := h.calendar.GetContent(props)
	jsonResponse(w, http.StatusOK, map[string]string{
		"content": content,
	})
}

// News returns parsed RSS feed items as JSON.
func (h *WidgetHandler) News(w http.ResponseWriter, r *http.Request) {
	feedURL := r.URL.Query().Get("url")
	if feedURL == "" {
		jsonError(w, "Missing url parameter", http.StatusBadRequest)
		return
	}

	maxItems := 5
	if m := r.URL.Query().Get("max"); m != "" {
		if v, err := strconv.Atoi(m); err == nil && v > 0 {
			maxItems = v
		}
	}

	props := map[string]any{
		"feedUrl":  feedURL,
		"maxItems": float64(maxItems),
	}

	content := h.news.GetContent(props)
	jsonResponse(w, http.StatusOK, map[string]string{
		"content": content,
	})
}

// System returns system info as JSON.
func (h *WidgetHandler) System(w http.ResponseWriter, r *http.Request) {
	props := map[string]any{
		"showLabels": true,
	}

	content := h.system.GetContent(props)
	jsonResponse(w, http.StatusOK, map[string]string{
		"content": content,
	})
}

// Layouts returns available layouts for a widget type.
func (h *WidgetHandler) Layouts(w http.ResponseWriter, r *http.Request) {
	widgetType := r.PathValue("type")
	if widgetType == "" {
		jsonError(w, "Missing widget type", http.StatusBadRequest)
		return
	}
	layouts := widgets.GetLayouts(widgetType)
	jsonResponse(w, http.StatusOK, map[string]any{
		"layouts":      layouts,
		"placeholders": widgets.Placeholders(widgetType),
	})
}

// Custom fetches data from a custom API and returns it.
func (h *WidgetHandler) Custom(w http.ResponseWriter, r *http.Request) {
	apiURL := r.URL.Query().Get("url")
	if apiURL == "" {
		jsonError(w, "Missing url parameter", http.StatusBadRequest)
		return
	}

	jsonPath := r.URL.Query().Get("jsonpath")
	props := map[string]any{
		"url":      apiURL,
		"jsonPath": jsonPath,
	}

	content := h.custom.GetContent(props)
	jsonResponse(w, http.StatusOK, map[string]string{
		"content": content,
	})
}
