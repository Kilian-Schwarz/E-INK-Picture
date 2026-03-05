package handlers

import (
	"encoding/json"
	"net/http"

	"e-ink-picture/server/internal/services"
)

type DesignHandler struct {
	svc *services.DesignService
}

func NewDesignHandler(svc *services.DesignService) *DesignHandler {
	return &DesignHandler{svc: svc}
}

func (h *DesignHandler) GetActive(w http.ResponseWriter, r *http.Request) {
	// TODO: call h.svc.GetActive() and return JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{})
}

func (h *DesignHandler) List(w http.ResponseWriter, r *http.Request) {
	// TODO: call h.svc.List() and return JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode([]any{})
}

func (h *DesignHandler) GetByName(w http.ResponseWriter, r *http.Request) {
	// TODO: get name from query param, call h.svc.Get(name)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{})
}

func (h *DesignHandler) SetActive(w http.ResponseWriter, r *http.Request) {
	// TODO: parse JSON body, call h.svc.SetActive(name)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Active design set."})
}

func (h *DesignHandler) Update(w http.ResponseWriter, r *http.Request) {
	// TODO: parse design JSON, call h.svc.Save()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Design updated."})
}

func (h *DesignHandler) Clone(w http.ResponseWriter, r *http.Request) {
	// TODO: parse JSON body, call h.svc.Clone()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Design cloned"})
}

func (h *DesignHandler) Delete(w http.ResponseWriter, r *http.Request) {
	// TODO: parse JSON body, call h.svc.Delete()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Design deleted"})
}
