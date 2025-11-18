package api

import (
	"log/slog"
	"net/http"
	"red-horse-tavern/internal/utils"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type App struct {
	DB        *pgxpool.Pool
	Minio     *minio.Client
	Log       *slog.Logger
	clients   map[*websocket.Conn]bool
	clientsMu sync.RWMutex
	AppConfig *utils.AppConfig
}

func NewApp(db *pgxpool.Pool, minioClient *minio.Client, appConfig *utils.AppConfig, log *slog.Logger) *App {
	return &App{
		DB:        db,
		Minio:     minioClient,
		AppConfig: appConfig,
		Log:       log,
		clients:   make(map[*websocket.Conn]bool),
	}
}

func (a *App) broadcast(message map[string]interface{}) {
	a.clientsMu.RLock()
	defer a.clientsMu.RUnlock()

	for client := range a.clients {
		err := client.WriteJSON(message)
		if err != nil {
			a.Log.Error("WebSocket write error", "error", err)
			err := client.Close()
			if err != nil {
				return
			}
			delete(a.clients, client)
		}
	}
}
