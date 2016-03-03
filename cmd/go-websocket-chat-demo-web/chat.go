package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/websocket"
)

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
)

// message sent to us by the javascript client
type message struct {
	Handle string `json:"handle"`
	Text   string `json:"text"`
}

// validateMessage so that we know it's valid JSON and contains a Handle and
// Text
func validateMessage(data []byte) (message, error) {
	var msg message

	if err := json.Unmarshal(data, &msg); err != nil {
		return msg, err
	}

	if msg.Handle == "" && msg.Text == "" {
		return msg, fmt.Errorf("Message has no Handle or Text")
	}

	return msg, nil
}

// handleWebsocket connection. Update to
func handleWebsocket(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.WithField("err", err).Println("Upgrading to websockets")
		http.Error(w, "Error Upgrading to websockets", 400)
		return
	}

	id := rr.register(ws)

	for {
		mt, data, err := ws.ReadMessage()
		ctx := log.Fields{"mt": mt, "data": data, "err": err}
		if err != nil {
			if err == io.EOF {
				log.WithFields(ctx).Info("Websocket closed!")
			} else {
				log.WithFields(ctx).Error("Error reading websocket message")
			}
			break
		}
		switch mt {
		case websocket.TextMessage:
			msg, err := validateMessage(data)
			if err != nil {
				ctx["msg"] = msg
				ctx["err"] = err
				log.WithFields(ctx).Error("Invalid Message")
				break
			}
			rw.publish(data)
		default:
			log.WithFields(ctx).Warning("Unknown Message!")
		}
	}

	rr.deRegister(id)

	ws.WriteMessage(websocket.CloseMessage, []byte{})
}
