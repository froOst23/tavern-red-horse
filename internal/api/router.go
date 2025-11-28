package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"red-horse-tavern/internal/models"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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

	r.Route("/api/events", func(r chi.Router) {
		r.Get("/", a.GetEvent)
		r.Post("/", a.CreateEvent)
		r.Post("/next", a.SwitchEvent)
		r.Post("/{id}/image", a.UploadEventImage)
		r.Put("/{id}/status", a.UpdateEvent)
		r.Delete("/{id}", a.DeleteEvent)
	})

	r.Route("/api/players", func(r chi.Router) {
		r.Get("/", a.GetPlayers)
		r.Post("/", a.CreatePlayer)
		r.Put("/{id}/reset", a.ResetPlayerMove)
		r.Put("/{id}/move", a.MarkPlayerMoved)
		r.Put("/{id}/skip", a.SkipPlayerTurn)
		r.Delete("/{id}", a.DeletePlayer)
		r.Post("/turn-order", a.SetPlayerTurnOrder)
	})

	r.Route("/api/teams", func(r chi.Router) {
		r.Get("/", a.GetTeamsAdmin)
		r.Post("/create", a.CreateTeam)
		r.Post("/reset", a.ResetTeams)
		r.Put("/{id}/name", a.UpdateTeamName)
		r.Put("/{id}/health", a.UpdateTeamHealth)
		r.Put("/{id}/drunk", a.UpdateTeamDrunk)
	})

	r.Route("/api/game", func(r chi.Router) {
		r.Get("/", a.GetGameState)
		r.Post("/next_round", a.NextRound)
		r.Post("/reset_rounds", a.ResetRounds)
		r.Post("/check_round", a.CheckRoundProgress)
		r.Post("/arrange_turns", a.AutoArrangeTurnOrder)
	})

	r.Route("/viewer", func(r chi.Router) {
		r.Get("/teams", a.GetTeamsViewer)
		r.Get("/events", a.GetEventsViewer)
		r.Get("/event", a.GetCurrentEvent)
		r.Get("/ws", a.EventWebSocket)
	})

	return r
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
