package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"red-horse-tavern/internal/models"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func (a *App) GetPlayers(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.Query(r.Context(), `
		SELECT id, name, team_id, has_moved, is_current, turn_order
		FROM players
		ORDER BY turn_order
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
		if err := rows.Scan(&p.ID, &p.Name, &p.TeamID, &p.HasMoved, &p.IsCurrent, &p.TurnOrder); err != nil {
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

func (a *App) GetNextPlayer(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.Query(r.Context(), `
		WITH current_player AS (
			SELECT turn_order, team_id
			FROM players 
			WHERE is_current = true 
			LIMIT 1
		),
		opposite_team AS (
			SELECT id
			FROM teams 
			WHERE id != (SELECT team_id FROM current_player)
			ORDER BY id 
			LIMIT 1
		),
		next_player_without_move AS (
			SELECT MIN(p.turn_order) as next_turn_order
			FROM players p
			WHERE p.turn_order > (SELECT turn_order FROM current_player)
			  AND p.has_moved IS FALSE
			  AND p.team_id = (SELECT id FROM opposite_team)
		),
		first_player_from_opposite_team AS (
			SELECT MIN(p.turn_order) as first_turn_order
			FROM players p
			WHERE p.team_id = (SELECT id FROM opposite_team)
		)
		SELECT id, name, team_id, has_moved, is_current, turn_order
		FROM players
		WHERE turn_order = COALESCE(
			(SELECT next_turn_order FROM next_player_without_move),
			(SELECT first_turn_order FROM first_player_from_opposite_team)
		)
		LIMIT 1;
	`)

	if err != nil {
		a.Log.Error("Failed to get next player", "error", err)
		http.Error(w, "Failed to get next player: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	if !rows.Next() {
		// Можно вернуть пустой объект или сообщение
		err := json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "No next player found",
			"player":  nil,
		})
		if err != nil {
			a.Log.Error("Failed to encode next player")
			http.Error(w, "Failed to encode next player: "+err.Error(), http.StatusInternalServerError)
			return
		}
		return
	}

	var player struct {
		ID        int64  `json:"id"`
		Name      string `json:"name"`
		TeamID    int64  `json:"team_id"`
		HasMoved  bool   `json:"has_moved"`
		IsCurrent bool   `json:"is_current"`
		TurnOrder int    `json:"turn_order"`
	}

	err = rows.Scan(&player.ID, &player.Name, &player.TeamID, &player.HasMoved, &player.IsCurrent, &player.TurnOrder)
	if err != nil {
		a.Log.Error("Failed to scan player data", "error", err)
		http.Error(w, "Failed to read player data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Проверяем, нет ли еще строк (не должно быть, так как LIMIT 1)
	if rows.Next() {
		a.Log.Warn("Multiple players returned for next player query")
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(player)
	if err != nil {
		a.Log.Error("Failed to encode response", "error", err)
		http.Error(w, "Failed to encode response: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

func (a *App) UpdatePlayerOrder(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		a.Log.Error("Invalid player ID", "error", err)
		http.Error(w, "Invalid player ID", http.StatusBadRequest)
		return
	}

	var req struct {
		TurnOrder int `json:"turn_order"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.Log.Error("Invalid request", "error", err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.TurnOrder < 1 {
		http.Error(w, "turn_order must be positive", http.StatusBadRequest)
		return
	}

	// Получаем текущий порядок игрока
	var currentOrder int
	err = a.DB.QueryRow(r.Context(),
		"SELECT turn_order FROM players WHERE id = $1", id).Scan(&currentOrder)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "Player not found", http.StatusNotFound)
		} else {
			a.Log.Error("Failed to get player order", "error", err)
			http.Error(w, "Failed to get player order", http.StatusInternalServerError)
		}
		return
	}

	// Если порядок не изменился, ничего не делаем
	if currentOrder == req.TurnOrder {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Смещаем других игроков, чтобы освободить место
	if req.TurnOrder < currentOrder {
		// Двигаем вверх - сдвигаем игроков между новым и старым порядком вниз
		_, err = a.DB.Exec(r.Context(), `
			UPDATE players 
			SET turn_order = turn_order + 1 
			WHERE turn_order >= $1 AND turn_order < $2 AND id != $3
		`, req.TurnOrder, currentOrder, id)
	} else {
		// Двигаем вниз - сдвигаем игроков между старым и новым порядком вверх
		_, err = a.DB.Exec(r.Context(), `
			UPDATE players 
			SET turn_order = turn_order - 1 
			WHERE turn_order > $1 AND turn_order <= $2 AND id != $3
		`, currentOrder, req.TurnOrder, id)
	}

	if err != nil {
		a.Log.Error("Failed to shift players", "error", err)
		http.Error(w, "Failed to update order", http.StatusInternalServerError)
		return
	}

	// Устанавливаем новый порядок игроку
	_, err = a.DB.Exec(r.Context(),
		"UPDATE players SET turn_order = $1 WHERE id = $2",
		req.TurnOrder, id)

	if err != nil {
		a.Log.Error("Failed to update player order", "error", err)
		http.Error(w, "Failed to update order", http.StatusInternalServerError)
		return
	}

	a.broadcast(map[string]interface{}{
		"type": "players_updated",
	})

	w.WriteHeader(http.StatusNoContent)
}
