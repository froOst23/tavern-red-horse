package api

import (
	"context"
	"encoding/json"
	"net/http"
	"red-horse-tavern/internal/models"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func (a *App) GetPlayers(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.Query(r.Context(), `
        SELECT id, name, team_id, has_moved, is_current, turn_order
        FROM players
        ORDER BY turn_order NULLS LAST, id
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

func (a *App) SetPlayerTurnOrder(w http.ResponseWriter, r *http.Request) {
	var players []struct {
		ID        int `json:"id"`
		TurnOrder int `json:"turn_order"`
	}

	if err := json.NewDecoder(r.Body).Decode(&players); err != nil {
		a.Log.Error("Invalid request body", "error", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		a.Log.Error("Failed to begin transaction", "error", err)
		http.Error(w, "Failed to update turn order", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	// Сбрасываем текущий порядок
	_, err = tx.Exec(r.Context(), `UPDATE players SET turn_order = NULL`)
	if err != nil {
		a.Log.Error("Failed to reset turn order", "error", err)
		http.Error(w, "Failed to update turn order", http.StatusInternalServerError)
		return
	}

	// Устанавливаем новый порядок
	for _, p := range players {
		_, err = tx.Exec(r.Context(),
			`UPDATE players SET turn_order = $1 WHERE id = $2`,
			p.TurnOrder, p.ID)
		if err != nil {
			a.Log.Error("Failed to update player turn order", "error", err)
			http.Error(w, "Failed to update turn order", http.StatusInternalServerError)
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		a.Log.Error("Failed to commit transaction", "error", err)
		http.Error(w, "Failed to update turn order", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	a.broadcast(map[string]interface{}{
		"type": "turn_order_updated",
	})
}

func (a *App) SkipPlayerTurn(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		a.Log.Error("Invalid player ID", "error", err)
		http.Error(w, "Invalid player ID", http.StatusBadRequest)
		return
	}

	// Получаем информацию о текущем игроке
	var currentPlayer models.Player
	err = a.DB.QueryRow(r.Context(), `
		SELECT id, name, team_id, turn_order 
		FROM players WHERE id = $1
	`, id).Scan(&currentPlayer.ID, &currentPlayer.Name, &currentPlayer.TeamID, &currentPlayer.TurnOrder)
	if err != nil {
		a.Log.Error("Failed to get player", "error", err)
		http.Error(w, "Player not found", http.StatusNotFound)
		return
	}

	// Находим следующего игрока в порядке хода
	var nextPlayerID int
	err = a.DB.QueryRow(r.Context(), `
		SELECT id FROM players 
		WHERE turn_order > $1 
		ORDER BY turn_order 
		LIMIT 1
	`, currentPlayer.TurnOrder).Scan(&nextPlayerID)

	if err != nil {
		// Если это последний игрок, берем первого
		err = a.DB.QueryRow(r.Context(), `
			SELECT id FROM players 
			WHERE turn_order IS NOT NULL 
			ORDER BY turn_order 
			LIMIT 1
		`).Scan(&nextPlayerID)
		if err != nil {
			a.Log.Error("No players in turn order", "error", err)
			http.Error(w, "No players in turn order", http.StatusBadRequest)
			return
		}
	}

	// Помечаем текущего игрока как сходившего и пропускающего ход
	_, err = a.DB.Exec(r.Context(), `
		UPDATE players SET has_moved = true, is_current = false WHERE id = $1
	`, id)
	if err != nil {
		a.Log.Error("Failed to mark player as moved", "error", err)
		http.Error(w, "Failed to skip turn", http.StatusInternalServerError)
		return
	}

	// Устанавливаем следующего игрока как текущего
	_, err = a.DB.Exec(r.Context(), `
		UPDATE players SET is_current = true WHERE id = $1
	`, nextPlayerID)
	if err != nil {
		a.Log.Error("Failed to set next player as current", "error", err)
		http.Error(w, "Failed to skip turn", http.StatusInternalServerError)
		return
	}

	a.broadcast(map[string]interface{}{
		"type": "turn_skipped",
		"data": map[string]interface{}{
			"skipped_player_id": id,
			"next_player_id":    nextPlayerID,
		},
	})
	w.WriteHeader(http.StatusOK)
}

func (a *App) GetNextPlayer(ctx context.Context) (int, error) {
	var nextPlayerID int

	// Находим текущего игрока
	var currentTurnOrder int
	err := a.DB.QueryRow(ctx, `
        SELECT turn_order FROM players WHERE is_current = true
    `).Scan(&currentTurnOrder)

	if err != nil {
		// Если текущего нет, берем первого
		err = a.DB.QueryRow(ctx, `
            SELECT id FROM players 
            WHERE turn_order IS NOT NULL 
            ORDER BY turn_order 
            LIMIT 1
        `).Scan(&nextPlayerID)
	} else {
		// Ищем следующего игрока
		err = a.DB.QueryRow(ctx, `
            SELECT id FROM players 
            WHERE turn_order > $1 
            ORDER BY turn_order 
            LIMIT 1
        `, currentTurnOrder).Scan(&nextPlayerID)

		if err != nil {
			// Если это последний, берем первого
			err = a.DB.QueryRow(ctx, `
                SELECT id FROM players 
                WHERE turn_order IS NOT NULL 
                ORDER BY turn_order 
                LIMIT 1
            `).Scan(&nextPlayerID)
		}
	}

	return nextPlayerID, err
}
