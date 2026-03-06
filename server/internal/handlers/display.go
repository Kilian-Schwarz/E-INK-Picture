package handlers

import (
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
	if h.clientURL == "" {
		jsonError(w, "E-Ink client not configured (set EINK_CLIENT_URL)", http.StatusServiceUnavailable)
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(h.clientURL+"/refresh", "application/json", nil)
	if err != nil {
		slog.Error("failed to reach e-ink client", "url", h.clientURL, "error", err)
		jsonError(w, fmt.Sprintf("Failed to reach E-Ink client: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		jsonError(w, fmt.Sprintf("E-Ink client returned status %d", resp.StatusCode), http.StatusBadGateway)
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "Display refresh triggered",
	})
}
