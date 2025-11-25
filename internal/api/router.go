package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"red-horse-tavern/internal/models"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/minio/minio-go/v7"
	slogchi "github.com/samber/slog-chi"
)

// ==================== МАРШРУТИЗАЦИЯ ====================

func (a *App) NewRouter() *chi.Mux {
	r := chi.NewRouter()

	r.Use(slogchi.New(a.Log))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/admin", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/admin.html")
	})
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/view.html")
	})

	r.Get("/static/events/{filename}", a.ServeEventImage)

	fs := http.FileServer(http.Dir("./static"))
	r.Handle("/static/*", http.StripPrefix("/static/", fs))

	r.Route("/api/admin/teams", func(r chi.Router) {
		r.Get("/", a.GetTeamsAdmin)
		r.Post("/create", a.CreateTeam)
		r.Post("/reset", a.ResetTeams)
		r.Put("/{id}/name", a.UpdateTeamName)
		r.Put("/{id}/health", a.UpdateTeamHealth)
		r.Put("/{id}/drunk", a.UpdateTeamDrunk)
	})

	r.Route("/api/admin/events", func(r chi.Router) {
		r.Get("/", a.GetEventsAdmin)
		r.Post("/", a.CreateEvent)
		r.Post("/{id}/image", a.UploadEventImage)
		r.Put("/{id}/use", a.MarkEventUsed)
		r.Put("/{id}/status", a.UpdateEventStatus)
		r.Put("/{id}", a.UpdateEvent)
		r.Post("/next", a.NextEvent)
		r.Delete("/{id}", a.DeleteEvent)
	})

	r.Route("/api/admin/players", func(r chi.Router) {
		r.Get("/", a.GetPlayers)
		r.Post("/", a.CreatePlayer)
		r.Put("/{id}/move", a.MarkPlayerMoved)
		r.Put("/{id}/reset", a.ResetPlayerMove)
		r.Delete("/{id}", a.DeletePlayer)
	})

	r.Route("/api/admin/game", func(r chi.Router) {
		r.Get("/", a.GetGameState)
		r.Post("/next_round", a.NextRound)
		r.Post("/reset_rounds", a.ResetRounds)
		r.Post("/check_round", a.CheckRoundProgress)
	})

	r.Route("/viewer", func(r chi.Router) {
		r.Get("/teams", a.GetTeamsViewer)
		r.Get("/events", a.GetEventsViewer)
		r.Get("/event", a.GetCurrentEvent)
		r.Get("/ws", a.EventWebSocket)
	})

	return r
}

// ==================== СТАТИЧЕСКИЕ ФАЙЛЫ ====================

func (a *App) ServeEventImage(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")

	a.Log.Info("ServeEventImage called", "filename", filename, "bucket", a.AppConfig.Minio.Bucket, "url", r.URL.String())

	// Проверяем существование объекта
	_, err := a.Minio.StatObject(r.Context(), a.AppConfig.Minio.Bucket, filename, minio.StatObjectOptions{})
	if err != nil {
		a.Log.Error("File not found in MinIO", "error", err, "filename", filename, "bucket", a.AppConfig.Minio.Bucket)
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	object, err := a.Minio.GetObject(r.Context(), a.AppConfig.Minio.Bucket, filename, minio.GetObjectOptions{})
	if err != nil {
		a.Log.Error("Failed to get object from MinIO", "error", err, "filename", filename)
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	defer object.Close()

	// Получаем информацию о файле для Content-Type
	stat, err := object.Stat()
	if err != nil {
		a.Log.Error("Failed to get object stats", "error", err, "filename", filename)
		http.Error(w, "Failed to get file info", http.StatusInternalServerError)
		return
	}

	// Устанавливаем правильный Content-Type
	w.Header().Set("Content-Type", stat.ContentType)
	w.Header().Set("Content-Type", stat.ContentType)
	w.Header().Set("Cache-Control", "public, max-age=31536000")
	w.Header().Set("Expires", time.Now().Add(365*24*time.Hour).Format(http.TimeFormat))
	w.Header().Set("ETag", fmt.Sprintf("\"%s\"", stat.ETag))

	// Копируем объект из MinIO в ответ
	_, err = io.Copy(w, object)
	if err != nil {
		a.Log.Error("Failed to copy object to response", "error", err, "filename", filename)
		http.Error(w, "Failed to serve image", http.StatusInternalServerError)
		return
	}

	a.Log.Info("Successfully served event image", "filename", filename, "content_type", stat.ContentType, "size", stat.Size)
}

// ==================== WEBSOCKET ====================

func (a *App) EventWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		a.Log.Error("WebSocket upgrade failed", "error", err)
		return
	}
	defer func(conn *websocket.Conn) {
		err := conn.Close()
		if err != nil {
			slog.Error(err.Error())
		}
	}(conn)

	a.clientsMu.Lock()
	a.clients[conn] = true
	a.clientsMu.Unlock()

	a.Log.Info("WebSocket client connected", "remote", r.RemoteAddr)

	go a.sendInitialState(conn)

	for {
		messageType, _, err := conn.ReadMessage()
		if err != nil || messageType == websocket.CloseMessage {
			break
		}
	}

	a.clientsMu.Lock()
	delete(a.clients, conn)
	a.clientsMu.Unlock()
	a.Log.Info("WebSocket client disconnected", "remote", r.RemoteAddr)
}

func (a *App) sendInitialState(conn *websocket.Conn) {
	err := conn.WriteJSON(map[string]string{
		"type":    "connected",
		"message": "WebSocket connected successfully",
	})
	if err != nil {
		slog.Error(err.Error())
		return
	}
}

// ==================== КОМАНДЫ - ADMIN ====================

func (a *App) GetTeamsAdmin(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.Query(r.Context(), `
		SELECT id, name, health, drunk, updated_at
		FROM teams
		ORDER BY id
	`)
	if err != nil {
		http.Error(w, "Failed to query teams: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var teams []models.Team
	for rows.Next() {
		var t models.Team
		if err := rows.Scan(&t.ID, &t.Name, &t.Health, &t.Drunk, &t.UpdatedAt); err != nil {
			http.Error(w, "Failed to scan team: "+err.Error(), http.StatusInternalServerError)
			return
		}
		teams = append(teams, t)
	}

	if err := rows.Err(); err != nil {
		http.Error(w, "Error iterating teams: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(teams); err != nil {
		slog.Error("Failed to encode teams to JSON", "error", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

func (a *App) CreateTeam(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	_, err := a.DB.Exec(r.Context(), `
		INSERT INTO teams (name, health, drunk, updated_at)
		VALUES ($1, 40, 0, NOW())
	`, body.Name)
	if err != nil {
		http.Error(w, "failed to create team: "+err.Error(), http.StatusInternalServerError)
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
		http.Error(w, "invalid team id", http.StatusBadRequest)
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		http.Error(w, "invalid request body", http.StatusBadRequest)
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
			"message": "Ходы игроков сброшены",
		},
	})

	// Отправляем отдельные broadcast для команд и событий
	a.broadcast(map[string]interface{}{
		"type": "teams_reset",
		"data": map[string]interface{}{
			"message": "Все команды сброшены",
		},
	})

	a.broadcast(map[string]interface{}{
		"type": "events_reset",
		"data": map[string]interface{}{
			"message":           "Все события сброшены",
			"new_current_event": currentEventData,
		},
	})

	// И общий broadcast для полного обновления
	a.broadcast(map[string]interface{}{
		"type": "world_reset",
		"data": map[string]interface{}{
			"message":           "Мир перерождён",
			"teams_reset":       true,
			"events_reset":      true,
			"new_current_event": currentEventData,
		},
	})

	w.WriteHeader(http.StatusNoContent)
}

// ==================== СОБЫТИЯ - ADMIN ====================

func (a *App) GetEventsAdmin(w http.ResponseWriter, r *http.Request) {
	// Сначала проверяем, есть ли неиспользованный инициализирующий ивент
	var hasUnusedInitEvent bool
	err := a.DB.QueryRow(r.Context(), `
        SELECT EXISTS(
            SELECT 1 FROM events 
            WHERE init = true AND used = false
        )
    `).Scan(&hasUnusedInitEvent)
	if err != nil {
		http.Error(w, "Failed to check init events: "+err.Error(), http.StatusInternalServerError)
		return
	}

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
		http.Error(w, "Failed to query events: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var events []models.Event
	for rows.Next() {
		var e models.Event
		if err := rows.Scan(&e.ID, &e.Title, &e.Description, &e.Type, &e.Difficult, &e.VictoryEffect, &e.DefeatEffect, &e.Requirement, &e.ImagePath, &e.Current, &e.Used, &e.Init, &e.CreatedAt); err != nil {
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
	json.NewEncoder(w).Encode(events)
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
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var id int
	err := a.DB.QueryRow(r.Context(), `INSERT INTO events (title, description, type, difficult, 
                    victory_effect, defeat_effect, requirement, current,
                    used, init, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, false, false, $8, NOW()) RETURNING id`, body.Title, body.Description, body.Type, body.Difficult, body.VictoryEffect, body.DefeatEffect, body.Requirement, body.Init).Scan(&id)

	if err != nil {
		http.Error(w, "failed to create event: "+err.Error(), http.StatusInternalServerError)
		return
	}

	a.broadcast(map[string]interface{}{
		"type": "event_created",
		"data": map[string]interface{}{"id": id, "title": body.Title},
	})

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]int{"id": id})
}

func (a *App) UploadEventImage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid event id", http.StatusBadRequest)
		return
	}

	file, handler, err := r.FormFile("image")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Генерируем уникальное имя файла
	fileName := fmt.Sprintf("event_%d_%d_%s", id, time.Now().Unix(), handler.Filename)

	// Загружаем в MinIO
	_, err = a.Minio.PutObject(r.Context(), a.AppConfig.Minio.Bucket, fileName, file, handler.Size, minio.PutObjectOptions{ContentType: handler.Header.Get("Content-Type")})
	if err != nil {
		http.Error(w, "Failed to upload image to MinIO: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Обновляем запись в PostgreSQL с путём к изображению
	_, err = a.DB.Exec(r.Context(), "UPDATE events SET image_path = $1 WHERE id = $2", fileName, id)
	if err != nil {
		http.Error(w, "Failed to update event with image path: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Image uploaded successfully",
		"path":    fileName,
	})
}

func (a *App) DeleteEvent(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid event id", http.StatusBadRequest)
		return
	}

	// Сначала получаем информацию о событии, чтобы узнать путь к изображению
	var imagePath *string
	err = a.DB.QueryRow(r.Context(), "SELECT image_path FROM events WHERE id = $1", id).Scan(&imagePath)
	if err != nil {
		http.Error(w, "failed to find event: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Удаляем событие из базы данных
	_, err = a.DB.Exec(r.Context(), `DELETE FROM events WHERE id=$1`, id)
	if err != nil {
		http.Error(w, "failed to delete event: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Если у события было изображение, удаляем его из MinIO
	if imagePath != nil && *imagePath != "" {
		err = a.Minio.RemoveObject(r.Context(), a.AppConfig.Minio.Bucket, *imagePath, minio.RemoveObjectOptions{})
		if err != nil {
			// Логируем ошибку, но не прерываем выполнение - событие уже удалено из БД
			a.Log.Error("Failed to delete image from MinIO", "error", err, "event_id", id, "image_path", *imagePath)
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

func (a *App) MarkEventUsed(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid event id", http.StatusBadRequest)
		return
	}

	_, err = a.DB.Exec(r.Context(), `UPDATE events SET used=true WHERE id=$1`, id)
	if err != nil {
		http.Error(w, "failed to mark event as used: "+err.Error(), http.StatusInternalServerError)
		return
	}

	a.broadcast(map[string]interface{}{
		"type": "event_status_changed",
		"data": map[string]interface{}{
			"id":   id,
			"used": true,
		},
	})

	w.WriteHeader(http.StatusNoContent)
}

func (a *App) UpdateEvent(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		a.Log.Error("Failed to update event", "error", err, "id", idStr)
		http.Error(w, "invalid event id", http.StatusBadRequest)
		return
	}

	var body struct {
		Title         string `json:"title"`
		Description   string `json:"description"`
		Type          string `json:"type"`
		Difficult     string `json:"difficult"`
		VictoryEffect string `json:"victory_effect"`
		DefeatEffect  string `json:"defeat_effect"`
		Requirement   string `json:"requirement"`
		ImagePath     string `json:"image_path"`
		Init          *bool  `json:"init,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		a.Log.Error("Failed to decode update event body", "error", err, "id", id)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Проверяем существование события перед обновлением
	var exists bool
	err = a.DB.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM events WHERE id = $1)", id).Scan(&exists)
	if err != nil {
		a.Log.Error("Failed to check event existence", "error", err, "id", id)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	if !exists {
		a.Log.Error("Event not found", "id", id)
		http.Error(w, "event not found", http.StatusNotFound)
		return
	}

	// Выполняем обновление
	result, err := a.DB.Exec(r.Context(), `UPDATE events 
        SET title=$2, description=$3, type=$4, difficult=$5, victory_effect=$6, defeat_effect=$7, 
            requirement=$8, image_path=$9, init=$10
        WHERE id = $1`,
		id, body.Title, body.Description, body.Type, body.Difficult, body.VictoryEffect,
		body.DefeatEffect, body.Requirement, body.ImagePath, body.Init)

	if err != nil {
		a.Log.Error("Failed to update event in database", "error", err, "id", id)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	// Проверяем, что запись действительно обновилась
	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		a.Log.Error("No rows affected when updating event", "id", id)
		http.Error(w, "event not found or no changes made", http.StatusNotFound)
		return
	}

	// Отправляем уведомления об изменении
	a.broadcast(map[string]interface{}{
		"type": "event_changed",
		"data": map[string]interface{}{
			"id": id,
		},
	})

	a.broadcast(map[string]interface{}{
		"type": "events_updated",
		"data": map[string]interface{}{
			"message": "Event updated successfully",
		},
	})

	w.WriteHeader(http.StatusNoContent)
}

func (a *App) UpdateEventStatus(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid event id", http.StatusBadRequest)
		return
	}

	var body struct {
		Completed bool `json:"completed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	_, err = a.DB.Exec(r.Context(), `
        UPDATE events SET used=$1 WHERE id=$2
    `, body.Completed, id)
	if err != nil {
		http.Error(w, "failed to update event status: "+err.Error(), http.StatusInternalServerError)
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

func (a *App) NextEvent(w http.ResponseWriter, r *http.Request) {
	// Начинаем транзакцию для обеспечения целостности данных
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		http.Error(w, "Failed to start transaction: "+err.Error(), http.StatusInternalServerError)
		return
	}

	defer func(tx pgx.Tx, ctx context.Context) {
		err := tx.Rollback(ctx)
		if err != nil {
			slog.Error("Failed to rollback transaction", "error", err)
			return
		}
	}(tx, r.Context())

	// Находим текущее активное событие
	var currentEventID int
	err = tx.QueryRow(r.Context(), `
        SELECT id 
        FROM events 
        WHERE current = true 
        LIMIT 1
    `).Scan(&currentEventID)

	if err == nil {
		// Помечаем текущее событие как использованное и снимаем флаг текущего
		_, err = tx.Exec(r.Context(), `
            UPDATE events
            SET used = true, current = false
            WHERE id = $1
        `, currentEventID)
		if err != nil {
			http.Error(w, "Failed to mark current event as used: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Получаем счетчик событий
	var eventCounter int
	err = tx.QueryRow(r.Context(), `
        SELECT event_counter 
        FROM game_state 
        WHERE id = 1
    `).Scan(&eventCounter)
	if err != nil {
		http.Error(w, "Failed to get event counter: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Увеличиваем счетчик (если это не init событие)
	nextCounter := eventCounter + 1
	isThirdEvent := nextCounter%3 == 0

	var nextEvent models.Event
	var found bool

	// Сначала проверяем, есть ли неиспользованное событие с init = true
	var initEvent models.Event
	err = tx.QueryRow(r.Context(), `
        SELECT id, title, description, type, image_path, used, created_at
        FROM events 
        WHERE init = true AND used = false
        LIMIT 1
    `).Scan(
		&initEvent.ID,
		&initEvent.Title,
		&initEvent.Description,
		&initEvent.Type,
		&initEvent.ImagePath,
		&initEvent.Used,
		&initEvent.CreatedAt)

	if err == nil {
		// Если нашли неиспользованное init событие - используем его
		// Для init событий не увеличиваем счетчик
		nextEvent = initEvent
		found = true
		_, err = tx.Exec(r.Context(), `
            UPDATE game_state 
            SET event_counter = event_counter 
            WHERE id = 1
        `) // Не меняем счетчик для init событий
		if err != nil {
			http.Error(w, "Failed to update event counter: "+err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		// Если это третье событие - ищем азартную игру
		if isThirdEvent {
			err = tx.QueryRow(r.Context(), `
                SELECT id, title, description, type, image_path, used, created_at
                FROM events 
                WHERE type = 'Азартная игра' AND used = false 
                ORDER BY RANDOM() 
                LIMIT 1
            `).Scan(
				&nextEvent.ID,
				&nextEvent.Title,
				&nextEvent.Description,
				&nextEvent.Type,
				&nextEvent.ImagePath,
				&nextEvent.Used,
				&nextEvent.CreatedAt)

			if err == nil {
				found = true
				// Сбрасываем счетчик после азартной игры
				_, err = tx.Exec(r.Context(), `
                    UPDATE game_state SET event_counter = 0 WHERE id = 1
                `)
				if err != nil {
					http.Error(w, "Failed to reset event counter: "+err.Error(), http.StatusInternalServerError)
					return
				}
			}
		}

		// Если не нашли азартную игру (или это не третье событие), ищем любое событие
		if !found {
			err = tx.QueryRow(r.Context(), `
                SELECT id, title, description, type, image_path, used, created_at
                FROM events 
                WHERE used = false AND type != 'Азартная игра'
                ORDER BY RANDOM() 
                LIMIT 1
            `).Scan(
				&nextEvent.ID,
				&nextEvent.Title,
				&nextEvent.Description,
				&nextEvent.Type,
				&nextEvent.ImagePath,
				&nextEvent.Used,
				&nextEvent.CreatedAt)

			if err != nil {
				// Если нет доступных событий, откатываем транзакцию и отправляем broadcast
				tx.Rollback(r.Context())
				a.broadcast(map[string]interface{}{
					"type": "event_changed",
					"data": nil,
				})
				a.broadcast(map[string]interface{}{
					"type": "events_updated",
					"data": map[string]interface{}{
						"message": "No more events available",
					},
				})
				http.Error(w, "No more events available", http.StatusNotFound)
				return
			}

			// Обновляем счетчик для обычных событий
			updateQuery := `
				UPDATE game_state 
				SET event_counter = $1 
				WHERE id = 1`

			if isThirdEvent {
				// Если это должно было быть третье событие, но азартной игры не нашлось,
				// все равно сбрасываем счетчик
				_, err = tx.Exec(r.Context(), updateQuery, 0)
			} else {
				_, err = tx.Exec(r.Context(), updateQuery, nextCounter)
			}
			if err != nil {
				http.Error(w, "Failed to update event counter: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}

	// Помечаем выбранное событие как текущее
	_, err = tx.Exec(r.Context(), `
		UPDATE events 
		SET current = true 
		WHERE id = $1
	`, nextEvent.ID)
	if err != nil {
		http.Error(w, "Failed to set current event: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Получаем обновленные данные события
	err = tx.QueryRow(r.Context(), `
        SELECT id, title, description, type, image_path, used, current, created_at
        FROM events 
        WHERE id = $1
    `, nextEvent.ID).Scan(
		&nextEvent.ID,
		&nextEvent.Title,
		&nextEvent.Description,
		&nextEvent.Type,
		&nextEvent.ImagePath,
		&nextEvent.Used,
		&nextEvent.Current,
		&nextEvent.CreatedAt)

	// ПЕРЕКЛЮЧЕНИЕ РАУНДОВ
	// Сначала получаем ID текущего игрока и обновляем его состояние
	var currentPlayerTeamID int
	var currentPlayerID int
	err = tx.QueryRow(r.Context(), `
		UPDATE players 
		SET has_moved = true, is_current = false 
		WHERE is_current = true 
		RETURNING team_id, id
	`).Scan(
		&currentPlayerTeamID,
		&currentPlayerID)

	if err != nil && err != pgx.ErrNoRows {
		http.Error(w, "Failed to update current player: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Проверяем все ли игроки сходили после обновления текущего
	var allMoved bool
	err = tx.QueryRow(r.Context(), `
		SELECT bool_and(has_moved) 
		FROM players`).Scan(
		&allMoved)
	if err != nil {
		http.Error(w, "Failed to check players state: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if allMoved {
		// Сбрасываем has_moved у всех игроков
		_, err = tx.Exec(r.Context(), `
			UPDATE players
			SET has_moved = false`)
		if err != nil {
			http.Error(w, "Failed to reset players: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Увеличиваем current_round
		_, err = tx.Exec(r.Context(), `
			UPDATE game_state 
			SET current_round = current_round + 1 
			WHERE id = 1`)
		if err != nil {
			http.Error(w, "Failed to increment round: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Выбираем случайного игрока из противоположной команды как нового текущего
		_, err = tx.Exec(r.Context(), `
			UPDATE players
			SET is_current = true
			WHERE id = (
				SELECT id
				FROM players
				WHERE team_id != $1
				ORDER BY RANDOM()
				LIMIT 1
			)
		`, currentPlayerTeamID)
		if err != nil {
			http.Error(w, "Failed to set new current player: "+err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		// Выбираем следующего игрока из противоположной команды который еще не ходил
		_, err = tx.Exec(r.Context(), `
			UPDATE players
			SET is_current = true
			WHERE id = (
				SELECT id
				FROM players
				WHERE team_id != $1 AND has_moved = false
				ORDER BY RANDOM()
				LIMIT 1
			)
		`, currentPlayerTeamID)
		if err != nil {
			http.Error(w, "Failed to set next player: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Коммитим транзакцию
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "Failed to commit transaction: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Отправляем broadcast о смене события
	a.broadcast(map[string]interface{}{
		"type": "event_changed",
		"data": nextEvent,
	})

	// И общий broadcast об обновлении событий
	a.broadcast(map[string]interface{}{
		"type": "events_updated",
		"data": map[string]interface{}{
			"message":   "Event changed",
			"new_event": nextEvent,
		},
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nextEvent)
}

// ==================== PLAYER ENDPOINTS ====================

func (a *App) GetPlayers(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.Query(r.Context(), `
		SELECT id, name, team_id, has_moved, is_current, created_at
		FROM players
		ORDER BY id
	`)
	if err != nil {
		http.Error(w, "Failed to query players: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var players []models.Player
	for rows.Next() {
		var p models.Player
		if err := rows.Scan(&p.ID, &p.Name, &p.TeamID, &p.HasMoved, &p.IsCurrent, &p.CreatedAt); err != nil {
			http.Error(w, "Failed to scan player: "+err.Error(), http.StatusInternalServerError)
			return
		}
		players = append(players, p)
	}

	json.NewEncoder(w).Encode(players)
}

func (a *App) CreatePlayer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name   string `json:"name"`
		TeamID int    `json:"team_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	_, err := a.DB.Exec(r.Context(),
		`INSERT INTO players (name, team_id) VALUES ($1, $2)`,
		body.Name, body.TeamID,
	)
	if err != nil {
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
		http.Error(w, "Invalid player ID", http.StatusBadRequest)
		return
	}

	_, err = a.DB.Exec(r.Context(), `
		UPDATE players SET has_moved = true WHERE id = $1
	`, id)
	if err != nil {
		http.Error(w, "Failed to update player: "+err.Error(), http.StatusInternalServerError)
		return
	}

	a.broadcast(map[string]interface{}{"type": "player_moved"})
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) ResetPlayerMove(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid player ID", http.StatusBadRequest)
		return
	}

	_, err = a.DB.Exec(r.Context(),
		`UPDATE players SET has_moved = false WHERE id=$1`,
		id,
	)
	if err != nil {
		http.Error(w, "Failed to reset move: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *App) DeletePlayer(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	_, err = a.DB.Exec(r.Context(), `DELETE FROM players WHERE id=$1`, id)
	if err != nil {
		http.Error(w, "Failed to delete player: "+err.Error(), http.StatusInternalServerError)
		return
	}

	a.broadcast(map[string]interface{}{
		"type": "player_deleted",
	})
	w.WriteHeader(http.StatusNoContent)
}

// ==================== GAME STATS ==========================

func (a *App) GetGameState(w http.ResponseWriter, r *http.Request) {
	var gs models.GameState

	err := a.DB.QueryRow(r.Context(),
		`SELECT id, current_round FROM game_state LIMIT 1`,
	).Scan(&gs.ID, &gs.CurrentRound)

	if err != nil {
		http.Error(w, "Failed to load game state: "+err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(gs)
}

func (a *App) NextRound(w http.ResponseWriter, r *http.Request) {
	_, err := a.DB.Exec(r.Context(), `
		UPDATE game_state 
		SET current_round = current_round + 1
	`)
	if err != nil {
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
		http.Error(w, "Failed to count players: "+err.Error(), http.StatusInternalServerError)
		return
	}

	err = a.DB.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM players WHERE has_moved = true`,
	).Scan(&moved)
	if err != nil {
		http.Error(w, "Failed to count moves: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if total > 0 && total == moved {
		// Все игроки сходили → следующий раунд
		_, err = a.DB.Exec(r.Context(),
			`UPDATE game_state SET current_round = current_round + 1`,
		)
		if err != nil {
			http.Error(w, "Failed to next round: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Сбрасываем has_moved всем игрокам
		_, _ = a.DB.Exec(r.Context(), `UPDATE players SET has_moved = false`)

		a.broadcast(map[string]interface{}{
			"type": "round_auto_advanced",
		})
	}

	json.NewEncoder(w).Encode(map[string]int{
		"total": total,
		"moved": moved,
	})
}

// ==================== VIEWER ENDPOINTS ====================

func (a *App) GetTeamsViewer(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.Query(r.Context(), `
        SELECT id, name, health, drunk, updated_at
        FROM teams
        ORDER BY id
    `)
	if err != nil {
		http.Error(w, "Failed to query teams: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var teams []models.Team
	for rows.Next() {
		var t models.Team
		if err := rows.Scan(&t.ID, &t.Name, &t.Health, &t.Drunk, &t.UpdatedAt); err != nil {
			http.Error(w, "Failed to scan team: "+err.Error(), http.StatusInternalServerError)
			return
		}
		teams = append(teams, t)
	}

	if err := rows.Err(); err != nil {
		http.Error(w, "Error iterating teams: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(teams); err != nil {
		slog.Error("Failed to encode teams to JSON", "error", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

func (a *App) GetEventsViewer(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.Query(r.Context(), `
        SELECT id, title, description, type, difficult, victory_effect, defeat_effect, requirement, image_path, current, used, init, created_at
        FROM events
        ORDER BY created_at DESC
    `)
	if err != nil {
		http.Error(w, "Failed to query events: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var events []models.Event
	for rows.Next() {
		var e models.Event
		if err := rows.Scan(&e.ID, &e.Title, &e.Description, &e.Type, &e.Difficult, &e.VictoryEffect, &e.DefeatEffect, &e.Requirement, &e.ImagePath, &e.Current, &e.Used, &e.Init, &e.CreatedAt); err != nil {
			a.Log.Error("Failed to scan event: " + err.Error())
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
		a.Log.Error("Failed to encode events to JSON", "error", err)
		return
	}
}

func (a *App) GetCurrentEvent(w http.ResponseWriter, r *http.Request) {
	var e models.Event

	err := a.DB.QueryRow(r.Context(), `
        SELECT id, title, type, description, image_path, used, created_at
        FROM events 
        WHERE current = true
        LIMIT 1
    `).Scan(&e.ID, &e.Title, &e.Type, &e.Description, &e.ImagePath, &e.Used, &e.CreatedAt)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(nil)
		if err != nil {
			a.Log.Error("Failed to encode current event to JSON: "+err.Error(), "error", err)
			return
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(e)
	if err != nil {
		a.Log.Error("Failed to encode current event to JSON: "+err.Error(), "error", err)
		return
	}
}
