package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"red-horse-tavern/internal/models"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/minio/minio-go/v7"
)

func (a *App) GetEvent(w http.ResponseWriter, r *http.Request) {
	// Сначала проверяем, есть ли неиспользованный инициализирующий ивент
	var hasUnusedInitEvent bool
	err := a.DB.QueryRow(r.Context(), `
        SELECT EXISTS(
            SELECT 1 FROM events 
            WHERE init = true AND used = false
        )
    `).Scan(&hasUnusedInitEvent)
	if err != nil {
		a.Log.Error("Failed to check init event", "error", err)
		http.Error(w, "Failed to check init events: "+err.Error(), http.StatusInternalServerError)
		return
	}

	a.Log.Debug("Found init events", "init", hasUnusedInitEvent)

	var rows pgx.Rows
	if hasUnusedInitEvent {
		// Если есть неиспользованный init ивент, выводим сначала его, потом остальные
		rows, err = a.DB.Query(r.Context(), `
            SELECT id, title, description, type, difficult, victory_effect, defeat_effect, requirement, image_path, current, used, init, created_at
            FROM events
            ORDER BY 
                CASE WHEN init = true AND used = false THEN 0 ELSE 1 END,
                id
        `)
	} else {
		// Если нет неиспользованного init ивента, выводим в случайном порядке
		rows, err = a.DB.Query(r.Context(), `
            SELECT id, title, description, type, difficult, victory_effect, defeat_effect, requirement, image_path, current, used, init, created_at
            FROM events
            ORDER BY RANDOM()
        `)
	}
	if err != nil {
		a.Log.Error("Failed to query events", "error", err)
		http.Error(w, "Failed to query events: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var events []models.Event
	for rows.Next() {
		var e models.Event
		if err := rows.Scan(
			&e.ID,
			&e.Title,
			&e.Description,
			&e.Type,
			&e.Difficult,
			&e.VictoryEffect,
			&e.DefeatEffect,
			&e.Requirement,
			&e.ImagePath,
			&e.Current,
			&e.Used,
			&e.Init,
			&e.CreatedAt,
		); err != nil {
			a.Log.Error("Failed to scan event", "error", err)
			http.Error(w, "Failed to scan event: "+err.Error(), http.StatusInternalServerError)
			return
		}

		events = append(events, models.Event{
			ID:            e.ID,
			Title:         e.Title,
			Description:   e.Description,
			Type:          e.Type,
			Difficult:     e.Difficult,
			VictoryEffect: e.VictoryEffect,
			DefeatEffect:  e.DefeatEffect,
			Requirement:   e.Requirement,
			ImagePath:     e.ImagePath,
			Current:       e.Current,
			Used:          e.Used,
			Init:          e.Init,
			CreatedAt:     e.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(events)
	if err != nil {
		a.Log.Error("Failed to encode events", "error", err)
		http.Error(w, "Failed to encode events: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

func (a *App) CreateEvent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title         string `json:"title"`
		Description   string `json:"description"`
		Type          string `json:"type"`
		Difficult     string `json:"difficult"`
		VictoryEffect string `json:"victory_effect"`
		DefeatEffect  string `json:"defeat_effect"`
		Requirement   string `json:"requirement"`
		Init          bool   `json:"init"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Title == "" {
		a.Log.Error("Failed to decode body", "error", err)
		http.Error(w, "Failed to decode body", http.StatusBadRequest)
		return
	}

	var id int
	err := a.DB.QueryRow(r.Context(), `
		INSERT INTO events (
			title, description, type, difficult, 
			victory_effect, defeat_effect, requirement, 
			current, used, init, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, false, false, $8, NOW())
		RETURNING id`,
		body.Title, body.Description, body.Type, body.Difficult,
		body.VictoryEffect, body.DefeatEffect, body.Requirement, body.Init,
	).Scan(&id)
	if err != nil {
		a.Log.Error("Failed to create event", "error", err)
		http.Error(w, "Failed to create event: "+err.Error(), http.StatusInternalServerError)
		return
	}

	a.broadcast(map[string]interface{}{
		"type": "event_created",
		"data": map[string]interface{}{"id": id, "title": body.Title},
	})

	w.WriteHeader(http.StatusCreated)
	err = json.NewEncoder(w).Encode(map[string]int{"id": id})
	if err != nil {
		a.Log.Error("Failed to encode event", "error", err)
		http.Error(w, "Failed to encode events: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

func (a *App) DeleteEvent(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		a.Log.Error("Invalid event id", "error", err)
		http.Error(w, "Invalid event id", http.StatusBadRequest)
		return
	}

	// Сначала получаем информацию о событии, чтобы узнать путь к изображению
	var imagePath *string
	err = a.DB.QueryRow(r.Context(), "SELECT image_path FROM events WHERE id = $1", id).Scan(&imagePath)
	if err != nil {
		a.Log.Error("Failed to find event", "error", err)
		http.Error(w, "Failed to find event: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Удаляем событие из базы данных
	_, err = a.DB.Exec(r.Context(), `DELETE FROM events WHERE id=$1`, id)
	if err != nil {
		a.Log.Error("Failed to delete event", "error", err)
		http.Error(w, "Failed to delete event: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Если у события было изображение, удаляем его из MinIO
	if imagePath != nil && *imagePath != "" {
		err = a.Minio.RemoveObject(r.Context(), a.AppConfig.Minio.Bucket, *imagePath, minio.RemoveObjectOptions{})
		if err != nil {
			a.Log.Error("Failed to delete image from MinIO", "error", err, "event_id", id, "image_path", *imagePath)
			http.Error(w, "Failed to delete image from MinIO: "+err.Error(), http.StatusInternalServerError)
		} else {
			a.Log.Info("Successfully deleted image from MinIO", "event_id", id, "image_path", *imagePath)
		}
	}

	a.broadcast(map[string]interface{}{
		"type": "event_deleted",
		"data": map[string]interface{}{"id": id},
	})

	w.WriteHeader(http.StatusNoContent)
}

func (a *App) UpdateEvent(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		a.Log.Error("Invalid event id", "error", err)
		http.Error(w, "invalid event id", http.StatusBadRequest)
		return
	}

	var body struct {
		Completed bool `json:"completed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		a.Log.Error("Failed to decode body", "error", err)
		http.Error(w, "Failed to decode body", http.StatusBadRequest)
		return
	}

	_, err = a.DB.Exec(r.Context(), `
        UPDATE events SET used=$1 WHERE id=$2
    `, body.Completed, id)
	if err != nil {
		a.Log.Error("Failed to update event", "error", err)
		http.Error(w, "failed to update event: "+err.Error(), http.StatusInternalServerError)
		return
	}

	a.broadcast(map[string]interface{}{
		"type": "event_status_changed",
		"data": map[string]interface{}{
			"id":   id,
			"used": body.Completed,
		},
	})

	w.WriteHeader(http.StatusNoContent)
}

func (a *App) SwitchEvent(w http.ResponseWriter, r *http.Request) {
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		a.Log.Error("Failed to start transaction", "error", err)
		http.Error(w, "Failed to start transaction: "+err.Error(), http.StatusInternalServerError)
		return
	}

	defer func() {
		if err := tx.Rollback(r.Context()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			a.Log.Debug("Rollback error", "error", err)
		}
	}()

	// Получаем полную информацию о текущем ивенте
	var currentEvent models.Event
	err = tx.QueryRow(r.Context(),
		`SELECT id, title, description, type, image_path, used, current, init, created_at 
         FROM events WHERE current = true LIMIT 1`).
		Scan(&currentEvent.ID, &currentEvent.Title, &currentEvent.Description,
			&currentEvent.Type, &currentEvent.ImagePath, &currentEvent.Used,
			&currentEvent.Current, &currentEvent.Init, &currentEvent.CreatedAt)

	// Если есть текущий ивент, помечаем его как использованный
	if err == nil {
		_, err = tx.Exec(r.Context(),
			`UPDATE events SET used = true, current = false WHERE id = $1`,
			currentEvent.ID)
		if err != nil {
			a.Log.Error("Failed to mark current event as used", "error", err)
			http.Error(w, "Failed to mark current event as used: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Получаем счетчик событий
	var eventCounter int
	err = tx.QueryRow(r.Context(),
		`SELECT event_counter FROM game_state WHERE id = 1`).
		Scan(&eventCounter)
	if err != nil {
		a.Log.Error("Failed to get event counter", "error", err)
		http.Error(w, "Failed to get event counter: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Логика выбора следующего ивента
	nextCounter := eventCounter + 1
	isThirdEvent := nextCounter%3 == 0
	var nextEvent models.Event
	var found bool

	// Поиск init ивента
	var initEvent models.Event
	err = tx.QueryRow(r.Context(),
		`SELECT id, title, description, type, image_path, used, init, created_at 
         FROM events WHERE init = true AND used = false LIMIT 1`).
		Scan(&initEvent.ID, &initEvent.Title, &initEvent.Description,
			&initEvent.Type, &initEvent.ImagePath, &initEvent.Used,
			&initEvent.Init, &initEvent.CreatedAt)

	if err == nil {
		nextEvent = initEvent
		found = true
		// Для init ивентов не увеличиваем счетчик
		_, err = tx.Exec(r.Context(),
			`UPDATE game_state SET event_counter = event_counter WHERE id = 1`)
		if err != nil {
			a.Log.Error("Failed to update event counter", "error", err)
			http.Error(w, "Failed to update event counter: "+err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		// Логика для азартных игр и обычных ивентов
		if isThirdEvent {
			err = tx.QueryRow(r.Context(),
				`SELECT id, title, description, type, image_path, used, init, created_at 
                 FROM events WHERE type = 'Азартная игра' AND used = false 
                 ORDER BY RANDOM() LIMIT 1`).
				Scan(&nextEvent.ID, &nextEvent.Title, &nextEvent.Description,
					&nextEvent.Type, &nextEvent.ImagePath, &nextEvent.Used,
					&nextEvent.Init, &nextEvent.CreatedAt)
			if err == nil {
				found = true
				_, err = tx.Exec(r.Context(),
					`UPDATE game_state SET event_counter = 0 WHERE id = 1`)
				if err != nil {
					a.Log.Error("Failed to reset event counter", "error", err)
					http.Error(w, "Failed to reset event counter: "+err.Error(), http.StatusInternalServerError)
					return
				}
			}
		}

		if !found {
			err = tx.QueryRow(r.Context(),
				`SELECT id, title, description, type, image_path, used, init, created_at 
                 FROM events WHERE used = false AND type != 'Азартная игра' 
                 ORDER BY RANDOM() LIMIT 1`).
				Scan(&nextEvent.ID, &nextEvent.Title, &nextEvent.Description,
					&nextEvent.Type, &nextEvent.ImagePath, &nextEvent.Used,
					&nextEvent.Init, &nextEvent.CreatedAt)
			if err != nil {
				// КОЛОДА ЗАКОНЧИЛАСЬ - сбрасываем все события кроме начальных
				a.Log.Info("Event deck exhausted, resetting all non-init events")

				// Сбрасываем все события кроме init
				_, err = tx.Exec(r.Context(),
					`UPDATE events SET used = false, current = false WHERE init = false`)
				if err != nil {
					a.Log.Error("Failed to reset event deck", "error", err)
					http.Error(w, "Failed to reset event deck: "+err.Error(), http.StatusInternalServerError)
					return
				}

				// Сбрасываем счетчик событий
				_, err = tx.Exec(r.Context(),
					`UPDATE game_state SET event_counter = 0 WHERE id = 1`)
				if err != nil {
					a.Log.Error("Failed to reset event counter", "error", err)
					http.Error(w, "Failed to reset event counter: "+err.Error(), http.StatusInternalServerError)
					return
				}

				// Теперь снова пытаемся найти обычное событие
				err = tx.QueryRow(r.Context(),
					`SELECT id, title, description, type, image_path, used, init, created_at 
                     FROM events WHERE used = false AND type != 'Азартная игра' 
                     ORDER BY RANDOM() LIMIT 1`).
					Scan(&nextEvent.ID, &nextEvent.Title, &nextEvent.Description,
						&nextEvent.Type, &nextEvent.ImagePath, &nextEvent.Used,
						&nextEvent.Init, &nextEvent.CreatedAt)
				if err != nil {
					// Если после сброса все равно не нашли события
					tx.Rollback(r.Context())
					a.broadcast(map[string]interface{}{
						"type": "event_changed",
						"data": nil,
					})
					a.broadcast(map[string]interface{}{
						"type": "events_updated",
						"data": map[string]interface{}{
							"message": "No events available even after reset",
						},
					})
					a.Log.Error("No events available even after reset", "error", err)
					http.Error(w, "No events available", http.StatusNotFound)
					return
				}

				// Обновляем счетчик для первого события после сброса
				_, err = tx.Exec(r.Context(),
					`UPDATE game_state SET event_counter = 1 WHERE id = 1`)
				if err != nil {
					a.Log.Error("Failed to set event counter after reset", "error", err)
					http.Error(w, "Failed to set event counter after reset: "+err.Error(), http.StatusInternalServerError)
					return
				}
			} else {
				// Обновляем счетчик для обычных ивентов (если колода не закончилась)
				updateQuery := `UPDATE game_state SET event_counter = $1 WHERE id = 1`
				if isThirdEvent {
					_, err = tx.Exec(r.Context(), updateQuery, 0)
				} else {
					_, err = tx.Exec(r.Context(), updateQuery, nextCounter)
				}
				if err != nil {
					a.Log.Error("Failed to update event counter", "error", err)
					http.Error(w, "Failed to update event counter: "+err.Error(), http.StatusInternalServerError)
					return
				}
			}
		}
	}

	// Устанавливаем новый ивент как текущий
	_, err = tx.Exec(r.Context(),
		`UPDATE events SET current = true WHERE id = $1`,
		nextEvent.ID)
	if err != nil {
		a.Log.Error("Failed to set current event", "error", err)
		http.Error(w, "Failed to set current event: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Ключевое изменение: проверяем тип ПРЕДЫДУЩЕГО ивента
	shouldSkipPlayerSwitch := currentEvent.Init // Пропускаем смену игрока если предыдущий ивент был init

	if !shouldSkipPlayerSwitch {
		// ЛОГИКА ПЕРЕКЛЮЧЕНИЯ ИГРОКОВ (только если предыдущий ивент не был init)
		var currentPlayerTeamID int
		var currentPlayerID int
		err = tx.QueryRow(r.Context(),
			`UPDATE players SET has_moved = true, is_current = false 
             WHERE is_current = true RETURNING team_id, id`).
			Scan(&currentPlayerTeamID, &currentPlayerID)

		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			a.Log.Error("Failed to update current player", "error", err)
			http.Error(w, "Failed to update current player: "+err.Error(), http.StatusInternalServerError)
			return
		}

		var allMoved bool
		err = tx.QueryRow(r.Context(),
			`SELECT bool_and(has_moved) FROM players`).
			Scan(&allMoved)
		if err != nil {
			a.Log.Error("Failed to check players state", "error", err)
			http.Error(w, "Failed to check players state: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if allMoved {
			// Сброс ходов и увеличение раунда
			_, err = tx.Exec(r.Context(), `UPDATE players SET has_moved = false`)
			if err != nil {
				a.Log.Error("Failed to reset players", "error", err)
				http.Error(w, "Failed to reset players: "+err.Error(), http.StatusInternalServerError)
				return
			}

			_, err = tx.Exec(r.Context(),
				`UPDATE game_state SET current_round = current_round + 1 WHERE id = 1`)
			if err != nil {
				a.Log.Error("Failed to increment round", "error", err)
				http.Error(w, "Failed to increment round: "+err.Error(), http.StatusInternalServerError)
				return
			}

			// Выбираем случайного игрока из противоположной команды
			_, err = tx.Exec(r.Context(),
				`UPDATE players SET is_current = true 
                 WHERE id = (
                     SELECT id FROM players 
                     WHERE team_id != $1 
                     ORDER BY id LIMIT 1
                 )`, currentPlayerTeamID)
		} else {
			// Выбираем следующего игрока из противоположной команды
			_, err = tx.Exec(r.Context(),
				`UPDATE players SET is_current = true 
                 WHERE id = (
                     SELECT id FROM players 
                     WHERE team_id != $1 AND has_moved = false 
                     ORDER BY id LIMIT 1
                 )`, currentPlayerTeamID)
		}

		if err != nil {
			a.Log.Error("Failed to set next player", "error", err)
			http.Error(w, "Failed to set next player: "+err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		a.Log.Debug("Skipping player switch because previous event was init")
	}

	if err := tx.Commit(r.Context()); err != nil {
		a.Log.Error("Failed to commit transaction", "error", err)
		http.Error(w, "Failed to commit transaction: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Отправка broadcast сообщений
	a.broadcast(map[string]interface{}{
		"type": "event_changed",
		"data": nextEvent,
	})

	a.broadcast(map[string]interface{}{
		"type": "events_updated",
		"data": map[string]interface{}{
			"message":   "Event changed",
			"new_event": nextEvent,
		},
	})

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(nextEvent); err != nil {
		a.Log.Error("Failed to encode event", "error", err)
		http.Error(w, "Failed to encode event: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

func (a *App) UploadEventImage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		a.Log.Error("Invalid event id", "error", err)
		http.Error(w, "Invalid event id", http.StatusBadRequest)
		return
	}

	file, handler, err := r.FormFile("image")
	if err != nil {
		a.Log.Error("Failed to get image", "error", err)
		http.Error(w, "Failed to get image: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer func(file multipart.File) {
		err := file.Close()
		if err != nil {
			a.Log.Error("Failed to close file", "error", err)
			http.Error(w, "Failed to close file: "+err.Error(), http.StatusInternalServerError)
		}
	}(file)

	// Генерируем уникальное имя файла
	fileName := fmt.Sprintf("event_%d_%d_%s", id, time.Now().Unix(), handler.Filename)

	// Загружаем в MinIO
	_, err = a.Minio.PutObject(r.Context(), a.AppConfig.Minio.Bucket, fileName, file, handler.Size, minio.PutObjectOptions{ContentType: handler.Header.Get("Content-Type")})
	if err != nil {
		a.Log.Error("Failed to upload image to MinIO", "error", err)
		http.Error(w, "Failed to upload image to MinIO: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Обновляем запись в PostgreSQL с путём к изображению
	_, err = a.DB.Exec(r.Context(), "UPDATE events SET image_path = $1 WHERE id = $2", fileName, id)
	if err != nil {
		a.Log.Error("Failed to update event with image path", "error", err)
		http.Error(w, "Failed to update event with image path: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(map[string]string{
		"message": "Image uploaded successfully",
		"path":    fileName,
	})
	if err != nil {
		a.Log.Error("Failed to encode event", "error", err)
		http.Error(w, "Failed to encode event: "+err.Error(), http.StatusInternalServerError)
		return
	}
}
