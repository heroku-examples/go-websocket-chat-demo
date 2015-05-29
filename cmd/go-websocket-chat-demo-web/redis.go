package main

import (
	"encoding/json"
	"net/url"
	"sync"
	"time"

	"github.com/heroku-examples/go-websocket-chat-demo/Godeps/_workspace/src/code.google.com/p/go-uuid/uuid"
	log "github.com/heroku-examples/go-websocket-chat-demo/Godeps/_workspace/src/github.com/Sirupsen/logrus"
	"github.com/heroku-examples/go-websocket-chat-demo/Godeps/_workspace/src/github.com/garyburd/redigo/redis"
	"github.com/heroku-examples/go-websocket-chat-demo/Godeps/_workspace/src/github.com/gorilla/websocket"
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

// redisReceiver receives redis messages and broadcasts them to all registered
// websocket connections
type redisReceiver struct {
	pool       *redis.Pool
	sync.Mutex // Protects the conns map
	conns      map[string]*websocket.Conn
}

func newRedisReceiver(pool *redis.Pool) redisReceiver {
	return redisReceiver{
		pool:  pool,
		conns: make(map[string]*websocket.Conn),
	}
}

func (rr *redisReceiver) run() {
	conn := rr.pool.Get()
	defer conn.Close()
	psc := redis.PubSubConn{conn}
	psc.Subscribe("chat")
	for {
		switch v := psc.Receive().(type) {
		case redis.Message:
			log.WithFields(log.Fields{
				"channel": v.Channel,
				"message": v.Data,
			}).Println("Redis Message Received")
			msg := message{}
			if err := json.Unmarshal(v.Data, &msg); err != nil {
				log.WithFields(log.Fields{
					"err":  err,
					"data": v.Data,
				}).Error("Error unmarshalling message from redis")
				continue
			}
			if msg.Handle != "" && msg.Text != "" {
				rr.broadcast(v.Data)
			}

		case redis.Subscription:
			log.WithFields(log.Fields{
				"channel": v.Channel,
				"kind":    v.Kind,
				"count":   v.Count,
			}).Println("Redis Subscription Received")
		case error:
			log.WithField("err", v).Error("Error while subscribed to redis")
		default:
			log.WithField("v", v).Println("Unknown redis receive during subscription")
		}
	}
}

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
			conn.Close()
			rr.deRegister(id)
		}
	}
}

func (rr *redisReceiver) register(conn *websocket.Conn) string {
	rr.Mutex.Lock()
	defer rr.Mutex.Unlock()
	id := uuid.New()
	rr.conns[id] = conn
	return id
}

func (rr *redisReceiver) deRegister(id string) {
	rr.Mutex.Lock()
	defer rr.Mutex.Unlock()
	delete(rr.conns, id)
}

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

func (rw *redisWriter) run() {
	for data := range rw.messages {
		if err := rw.write(data); err != nil {
			log.WithFields(log.Fields{
				"data": data,
				"err":  err,
			}).Panic("Error writing to redis")
		}
	}
}

func (rw *redisWriter) write(data []byte) error {
	conn := rw.pool.Get()
	defer conn.Close()

	if err := conn.Send("PUBLISH", "chat", data); err != nil {
		return err
	}
	if err := conn.Flush(); err != nil {
		return err
	}
	return nil
}

func (rw *redisWriter) writeMessage(data []byte) {
	rw.messages <- data
}
