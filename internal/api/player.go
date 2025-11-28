package api

import (
	"encoding/json"
	"net/http"
	"red-horse-tavern/internal/models"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func (a *App) GetPlayers(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.Query(r.Context(), `
		SELECT id, name, team_id, has_moved, is_current, created_at
		FROM players
		ORDER BY id
	`)
	if err != nil {
		a.Log.Error("Failed to query players", "error", err)
		http.Error(w, "Failed to query players: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var players []models.Player
	for rows.Next() {
		var p models.Player
		if err := rows.Scan(&p.ID, &p.Name, &p.TeamID, &p.HasMoved, &p.IsCurrent, &p.CreatedAt); err != nil {
			a.Log.Error("Failed to scan players", "error", err)
			http.Error(w, "Failed to scan player: "+err.Error(), http.StatusInternalServerError)
			return
		}
		players = append(players, p)
	}

	err = json.NewEncoder(w).Encode(players)
	if err != nil {
		a.Log.Error("Failed to encode response", "error", err)
		http.Error(w, "Failed to encode response: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

func (a *App) CreatePlayer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name   string `json:"name"`
		TeamID int    `json:"team_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		a.Log.Error("Invalid request body", "error", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	_, err := a.DB.Exec(r.Context(),
		`INSERT INTO players (name, team_id) VALUES ($1, $2)`,
		body.Name, body.TeamID,
	)
	if err != nil {
		a.Log.Error("Failed to create player", "error", err)
		http.Error(w, "Failed to create player: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	a.broadcast(map[string]interface{}{
		"type": "players_updated",
	})
}

func (a *App) MarkPlayerMoved(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		a.Log.Error("Invalid player ID", "error", err)
		http.Error(w, "Invalid player ID", http.StatusBadRequest)
		return
	}

	// Переключение значения has_moved
	_, err = a.DB.Exec(r.Context(),
		`UPDATE players SET has_moved = NOT has_moved WHERE id = $1`,
		id,
	)
	if err != nil {
		a.Log.Error("Failed to toggle player", "error", err)
		http.Error(w, "Failed to toggle player: "+err.Error(), http.StatusInternalServerError)
		return
	}

	a.broadcast(map[string]interface{}{"type": "player_moved"})
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) ResetPlayerMove(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		a.Log.Error("Invalid player ID", "error", err)
		http.Error(w, "Invalid player ID", http.StatusBadRequest)
		return
	}

	_, err = a.DB.Exec(r.Context(),
		`UPDATE players SET has_moved = false WHERE id=$1`,
		id,
	)
	if err != nil {
		a.Log.Error("Failed to reset move", "error", err)
		http.Error(w, "Failed to reset move: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *App) DeletePlayer(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		a.Log.Error("Invalid player ID", "error", err)
		http.Error(w, "Invalid player ID", http.StatusBadRequest)
		return
	}

	_, err = a.DB.Exec(r.Context(), `DELETE FROM players WHERE id=$1`, id)
	if err != nil {
		a.Log.Error("Failed to delete player", "error", err)
		http.Error(w, "Failed to delete player: "+err.Error(), http.StatusInternalServerError)
		return
	}

	a.broadcast(map[string]interface{}{
		"type": "player_deleted",
	})
	w.WriteHeader(http.StatusNoContent)
}
