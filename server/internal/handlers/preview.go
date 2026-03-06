package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"e-ink-picture/server/internal/models"
	"e-ink-picture/server/internal/services"
)

type PreviewHandler struct {
	svc       *services.PreviewService
	designSvc *services.DesignService
}

func NewPreviewHandler(svc *services.PreviewService, designSvc *services.DesignService) *PreviewHandler {
	return &PreviewHandler{svc: svc, designSvc: designSvc}
}

func (h *PreviewHandler) Preview(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")

	var pngData []byte
	var err error

	raw := r.URL.Query().Get("raw") == "true"

	if name != "" {
		design, getErr := h.designSvc.Get(name)
		if getErr != nil {
			jsonError(w, "Design not found", http.StatusNotFound)
			return
		}
		pngData, err = h.svc.Render(design, raw)
	} else {
		pngData, err = h.svc.RenderActive()
	}

	if err != nil {
		jsonError(w, "No design", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(pngData)))
	w.Write(pngData)
}

// PreviewLive renders a design from the request body without saving it.
func (h *PreviewHandler) PreviewLive(w http.ResponseWriter, r *http.Request) {
	var design models.DesignV2
	if err := json.NewDecoder(r.Body).Decode(&design); err != nil {
		jsonError(w, "Invalid design data", http.StatusBadRequest)
		return
	}

	raw := r.URL.Query().Get("raw") == "true"
	pngData, err := h.svc.Render(&design, raw)
	if err != nil {
		jsonError(w, "Render failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(pngData)))
	w.Write(pngData)
}
