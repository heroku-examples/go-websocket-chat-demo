package main

import (
	"encoding/json"
	"net/http"
	"os"

	log "github.com/heroku-examples/go-websocket-chat-demo/Godeps/_workspace/src/github.com/Sirupsen/logrus"
	"github.com/heroku-examples/go-websocket-chat-demo/Godeps/_workspace/src/github.com/codegangsta/negroni"
	"github.com/heroku-examples/go-websocket-chat-demo/Godeps/_workspace/src/github.com/gorilla/websocket"
)

type message struct {
	Handle string `json:"handle"`
	Text   string `json:"text"`
}

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	rr redisReceiver
	rw redisWriter
)

func serveWs(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.WithField("Error", err).Println("Upgrading to websockets")
		return
	}

	id := rr.register(ws)

	for {
		mt, data, err := ws.ReadMessage()
		if err != nil {
			break
		}
		switch mt {
		case websocket.TextMessage:
			var msg message
			// This is here to validate the message and
			if err := json.Unmarshal(data, &msg); err != nil {
				log.WithField("data", string(data)).Println("Unable to unmarshal TextMessage to JSON!")
				break
			}
			if msg.Handle != "" && msg.Text != "" {
				rw.writeMessage(data)
				log.WithFields(
					log.Fields{
						"mt":     mt,
						"Handle": msg.Handle,
						"Text":   msg.Text,
					}).Println("Message written to redis")
				continue
			}
			log.WithFields(
				log.Fields{
					"mt":     mt,
					"Handle": msg.Handle,
					"Text":   msg.Text,
				}).Warning("Messed not written to redis")

		default:
			log.WithFields(
				log.Fields{
					"mt":   mt,
					"data": data,
					"err":  err,
				}).Println("Message!")
		}
	}

	rr.deRegister(id)

	ws.WriteMessage(websocket.CloseMessage, []byte{})
}

func main() {
	redisURL := os.Getenv("REDIS_URL")
	redisPool, err := newRedisPool(redisURL)
	if err != nil {
		log.WithField("url", redisURL).Fatal("Unable to create redis pool")
	}

	rr = newRedisReceiver(redisPool)
	go rr.run()
	rw = newRedisWriter(redisPool)
	go rw.run()

	port := os.Getenv("PORT")
	if port == "" {
		log.WithField("PORT", port).Fatal("$PORT must be set")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", serveWs)

	n := negroni.Classic()
	n.UseHandler(mux)
	n.Run(":" + port)
}
