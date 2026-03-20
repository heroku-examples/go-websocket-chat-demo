package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	logger := slog.Default()

	port := os.Getenv("PORT")
	if port == "" {
		logger.Error("$PORT must be set")
		os.Exit(1)
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		logger.Error("$REDIS_URL must be set")
		os.Exit(1)
	}

	rdb, err := newRedisClient(redisURL)
	if err != nil {
		logger.Error("failed to create redis client", "err", err)
		os.Exit(1)
	}

	hub := NewHub(rdb, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		for {
			if err := waitForRedis(ctx, rdb, logger, func() {
				hub.Broadcast(waitingMsg)
			}); err != nil {
				return
			}
			hub.Broadcast(availableMsg)

			if err := hub.Subscribe(ctx); err != nil {
				if ctx.Err() != nil {
					return
				}
				logger.Error("redis subscription error, reconnecting", "err", err)
				continue
			}
			return
		}
	}()

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir("./public")))
	mux.HandleFunc("/ws", handleWebSocket(hub, logger))

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	go func() {
		logger.Info("server starting", "port", port)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			logger.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)
	rdb.Close()
}
