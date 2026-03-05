package handlers

import (
	"encoding/json"
	"net/http"

	"e-ink-picture/server/internal/services"
)

type WeatherHandler struct {
	svc *services.WeatherService
}

func NewWeatherHandler(svc *services.WeatherService) *WeatherHandler {
	return &WeatherHandler{svc: svc}
}

func (h *WeatherHandler) ListStyles(w http.ResponseWriter, r *http.Request) {
	// TODO: list weather style files
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode([]any{})
}

func (h *WeatherHandler) LocationSearch(w http.ResponseWriter, r *http.Request) {
	// TODO: proxy location search to nominatim API
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode([]any{})
}
