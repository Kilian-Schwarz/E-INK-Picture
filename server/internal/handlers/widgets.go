package handlers

import (
	"encoding/json"
	"net/http"

	"e-ink-picture/server/internal/services"
	"e-ink-picture/server/internal/services/widgets"
)

// WidgetHandler provides widget data API endpoints.
type WidgetHandler struct {
	preview *services.PreviewService
}

// NewWidgetHandler creates a new WidgetHandler. preview provides the shared
// WidgetTextContent dispatch behind POST /api/widget_content.
func NewWidgetHandler(preview *services.PreviewService) *WidgetHandler {
	return &WidgetHandler{
		preview: preview,
	}
}

// contentRequest is the POST /api/widget_content body: a widget/text element
// type plus the full properties map the panel would draw.
type contentRequest struct {
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
}

// Content returns the server-rendered text content for a widget/text element.
// It routes the FULL properties map through PreviewService.WidgetTextContent —
// the exact same dispatch the panel renderer uses — so the editor preview and
// the E-Ink panel share one content source. Element types without server-side
// text content (image, shape, unknown) yield 400.
func (h *WidgetHandler) Content(w http.ResponseWriter, r *http.Request) {
	var req contentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	content, ok := h.preview.WidgetTextContent(req.Type, req.Properties)
	if !ok {
		jsonError(w, "unsupported widget type", http.StatusBadRequest)
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{"content": content})
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
