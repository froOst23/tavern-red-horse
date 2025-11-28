package api

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
)

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
