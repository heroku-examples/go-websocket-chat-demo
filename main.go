package main

import (
	"net/http"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/heroku/gobits/redis"
)

var (
	log = logrus.WithField("cmd", "go-websocket-chat-demo")
	rr  redisReceiver
	rw  redisWriter
)

func main() {
	redisURL := os.Getenv("REDIS_URL")
	redisPool, err := redis.NewRedisPoolFromURL(redisURL)
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

	http.Handle("/", http.FileServer(http.Dir("./public")))
	http.HandleFunc("/ws", handleWebsocket)
	log.Println(http.ListenAndServe(":"+port, nil))
}
