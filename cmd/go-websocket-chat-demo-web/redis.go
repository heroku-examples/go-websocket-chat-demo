package main

import (
	"net/url"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
	"github.com/gorilla/websocket"
	"github.com/pborman/uuid"
)

const (
	CHANNEL = "chat"
)

func newRedisPool(us string) (*redis.Pool, error) {
	u, err := url.Parse(us)
	if err != nil {
		return nil, err
	}

	var password string
	if u.User != nil {
		password, _ = u.User.Password()
	}
	return &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", u.Host)
			if err != nil {
				return nil, err
			}
			if password != "" {
				if _, err := c.Do("AUTH", password); err != nil {
					c.Close()
					return nil, err
				}
			}
			return c, err
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}, nil
}

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
	conn := rr.pool.Get()
	defer conn.Close()
	psc := redis.PubSubConn{conn}
	psc.Subscribe(CHANNEL)
	for {
		switch v := psc.Receive().(type) {
		case redis.Message:
			log.WithFields(log.Fields{
				"channel": v.Channel,
				"message": string(v.Data),
			}).Println("Redis Message Received")
			msg, err := validateMessage(v.Data)
			if err != nil {
				log.WithFields(log.Fields{
					"err":  err,
					"data": v.Data,
					"msg":  msg,
				}).Error("Error unmarshalling message from Redis")
				continue
			}
			rr.broadcast(v.Data)
		case redis.Subscription:
			log.WithFields(log.Fields{
				"channel": v.Channel,
				"kind":    v.Kind,
				"count":   v.Count,
			}).Println("Redis Subscription Received")
		case error:
			log.WithField("err", v).Errorf("Error while subscribed to Redis channel %s", CHANNEL)
		default:
			log.WithField("v", v).Println("Unknown Redis receive during subscription")
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
			log.WithFields(log.Fields{
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
	id := uuid.New()
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
		ctx := log.Fields{"data": data}
		if err := conn.Send("PUBLISH", CHANNEL, data); err != nil {
			ctx["err"] = err
			log.WithFields(ctx).Fatalf("Unable to publish message to Redis")
		}
		if err := conn.Flush(); err != nil {
			ctx["err"] = err
			log.WithFields(ctx).Fatalf("Unable to flush published message to Redis")
		}
	}
}

// publish to Redis via channel.
func (rw *redisWriter) publish(data []byte) {
	rw.messages <- data
}
