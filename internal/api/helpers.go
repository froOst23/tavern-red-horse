package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func (a *App) logError(w http.ResponseWriter, message string, err error) {
	a.Log.Error(message, "error", err)
	http.Error(w, fmt.Sprintf("%s: %v", message, err), http.StatusInternalServerError)
}

func (a *App) respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		a.Log.Error("Failed to encode response", "error", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}
