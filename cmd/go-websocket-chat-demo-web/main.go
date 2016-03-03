package main

import (
	"net/http"
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/codegangsta/negroni"
)

var (
	rr redisReceiver
	rw redisWriter
)

func main() {
	redisURL := os.Getenv("REDIS_URL")
	redisPool, err := newRedisPool(redisURL)
	if err != nil {
		log.WithField("url", redisURL).Fatal("Unable to create Redis pool")
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
	mux.HandleFunc("/ws", handleWebsocket)

	n := negroni.Classic()
	n.UseHandler(mux)
	n.Run(":" + port)
}
