package handlers

import (
	"net/http"

	"e-ink-picture/server/internal/services"
)

type PreviewHandler struct {
	svc *services.PreviewService
}

func NewPreviewHandler(svc *services.PreviewService) *PreviewHandler {
	return &PreviewHandler{svc: svc}
}

func (h *PreviewHandler) Preview(w http.ResponseWriter, r *http.Request) {
	// TODO: get optional name param, render preview, return PNG
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
