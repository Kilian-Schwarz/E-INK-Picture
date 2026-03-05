package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type DisplayHandler struct {
	clientURL string
}

func NewDisplayHandler(clientURL string) *DisplayHandler {
	return &DisplayHandler{clientURL: clientURL}
}

func (h *DisplayHandler) RefreshDisplay(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.clientURL == "" {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "E-Ink client not configured (set EINK_CLIENT_URL)",
		})
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(h.clientURL+"/refresh", "application/json", nil)
	if err != nil {
		slog.Error("failed to reach e-ink client", "url", h.clientURL, "error", err)
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": fmt.Sprintf("Failed to reach E-Ink client: %v", err),
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": fmt.Sprintf("E-Ink client returned status %d", resp.StatusCode),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": "Display refresh triggered",
	})
}
