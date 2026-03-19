package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

const channel = "chat"

var (
	waitingMsg, _   = json.Marshal(message{Handle: "system", Text: "Waiting for redis to be available. Messaging won't work until redis is available"})
	availableMsg, _ = json.Marshal(message{Handle: "system", Text: "Redis is now available & messaging is now possible"})
)

// Hub manages WebSocket connections and Redis pub/sub.
type Hub struct {
	rdb    *redis.Client
	logger *slog.Logger

	mu    sync.RWMutex
	conns map[*websocket.Conn]struct{}
}

func NewHub(rdb *redis.Client, logger *slog.Logger) *Hub {
	return &Hub{
		rdb:    rdb,
		logger: logger,
		conns:  make(map[*websocket.Conn]struct{}),
	}
}

func (h *Hub) Register(conn *websocket.Conn) {
	h.mu.Lock()
	h.conns[conn] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) Deregister(conn *websocket.Conn) {
	h.mu.Lock()
	delete(h.conns, conn)
	h.mu.Unlock()
}

func (h *Hub) Broadcast(msg []byte) {
	h.mu.RLock()
	var failed []*websocket.Conn
	for conn := range h.conns {
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			h.logger.Error("write to websocket failed", "err", err)
			failed = append(failed, conn)
		}
	}
	h.mu.RUnlock()

	for _, conn := range failed {
		h.Deregister(conn)
		conn.Close()
	}
}

func (h *Hub) Publish(ctx context.Context, data []byte) error {
	return h.rdb.Publish(ctx, channel, data).Err()
}

func (h *Hub) Subscribe(ctx context.Context) error {
	sub := h.rdb.Subscribe(ctx, channel)
	defer sub.Close()

	h.logger.Info("subscribed to redis channel", "channel", channel)

	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return fmt.Errorf("redis subscription channel closed")
			}
			data := []byte(msg.Payload)
			h.logger.Info("redis message received", "data", msg.Payload)
			if _, err := validateMessage(data); err != nil {
				h.logger.Error("invalid message from redis", "err", err)
				continue
			}
			h.Broadcast(data)
		}
	}
}

func newRedisClient(redisURL string) (*redis.Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parsing redis URL: %w", err)
	}
	if strings.HasPrefix(redisURL, "rediss") {
		opts.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return redis.NewClient(opts), nil
}

func waitForRedis(ctx context.Context, rdb *redis.Client, logger *slog.Logger, onWait func()) error {
	for {
		if err := rdb.Ping(ctx).Err(); err == nil {
			return nil
		}
		logger.Info("redis not yet available, retrying...")
		if onWait != nil {
			onWait()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
}
