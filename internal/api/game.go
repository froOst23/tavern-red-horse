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
