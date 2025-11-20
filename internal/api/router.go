package api

import (
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

	// Сбрасываем события - все непройденные, снимаем текущее
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
	// Находим текущее активное событие
	var currentEventID int
	err := a.DB.QueryRow(r.Context(), `
        SELECT id FROM events WHERE current = true LIMIT 1
    `).Scan(&currentEventID)

	if err == nil {
		// Помечаем текущее событие как использованное и снимаем флаг текущего
		_, err = a.DB.Exec(r.Context(), `
            UPDATE events SET used = true, current = false WHERE id = $1
        `, currentEventID)
		if err != nil {
			http.Error(w, "Failed to mark current event as used: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Сначала проверяем, есть ли неиспользованное событие с init = true
	var initEvent models.Event
	err = a.DB.QueryRow(r.Context(), `
        SELECT id, title, description, image_path, used, created_at
        FROM events 
        WHERE init = true AND used = false
        LIMIT 1
    `).Scan(&initEvent.ID, &initEvent.Title, &initEvent.Description, &initEvent.ImagePath, &initEvent.Used, &initEvent.CreatedAt)

	var nextEvent models.Event

	if err == nil {
		// Если нашли неиспользованное init событие - используем его
		nextEvent = initEvent
	} else {
		// Если нет неиспользованного init события, ищем случайное непройденное событие
		err = a.DB.QueryRow(r.Context(), `
            SELECT id, title, description, image_path, used, created_at
            FROM events 
            WHERE used = false 
            ORDER BY RANDOM() 
            LIMIT 1
        `).Scan(&nextEvent.ID, &nextEvent.Title, &nextEvent.Description, &nextEvent.ImagePath, &nextEvent.Used, &nextEvent.CreatedAt)

		if err != nil {
			// Если нет доступных событий, отправляем broadcast о смене
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
	}

	// Помечаем выбранное событие как текущее
	_, err = a.DB.Exec(r.Context(), `UPDATE events SET current = true WHERE id = $1`, nextEvent.ID)
	if err != nil {
		http.Error(w, "Failed to set current event: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Получаем обновленные данные события
	err = a.DB.QueryRow(r.Context(), `
        SELECT id, title, description, image_path, used, current, created_at
        FROM events WHERE id = $1
    `, nextEvent.ID).Scan(&nextEvent.ID, &nextEvent.Title, &nextEvent.Description, &nextEvent.ImagePath, &nextEvent.Used, &nextEvent.Current, &nextEvent.CreatedAt)

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
