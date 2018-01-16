package main

import (
	"net/http"
	"os"

	"github.com/Sirupsen/logrus"
)

var (
	log = logrus.WithField("cmd", "go-websocket-chat-demo")
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		log.WithField("PORT", port).Fatal("$PORT must be set")
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		log.WithField("REDIS_URL", redisURL).Fatal("$REDIS_URL must be set")
	}

	router := NewRouter(redisURL)
	http.Handle("/", router)
	log.Println(http.ListenAndServe(":"+port, nil))
}
