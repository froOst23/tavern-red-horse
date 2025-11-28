package api

import (
	"encoding/json"
	"net/http"
	"red-horse-tavern/internal/models"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func (a *App) GetTeamsAdmin(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.Query(r.Context(), `
		SELECT id, name, health, drunk, updated_at
		FROM teams
		ORDER BY id
	`)
	if err != nil {
		a.Log.Error("Failed to query teams", "error", err)
		http.Error(w, "Failed to query teams: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var teams []models.Team
	for rows.Next() {
		var t models.Team
		if err := rows.Scan(&t.ID, &t.Name, &t.Health, &t.Drunk, &t.UpdatedAt); err != nil {
			a.Log.Error("Failed to scan team", "error", err)
			http.Error(w, "Failed to scan team: "+err.Error(), http.StatusInternalServerError)
			return
		}
		teams = append(teams, t)
	}

	if err := rows.Err(); err != nil {
		a.Log.Error("Error iterating teams", "error", err)
		http.Error(w, "Error iterating teams: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(teams); err != nil {
		a.Log.Error("Failed to encode teams to JSON", "error", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

func (a *App) CreateTeam(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		a.Log.Error("Error iterating teams", "error", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	_, err := a.DB.Exec(r.Context(), `
		INSERT INTO teams (name, health, drunk, updated_at)
		VALUES ($1, 40, 0, NOW())
	`, body.Name)
	if err != nil {
		a.Log.Error("Failed to create team", "error", err)
		http.Error(w, "Failed to create team: "+err.Error(), http.StatusInternalServerError)
		return
	}

	a.broadcast(map[string]interface{}{
		"type": "team_created",
	})

	w.WriteHeader(http.StatusCreated)
}

func (a *App) UpdateTeamName(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		a.Log.Error("Invalid team id", "error", err)
		http.Error(w, "Invalid team id", http.StatusBadRequest)
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		a.Log.Error("Invalid request body", "error", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	_, err = a.DB.Exec(r.Context(), `
		UPDATE teams SET name=$1, updated_at=NOW() WHERE id=$2
	`, body.Name, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	a.broadcast(map[string]interface{}{
		"type": "teams_updated",
		"data": map[string]interface{}{
			"team_id":  id,
			"field":    "name",
			"new_name": body.Name,
		},
	})

	w.WriteHeader(http.StatusNoContent)
}

func (a *App) UpdateTeamHealth(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		a.Log.Error("Invalid team id", "error", err)
		http.Error(w, "invalid team id", http.StatusBadRequest)
		return
	}

	var body struct {
		Delta int `json:"delta"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		a.Log.Error("Invalid request body", "error", err)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	_, err = a.DB.Exec(r.Context(), `
		UPDATE teams
		SET health = GREATEST(health + $1, 0), updated_at = NOW()
		WHERE id = $2
	`, body.Delta, id)
	if err != nil {
		http.Error(w, "failed to update health: "+err.Error(), http.StatusInternalServerError)
		return
	}

	a.broadcast(map[string]interface{}{
		"type": "teams_updated",
		"data": map[string]interface{}{
			"team_id": id,
			"field":   "health",
			"delta":   body.Delta,
		},
	})

	w.WriteHeader(http.StatusNoContent)
}

func (a *App) UpdateTeamDrunk(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid team id", http.StatusBadRequest)
		return
	}

	var body struct {
		Delta int `json:"delta"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	_, err = a.DB.Exec(r.Context(), `
		UPDATE teams
		SET drunk = GREATEST(drunk + $1, 0), updated_at = NOW()
		WHERE id = $2
	`, body.Delta, id)
	if err != nil {
		http.Error(w, "failed to update drunk: "+err.Error(), http.StatusInternalServerError)
		return
	}

	a.broadcast(map[string]interface{}{
		"type": "teams_updated",
		"data": map[string]interface{}{
			"team_id": id,
			"field":   "drunk",
			"delta":   body.Delta,
		},
	})

	w.WriteHeader(http.StatusNoContent)
}

func (a *App) ResetTeams(w http.ResponseWriter, r *http.Request) {
	const defaultHealth = 40
	const defaultDrunk = 0

	// Сбрасываем команды
	_, err := a.DB.Exec(r.Context(), `
        UPDATE teams
        SET health=$1, drunk=$2, updated_at=NOW()
    `, defaultHealth, defaultDrunk)

	if err != nil {
		http.Error(w, "Failed to reset teams: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Сбрасываем ход игроков
	_, err = a.DB.Exec(r.Context(), `
		UPDATE players
		SET has_moved = false, is_current = false
	`)

	// Выбираем следующего игрока
	_, err = a.DB.Exec(r.Context(), `
		UPDATE players
		SET has_moved = false, is_current = false
	`)

	// Выбираем ход случайного игрока
	_, err = a.DB.Exec(r.Context(), `
		UPDATE players
		SET is_current = true
		WHERE id = (
		SELECT id
		FROM players
		ORDER BY RANDOM()
		LIMIT 1)
	`)

	// Сбрасываем счетчик раунда
	_, err = a.DB.Exec(r.Context(), `
		UPDATE game_state
		SET current_round = 1, event_counter = 0
	`)

	// Сбрасываем события - все не пройденные, снимаем текущее
	_, err = a.DB.Exec(r.Context(), `
        UPDATE events 
        SET used = false, current = false
    `)
	if err != nil {
		http.Error(w, "Failed to reset events: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Сначала пытаемся найти init событие
	var initEvent models.Event
	err = a.DB.QueryRow(r.Context(), `
        SELECT id, title, description, image_path, used, created_at
        FROM events 
        WHERE init = true
        LIMIT 1
    `).Scan(&initEvent.ID, &initEvent.Title, &initEvent.Description, &initEvent.ImagePath, &initEvent.Used, &initEvent.CreatedAt)

	var currentEventData interface{}

	if err == nil {
		// Если нашли init событие, устанавливаем его как текущее
		_, err = a.DB.Exec(r.Context(), `UPDATE events SET current = true WHERE id = $1`, initEvent.ID)
		if err != nil {
			a.Log.Error("Failed to set init event as current", "error", err)
		} else {
			// Обновляем данные события после установки флага current
			err = a.DB.QueryRow(r.Context(), `
                SELECT id, title, description, image_path, used, current, created_at
                FROM events WHERE id = $1
            `, initEvent.ID).Scan(&initEvent.ID, &initEvent.Title, &initEvent.Description, &initEvent.ImagePath, &initEvent.Used, &initEvent.Current, &initEvent.CreatedAt)
			if err == nil {
				currentEventData = initEvent
			}
		}
	} else {
		// Если init события нет, выбираем случайное событие как текущее
		var randomEvent models.Event
		err = a.DB.QueryRow(r.Context(), `
            SELECT id, title, description, image_path, used, created_at
            FROM events 
            ORDER BY RANDOM() 
            LIMIT 1
        `).Scan(&randomEvent.ID, &randomEvent.Title, &randomEvent.Description, &randomEvent.ImagePath, &randomEvent.Used, &randomEvent.CreatedAt)

		if err == nil {
			// Если есть события, устанавливаем случайное как текущее
			_, err = a.DB.Exec(r.Context(), `UPDATE events SET current = true WHERE id = $1`, randomEvent.ID)
			if err != nil {
				a.Log.Error("Failed to set random event as current", "error", err)
			} else {
				// Обновляем данные события после установки флага current
				err = a.DB.QueryRow(r.Context(), `
                    SELECT id, title, description, image_path, used, current, created_at
                    FROM events WHERE id = $1
                `, randomEvent.ID).Scan(&randomEvent.ID, &randomEvent.Title, &randomEvent.Description, &randomEvent.ImagePath, &randomEvent.Used, &randomEvent.Current, &randomEvent.CreatedAt)
				if err == nil {
					currentEventData = randomEvent
				}
			}
		}
	}

	a.broadcast(map[string]interface{}{
		"type": "player_reset",
		"data": map[string]interface{}{
			"message": "Players' moves are reset",
		},
	})

	// Отправляем отдельные broadcast для команд и событий
	a.broadcast(map[string]interface{}{
		"type": "teams_reset",
		"data": map[string]interface{}{
			"message": "All commands are reset",
		},
	})

	a.broadcast(map[string]interface{}{
		"type": "events_reset",
		"data": map[string]interface{}{
			"message":           "All events are reset",
			"new_current_event": currentEventData,
		},
	})

	// И общий broadcast для полного обновления
	a.broadcast(map[string]interface{}{
		"type": "world_reset",
		"data": map[string]interface{}{
			"message":           "The world is reborn",
			"teams_reset":       true,
			"events_reset":      true,
			"new_current_event": currentEventData,
		},
	})

	w.WriteHeader(http.StatusNoContent)
}
