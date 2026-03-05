package handlers

import (
	"encoding/json"
	"net/http"

	"e-ink-picture/server/internal/services"
)

type MediaHandler struct {
	svc *services.ImageService
}

func NewMediaHandler(svc *services.ImageService) *MediaHandler {
	return &MediaHandler{svc: svc}
}

func (h *MediaHandler) ListImages(w http.ResponseWriter, r *http.Request) {
	// TODO: call h.svc.ListImages()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode([]any{})
}

func (h *MediaHandler) GetImage(w http.ResponseWriter, r *http.Request) {
	// TODO: get filename from URL path, serve file
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (h *MediaHandler) DeleteImage(w http.ResponseWriter, r *http.Request) {
	// TODO: parse JSON body, call h.svc.DeleteImage()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Image deleted"})
}

func (h *MediaHandler) Upload(w http.ResponseWriter, r *http.Request) {
	// TODO: parse multipart form, save image/font
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "File uploaded"})
}

func (h *MediaHandler) ListFonts(w http.ResponseWriter, r *http.Request) {
	// TODO: call h.svc.ListFonts()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode([]any{})
}

func (h *MediaHandler) GetFont(w http.ResponseWriter, r *http.Request) {
	// TODO: get filename from URL path, serve file
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
