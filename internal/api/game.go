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

func (a *App) ResetRounds(w http.ResponseWriter, r *http.Request) {
	_, err := a.DB.Exec(r.Context(),
		`UPDATE game_state SET current_round = 1, event_counter = 0`,
	)
	if err != nil {
		a.Log.Error("Failed to reset rounds", "error", err)
		http.Error(w, "Failed to reset rounds: "+err.Error(), http.StatusInternalServerError)
		return
	}

	a.broadcast(map[string]interface{}{
		"type": "round_reset",
	})
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) CheckRoundProgress(w http.ResponseWriter, r *http.Request) {
	var total, moved int

	err := a.DB.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM players`,
	).Scan(&total)
	if err != nil {
		a.Log.Error("Failed to count players", "error", err)
		http.Error(w, "Failed to count players: "+err.Error(), http.StatusInternalServerError)
		return
	}

	err = a.DB.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM players WHERE has_moved = true`,
	).Scan(&moved)
	if err != nil {
		a.Log.Error("Failed to count moves", "error", err)
		http.Error(w, "Failed to count moves: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if total > 0 && total == moved {
		// Все игроки сходили → следующий раунд
		_, err = a.DB.Exec(r.Context(),
			`UPDATE game_state SET current_round = current_round + 1`,
		)
		if err != nil {
			a.Log.Error("Failed to next round", "error", err)
			http.Error(w, "Failed to next round: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Сбрасываем has_moved всем игрокам
		_, _ = a.DB.Exec(r.Context(), `UPDATE players SET has_moved = false`)

		a.broadcast(map[string]interface{}{
			"type": "round_auto_advanced",
		})
	}

	err = json.NewEncoder(w).Encode(map[string]int{
		"total": total,
		"moved": moved,
	})
	if err != nil {
		a.Log.Error("Failed to encode game state", "error", err)
		http.Error(w, "Failed to encode game state: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

func (a *App) AutoArrangeTurnOrder(w http.ResponseWriter, r *http.Request) {
	// Получаем игроков по командам
	team1Players := []models.Player{}
	team2Players := []models.Player{}

	rows, err := a.DB.Query(r.Context(), `
        SELECT id, name, team_id FROM players ORDER BY team_id, id
    `)
	if err != nil {
		a.Log.Error("Failed to query players", "error", err)
		http.Error(w, "Failed to arrange turn order", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var p models.Player
		if err := rows.Scan(&p.ID, &p.Name, &p.TeamID); err != nil { // Убрали &p.TurnOrder
			a.Log.Error("Failed to scan player", "error", err)
			continue
		}

		if p.TeamID == 1 {
			team1Players = append(team1Players, p)
		} else {
			team2Players = append(team2Players, p)
		}
	}

	// Остальной код без изменений...
	// Чередуем игроков из разных команд
	turnOrder := 1
	maxLen := len(team1Players)
	if len(team2Players) > maxLen {
		maxLen = len(team2Players)
	}

	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		a.Log.Error("Failed to begin transaction", "error", err)
		http.Error(w, "Failed to arrange turn order", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	// Сбрасываем текущий порядок
	_, err = tx.Exec(r.Context(), `UPDATE players SET turn_order = NULL, is_current = false`)
	if err != nil {
		a.Log.Error("Failed to reset turn order", "error", err)
		http.Error(w, "Failed to arrange turn order", http.StatusInternalServerError)
		return
	}

	// Устанавливаем новый порядок (чередование команд)
	for i := 0; i < maxLen; i++ {
		if i < len(team1Players) {
			_, err = tx.Exec(r.Context(),
				`UPDATE players SET turn_order = $1 WHERE id = $2`,
				turnOrder, team1Players[i].ID)
			if err != nil {
				a.Log.Error("Failed to update player turn order", "error", err)
				http.Error(w, "Failed to arrange turn order", http.StatusInternalServerError)
				return
			}
			turnOrder++
		}

		if i < len(team2Players) {
			_, err = tx.Exec(r.Context(),
				`UPDATE players SET turn_order = $1 WHERE id = $2`,
				turnOrder, team2Players[i].ID)
			if err != nil {
				a.Log.Error("Failed to update player turn order", "error", err)
				http.Error(w, "Failed to arrange turn order", http.StatusInternalServerError)
				return
			}
			turnOrder++
		}
	}

	// Устанавливаем первого игрока как текущего
	_, err = tx.Exec(r.Context(), `
        UPDATE players SET is_current = true 
        WHERE turn_order = (SELECT MIN(turn_order) FROM players WHERE turn_order IS NOT NULL)
    `)
	if err != nil {
		a.Log.Error("Failed to set first player as current", "error", err)
		http.Error(w, "Failed to arrange turn order", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		a.Log.Error("Failed to commit transaction", "error", err)
		http.Error(w, "Failed to arrange turn order", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	a.broadcast(map[string]interface{}{
		"type": "turn_order_arranged",
	})
}
