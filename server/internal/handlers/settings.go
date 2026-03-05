package handlers

import (
	"encoding/json"
	"net/http"
)

func UpdateSettings(w http.ResponseWriter, r *http.Request) {
	// TODO: handle settings update
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Settings updated."})
}
