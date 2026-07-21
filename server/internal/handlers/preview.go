package handlers

import (
	"context"
	"encoding/json"
	"errors"
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

// isContextErr reports whether err stems from a canceled/expired request
// context (client gone or WriteTimeout hit while queued for the render
// semaphore) — answered with 503; the write may fail, which is fine.
func isContextErr(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// Preview renders a saved or the active design as a PNG. The ?raw=true query
// param bypasses palette quantization (unquantized original). Per specs/F10 the
// panel_image_mode setting is honoured client-side: the pi client appends
// ?raw=true when the persisted mode is "original", so the server just honours
// the query param here — no server-side setting fallback, keeping the browser
// debug path (?raw=true forces raw, absent/false stays dithered) unchanged.
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
		pngData, err = h.svc.Render(r.Context(), design, raw)
	} else {
		pngData, err = h.svc.RenderActiveRaw(r.Context(), raw)
	}

	if err != nil {
		if isContextErr(err) {
			jsonError(w, "Render canceled", http.StatusServiceUnavailable)
			return
		}
		jsonError(w, "No design", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(pngData)))
	w.Write(pngData)
}

// PreviewLive renders a design from the request body without saving it.
//
// Decision (specs/F10): the designer live preview stays dithered by default and
// does NOT follow panel_image_mode. It is WYSIWYG against the panel's default
// output, and the designer keeps its own explicit ?raw=true debug toggle
// (toolbar.js). The global send-mode setting only affects what the pi client
// requests for the physical panel, never the designer's on-screen preview.
func (h *PreviewHandler) PreviewLive(w http.ResponseWriter, r *http.Request) {
	var design models.DesignV2
	if err := json.NewDecoder(r.Body).Decode(&design); err != nil {
		jsonError(w, "Invalid design data", http.StatusBadRequest)
		return
	}

	raw := r.URL.Query().Get("raw") == "true"
	pngData, err := h.svc.Render(r.Context(), &design, raw)
	if err != nil {
		if isContextErr(err) {
			jsonError(w, "Render canceled", http.StatusServiceUnavailable)
			return
		}
		jsonError(w, "Render failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(pngData)))
	w.Write(pngData)
}
