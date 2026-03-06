package handlers

import (
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
	styles, err := h.svc.ListStyles()
	if err != nil {
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, styles)
}

func (h *WeatherHandler) LocationSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		jsonResponse(w, http.StatusOK, []any{})
		return
	}
	results, err := h.svc.SearchLocation(q)
	if err != nil {
		jsonResponse(w, http.StatusOK, []any{})
		return
	}
	jsonResponse(w, http.StatusOK, results)
}
