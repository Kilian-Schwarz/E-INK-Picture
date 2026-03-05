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
	h.preview.FillContent(design)
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
	h.preview.FillContent(d)
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

	// Re-marshal the full data into a Design struct
	raw, _ := json.Marshal(data)
	var design models.Design
	json.Unmarshal(raw, &design)

	timestamp := time.Now().Format("2006-01-02_15-04-05")

	if saveAsNew {
		// Deactivate all existing designs
		allDesigns, err := h.svc.ListFull()
		if err != nil {
			jsonError(w, "Server error", http.StatusInternalServerError)
			return
		}
		for i := range allDesigns {
			allDesigns[i].Active = false
			h.svc.Save(allDesigns[i].Name, &allDesigns[i])
		}

		design.Timestamp = timestamp
		design.Active = true
		design.Name = designName
		design.KeepAlive = keepAlive
		design.Filename = "design_" + timestamp + ".json"

		if err := h.svc.Save(designName, &design); err != nil {
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

		// Ensure all others are inactive
		allDesigns, _ := h.svc.ListFull()
		for i := range allDesigns {
			if allDesigns[i].Filename != active.Filename {
				allDesigns[i].Active = false
				h.svc.Save(allDesigns[i].Name, &allDesigns[i])
			}
		}

		if err := h.svc.Save(designName, &design); err != nil {
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

// jsonResponse writes a JSON response with the given status code.
func jsonResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// jsonError writes a JSON error response.
func jsonError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"message": message})
}
