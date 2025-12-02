package api

import (
	"encoding/json"
	"net/http"
	"red-horse-tavern/internal/models"
)

func (a *App) GetGameState(w http.ResponseWriter, r *http.Request) {
	var gs models.GameState

	err := a.DB.QueryRow(r.Context(),
		`SELECT id, current_round FROM game_state LIMIT 1`,
	).Scan(&gs.ID, &gs.CurrentRound)

	if err != nil {
		a.Log.Error("Failed to load game state", "error", err)
		http.Error(w, "Failed to load game state: "+err.Error(), http.StatusInternalServerError)
		return
	}

	err = json.NewEncoder(w).Encode(gs)
	if err != nil {
		a.Log.Error("Failed to encode game state", "error", err)
		http.Error(w, "Failed to encode game state: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

func (a *App) NextRound(w http.ResponseWriter, r *http.Request) {
	_, err := a.DB.Exec(r.Context(), `
		UPDATE game_state 
		SET current_round = current_round + 1
	`)
	if err != nil {
		a.Log.Error("Failed to next round", "error", err)
		http.Error(w, "Failed to next round: "+err.Error(), http.StatusInternalServerError)
		return
	}

	a.broadcast(map[string]interface{}{
		"type": "round_updated",
	})
	w.WriteHeader(http.StatusNoContent)
}
