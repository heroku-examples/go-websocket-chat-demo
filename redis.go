package main

import (
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
	"github.com/gorilla/websocket"
	"github.com/satori/go.uuid"
)

const (
	// Channel name to use with redis
	Channel = "chat"
)

// redisReceiver receives messages from Redis and broadcasts them to all
// registered websocket connections that are Registered.
type redisReceiver struct {
	pool       *redis.Pool
	sync.Mutex // Protects the conns map
	conns      map[string]*websocket.Conn
}

// newRedisReceiver creates a redisReceiver that will use the provided
// rredis.Pool.
func newRedisReceiver(pool *redis.Pool) redisReceiver {
	return redisReceiver{
		pool:  pool,
		conns: make(map[string]*websocket.Conn),
	}
}

// run receives pubsub messages from Redis after establishing a connection.
// When a valid message is received it is broadcast to all connected websockets
func (rr *redisReceiver) run() {
	l := log.WithField("channel", Channel)
	conn := rr.pool.Get()
	defer conn.Close()
	psc := redis.PubSubConn{Conn: conn}
	psc.Subscribe(Channel)
	for {
		switch v := psc.Receive().(type) {
		case redis.Message:
			l.WithField("message", string(v.Data)).Info("Redis Message Received")
			if _, err := validateMessage(v.Data); err != nil {
				l.WithField("err", err).Error("Error unmarshalling message from Redis")
				continue
			}
			rr.broadcast(v.Data)
		case redis.Subscription:
			l.WithFields(logrus.Fields{
				"kind":  v.Kind,
				"count": v.Count,
			}).Println("Redis Subscription Received")
		case error:
			l.WithField("err", v).Error("Error while subscribed to Redis channel")
		default:
			l.WithField("v", v).Info("Unknown Redis receive during subscription")
		}
	}
}

// broadcast the provided message to all connected websocket connections.
// If an error occurs while writting a message to a websocket connection it is
// closed and deregistered.
func (rr *redisReceiver) broadcast(data []byte) {
	rr.Mutex.Lock()
	defer rr.Mutex.Unlock()
	for id, conn := range rr.conns {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.WithFields(logrus.Fields{
				"id":   id,
				"data": data,
				"err":  err,
				"conn": conn,
			}).Error("Error writting data to connection! Closing and removing Connection")
			rr.deRegister(id)
		}
	}
}

// register the websocket connection with the receiver and return a unique
// identifier for the connection. This identifier can be used to deregister the
// connection later
func (rr *redisReceiver) register(conn *websocket.Conn) string {
	rr.Mutex.Lock()
	defer rr.Mutex.Unlock()
	id := uuid.NewV4().String()
	rr.conns[id] = conn
	return id
}

// deRegister the connection by closing it and removing it from our list.
func (rr *redisReceiver) deRegister(id string) {
	rr.Mutex.Lock()
	defer rr.Mutex.Unlock()
	conn, ok := rr.conns[id]
	if ok {
		conn.Close()
		delete(rr.conns, id)
	}
}

// redisWriter publishes messages to the Redis CHANNEL
type redisWriter struct {
	pool     *redis.Pool
	messages chan []byte
}

func newRedisWriter(pool *redis.Pool) redisWriter {
	return redisWriter{
		pool:     pool,
		messages: make(chan []byte),
	}
}

// run the main redisWriter loop that publishes incoming messages to Redis.
func (rw *redisWriter) run() {
	conn := rw.pool.Get()
	defer conn.Close()

	for data := range rw.messages {
		l := log.WithField("data", data)
		if err := conn.Send("PUBLISH", Channel, data); err != nil {
			l.WithField("err", err).Fatalf("Unable to publish message to Redis")
		}
		if err := conn.Flush(); err != nil {
			l.WithField("err", err).Fatalf("Unable to flush published message to Redis")
		}
	}
}

// publish to Redis via channel.
func (rw *redisWriter) publish(data []byte) {
	rw.messages <- data
}
