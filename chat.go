package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type message struct {
	Handle string `json:"handle"`
	Text   string `json:"text"`
}

func validateMessage(data []byte) (message, error) {
	var msg message
	if err := json.Unmarshal(data, &msg); err != nil {
		return msg, fmt.Errorf("unmarshaling message: %w", err)
	}
	if msg.Handle == "" && msg.Text == "" {
		return msg, fmt.Errorf("message has no handle or text")
	}
	return msg, nil
}

func handleWebSocket(hub *Hub, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			logger.Error("websocket upgrade failed", "err", err)
			return
		}
		defer ws.Close()

		hub.Register(ws)
		defer hub.Deregister(ws)

		for {
			mt, data, err := ws.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) || err == io.EOF {
					logger.Info("websocket closed")
				} else {
					logger.Error("error reading websocket message", "err", err)
				}
				return
			}
			if mt == websocket.TextMessage {
				if _, err := validateMessage(data); err != nil {
					logger.Error("invalid message", "err", err)
					continue
				}
				if err := hub.Publish(r.Context(), data); err != nil {
					logger.Error("failed to publish message", "err", err)
				}
			}
		}
	}
}
