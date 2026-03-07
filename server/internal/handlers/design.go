package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"e-ink-picture/server/internal/models"
	"e-ink-picture/server/internal/services"
)

type DesignHandler struct {
	svc     *services.DesignService
	preview *services.PreviewService
}

func NewDesignHandler(svc *services.DesignService, preview *services.PreviewService) *DesignHandler {
	return &DesignHandler{svc: svc, preview: preview}
}

func (h *DesignHandler) GetActive(w http.ResponseWriter, r *http.Request) {
	design, err := h.svc.GetActive()
	if err != nil {
		if errors.Is(err, services.ErrDesignNotFound) {
			jsonError(w, "No design", http.StatusNotFound)
			return
		}
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, design)
}

func (h *DesignHandler) List(w http.ResponseWriter, r *http.Request) {
	designs, err := h.svc.ListFull()
	if err != nil {
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, designs)
}

func (h *DesignHandler) GetByName(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		jsonError(w, "Missing name parameter", http.StatusBadRequest)
		return
	}
	d, err := h.svc.Get(name)
	if err != nil {
		if errors.Is(err, services.ErrDesignNotFound) {
			jsonError(w, "Design not found", http.StatusNotFound)
			return
		}
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, d)
}

func (h *DesignHandler) SetActive(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if err := h.svc.SetActive(body.Name); err != nil {
		if errors.Is(err, services.ErrDesignNotFound) {
			jsonError(w, "Design not found", http.StatusNotFound)
			return
		}
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"message": "Active design set."})
}

func (h *DesignHandler) Update(w http.ResponseWriter, r *http.Request) {
	var data map[string]any
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	saveAsNew, _ := data["save_as_new"].(bool)
	designName, _ := data["name"].(string)
	if designName == "" {
		designName = "Unnamed Design"
	}
	keepAlive, _ := data["keep_alive"].(bool)

	// Re-marshal the full data into a DesignV2 struct
	raw, err := json.Marshal(data)
	if err != nil {
		jsonError(w, "Invalid request data", http.StatusBadRequest)
		return
	}
	var design models.DesignV2
	if err := json.Unmarshal(raw, &design); err != nil {
		jsonError(w, "Invalid design data", http.StatusBadRequest)
		return
	}

	// Ensure version is set
	if design.Version < 2 {
		design.Version = 2
	}

	timestamp := time.Now().Format("2006-01-02_15-04-05")

	if saveAsNew {
		design.Timestamp = timestamp
		design.Active = true
		design.Name = designName
		design.KeepAlive = keepAlive
		design.Filename = "design_" + timestamp + ".json"

		if err := h.svc.Save(designName, &design); err != nil {
			jsonError(w, "Server error", http.StatusInternalServerError)
			return
		}
		if err := h.svc.SetActive(designName); err != nil {
			jsonError(w, "Server error", http.StatusInternalServerError)
			return
		}
		jsonResponse(w, http.StatusOK, map[string]string{"message": "New design saved and set active."})
	} else {
		active, err := h.svc.GetActive()
		if err != nil {
			if errors.Is(err, services.ErrDesignNotFound) {
				jsonError(w, "No active design found.", http.StatusNotFound)
				return
			}
			jsonError(w, "Server error", http.StatusInternalServerError)
			return
		}

		design.Timestamp = active.Timestamp
		design.Active = true
		design.Name = designName
		design.KeepAlive = keepAlive
		design.Filename = active.Filename

		if err := h.svc.Save(designName, &design); err != nil {
			jsonError(w, "Server error", http.StatusInternalServerError)
			return
		}
		if err := h.svc.SetActive(designName); err != nil {
			jsonError(w, "Server error", http.StatusInternalServerError)
			return
		}
		jsonResponse(w, http.StatusOK, map[string]string{"message": "Design updated successfully!"})
	}
}

func (h *DesignHandler) Clone(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	target := body.Name + " (Clone)"
	if err := h.svc.Clone(body.Name, target); err != nil {
		if errors.Is(err, services.ErrDesignNotFound) {
			jsonError(w, "Design not found", http.StatusNotFound)
			return
		}
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"message": "Design cloned"})
}

func (h *DesignHandler) Delete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if err := h.svc.Delete(body.Name); err != nil {
		if errors.Is(err, services.ErrDesignNotFound) {
			jsonError(w, "Design not found", http.StatusNotFound)
			return
		}
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"message": "Design deleted"})
}

// --- New Design Management API endpoints ---

// APIListDesigns returns design cards for the dashboard.
// GET /api/designs
func (h *DesignHandler) APIListDesigns(w http.ResponseWriter, r *http.Request) {
	cards, err := h.svc.ListCards()
	if err != nil {
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{
		"designs": cards,
	})
}

// APICreateDesign creates a new design.
// POST /api/designs
func (h *DesignHandler) APICreateDesign(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string            `json:"name"`
		Elements []models.Element  `json:"elements"`
		Canvas   models.CanvasConfig `json:"canvas"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if body.Name == "" {
		jsonError(w, "Name is required", http.StatusBadRequest)
		return
	}

	d, err := h.svc.CreateDesign(body.Name, body.Elements, body.Canvas)
	if err != nil {
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, d)
}

// APIGetDesign returns a design by ID.
// GET /api/designs/{id}
func (h *DesignHandler) APIGetDesign(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		jsonError(w, "Missing design ID", http.StatusBadRequest)
		return
	}
	d, err := h.svc.GetByID(id)
	if err != nil {
		if errors.Is(err, services.ErrDesignNotFound) {
			jsonError(w, "Design not found", http.StatusNotFound)
			return
		}
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, d)
}

// APIUpdateDesign updates a design by ID (full update or partial).
// PUT /api/designs/{id}
func (h *DesignHandler) APIUpdateDesign(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		jsonError(w, "Missing design ID", http.StatusBadRequest)
		return
	}

	var body struct {
		Name      string              `json:"name"`
		Elements  []models.Element    `json:"elements"`
		Canvas    models.CanvasConfig `json:"canvas"`
		KeepAlive bool                `json:"keep_alive"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	d, err := h.svc.UpdateDesignByID(id, body.Name, body.Elements, body.Canvas, body.KeepAlive)
	if err != nil {
		if errors.Is(err, services.ErrDesignNotFound) {
			jsonError(w, "Design not found", http.StatusNotFound)
			return
		}
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, d)
}

// APIPatchDesign partially updates a design (e.g. rename).
// PATCH /api/designs/{id}
func (h *DesignHandler) APIPatchDesign(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		jsonError(w, "Missing design ID", http.StatusBadRequest)
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if body.Name == "" {
		jsonError(w, "Name is required", http.StatusBadRequest)
		return
	}

	d, err := h.svc.RenameDesign(id, body.Name)
	if err != nil {
		if errors.Is(err, services.ErrDesignNotFound) {
			jsonError(w, "Design not found", http.StatusNotFound)
			return
		}
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, d)
}

// APIDeleteDesign deletes a design by ID.
// DELETE /api/designs/{id}
func (h *DesignHandler) APIDeleteDesign(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		jsonError(w, "Missing design ID", http.StatusBadRequest)
		return
	}

	if err := h.svc.DeleteByID(id); err != nil {
		if errors.Is(err, services.ErrDesignNotFound) {
			jsonError(w, "Design not found", http.StatusNotFound)
			return
		}
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"message": "Design deleted"})
}

// APIActivateDesign sets a design as active on the display.
// POST /api/designs/{id}/activate
func (h *DesignHandler) APIActivateDesign(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		jsonError(w, "Missing design ID", http.StatusBadRequest)
		return
	}

	if err := h.svc.ActivateByID(id); err != nil {
		if errors.Is(err, services.ErrDesignNotFound) {
			jsonError(w, "Design not found", http.StatusNotFound)
			return
		}
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"message": "Design activated"})
}

// APIDuplicateDesign creates a copy of a design.
// POST /api/designs/{id}/duplicate
func (h *DesignHandler) APIDuplicateDesign(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		jsonError(w, "Missing design ID", http.StatusBadRequest)
		return
	}

	d, err := h.svc.DuplicateDesign(id)
	if err != nil {
		if errors.Is(err, services.ErrDesignNotFound) {
			jsonError(w, "Design not found", http.StatusNotFound)
			return
		}
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, d)
}

// APIGetActiveDesign returns the currently active design.
// GET /api/designs/active
func (h *DesignHandler) APIGetActiveDesign(w http.ResponseWriter, r *http.Request) {
	d, err := h.svc.GetActiveDesign()
	if err != nil {
		if errors.Is(err, services.ErrDesignNotFound) {
			jsonError(w, "No active design", http.StatusNotFound)
			return
		}
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, d)
}

// APIGetHistory returns history entries for a design.
// GET /api/designs/{id}/history
func (h *DesignHandler) APIGetHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		jsonError(w, "Missing design ID", http.StatusBadRequest)
		return
	}

	entries, err := h.svc.ListHistory(id)
	if err != nil {
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, entries)
}

// APIGetHistorySnapshot returns a specific history snapshot.
// GET /api/designs/{id}/history/{timestamp}
func (h *DesignHandler) APIGetHistorySnapshot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	timestamp := r.PathValue("timestamp")

	if id == "" || timestamp == "" {
		jsonError(w, "Missing parameters", http.StatusBadRequest)
		return
	}

	snap, err := h.svc.GetHistorySnapshot(id, timestamp)
	if err != nil {
		if errors.Is(err, services.ErrDesignNotFound) {
			jsonError(w, "Snapshot not found", http.StatusNotFound)
			return
		}
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, snap)
}

// APIRestoreHistorySnapshot restores a design from a history snapshot.
// POST /api/designs/{id}/history/{timestamp}/restore
func (h *DesignHandler) APIRestoreHistorySnapshot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	timestamp := r.PathValue("timestamp")

	if id == "" || timestamp == "" {
		jsonError(w, "Missing parameters", http.StatusBadRequest)
		return
	}

	d, err := h.svc.RestoreHistorySnapshot(id, timestamp)
	if err != nil {
		if errors.Is(err, services.ErrDesignNotFound) {
			jsonError(w, "Snapshot not found", http.StatusNotFound)
			return
		}
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, d)
}
