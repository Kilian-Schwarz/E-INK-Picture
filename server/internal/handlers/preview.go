package handlers

import (
	"fmt"
	"net/http"

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

	if name != "" {
		design, getErr := h.designSvc.Get(name)
		if getErr != nil {
			jsonError(w, "Design not found", http.StatusNotFound)
			return
		}
		pngData, err = h.svc.Render(design)
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
