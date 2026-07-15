package handlers

import (
	"encoding/json"
	"net/http"
)

// HealthCheck returns the public health probe handler. The version string is
// stamped at build time via -ldflags "-X main.version=..." and defaults to
// "dev" (specs/E6.2-release-workflow.md AC1).
func HealthCheck(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"version": version,
		}); err != nil {
			http.Error(w, "encoding error", http.StatusInternalServerError)
		}
	}
}
