package redis

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
)

var (
	log         = logrus.WithField("cmd", "go-websocket-chat-demo-redis")
	waitSleep   = time.Second * 10
	waitTimeout = time.Minute * 10
)

// message sent to us by the javascript client
type message struct {
	Handle string `json:"handle"`
	Text   string `json:"text"`
}

// Receiver receives messages from Redis and broadcasts them to all
// registered websocket connections that are Registered.
type Receiver struct {
	pool *redis.Pool

	validateMessage func([]byte) error
	channel         string
	messages        chan []byte
	newConnections  chan *websocket.Conn
	rmConnections   chan *websocket.Conn
}

// NewReceiver creates a redisReceiver that will use the provided
// rredis.Pool.
func NewReceiver(pool *redis.Pool, channel string, validateMessage func([]byte) error) Receiver {
	return Receiver{
		pool:            pool,
		channel:         channel,
		validateMessage: validateMessage,
		messages:        make(chan []byte, 1000), // 1000 is arbitrary
		newConnections:  make(chan *websocket.Conn),
		rmConnections:   make(chan *websocket.Conn),
	}
}

//Wait for redis availability
func (rr *Receiver) Wait(_ time.Time, waitingMessage []byte) error {
	rr.Broadcast(waitingMessage)
	time.Sleep(waitSleep)
	return nil
}

//Setup spawns redis receiver service
func (rr *Receiver) Setup(redisURL string) {
	for {
		waitingMessage, _ := json.Marshal(message{
			Handle: "redis",
			Text:   "Waiting for redis to be available. Messaging won't work until redis is available",
		})
		waited, err := waitForAvailability(redisURL, waitTimeout, waitingMessage, rr.Wait)
		if !waited || err != nil {
			log.WithFields(logrus.Fields{"waitTimeout": waitTimeout, "err": err}).Fatal("Redis not available by timeout!")
		}
		availableMessage, _ := json.Marshal(message{
			Handle: "redis",
			Text:   "Redis is now available & messaging is now possible",
		})
		rr.Broadcast(availableMessage)
		err = rr.Run()
		if err == nil {
			break
		}
		log.Error(err)
	}
}

// Run receives pubsub messages from Redis after establishing a connection.
// When a valid message is received it is broadcast to all connected websockets
func (rr *Receiver) Run() error {
	l := log.WithField("channel", rr.channel)
	if rr.channel == "" {
		return errors.New("Redis channel is not set")
	}

	conn := rr.pool.Get()
	defer conn.Close()
	psc := redis.PubSubConn{Conn: conn}
	psc.Subscribe(rr.channel)
	go rr.connHandler()
	for {
		switch v := psc.Receive().(type) {
		case redis.Message:
			l.WithField("message", string(v.Data)).Info("Redis Message Received")
			if err := rr.validateMessage(v.Data); err != nil {
				if status := validateStatusMessage(v.Data); status != nil {
					l.WithField("err", err).Error("Error unmarshalling message from Redis")
					continue
				}
			}
			rr.Broadcast(v.Data)
		case redis.Subscription:
			l.WithFields(logrus.Fields{
				"kind":  v.Kind,
				"count": v.Count,
			}).Println("Redis Subscription Received")
		case error:
			return errors.Wrap(v, "Error while subscribed to Redis channel")
		default:
			l.WithField("v", v).Info("Unknown Redis receive during subscription")
		}
	}
}

// Broadcast the provided message to all connected websocket connections.
// If an error occurs while writting a message to a websocket connection it is
// closed and deregistered.
func (rr *Receiver) Broadcast(msg []byte) {
	rr.messages <- msg
}

// Register the websocket connection with the receiver.
func (rr *Receiver) Register(conn *websocket.Conn) {
	rr.newConnections <- conn
}

// DeRegister the connection by closing it and removing it from our list.
func (rr *Receiver) DeRegister(conn *websocket.Conn) {
	rr.rmConnections <- conn
}

func (rr *Receiver) connHandler() {
	conns := make([]*websocket.Conn, 0)
	defer func() {
		for _, conn := range conns {
			conn.Close()
		}
	}()

	for {
		select {
		case msg := <-rr.messages:
			for _, conn := range conns {
				if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					log.WithFields(logrus.Fields{
						"data": msg,
						"err":  err,
						"conn": conn,
					}).Error("Error writting data to connection! Closing and removing Connection")
					conns = removeConn(conns, conn)
				}
			}
		case conn := <-rr.newConnections:
			conns = append(conns, conn)
		case conn := <-rr.rmConnections:
			conns = removeConn(conns, conn)
		}
	}
}

func removeConn(conns []*websocket.Conn, remove *websocket.Conn) []*websocket.Conn {
	var i int
	var found bool
	for i = 0; i < len(conns); i++ {
		if conns[i] == remove {
			found = true
			break
		}
	}
	if !found {
		fmt.Printf("conns: %#v\nconn: %#v\n", conns, remove)
		panic("Conn not found")
	}
	copy(conns[i:], conns[i+1:]) // shift down
	conns[len(conns)-1] = nil    // nil last element
	return conns[:len(conns)-1]  // truncate slice
}

func validateStatusMessage(data []byte) error {
	var msg message
	if err := json.Unmarshal(data, &msg); err != nil {
		return errors.Wrap(err, "Unmarshaling message")
	}
	if msg.Handle == "" && msg.Text == "" {
		return errors.New("Message has no Handle or Text")
	}
	return nil
}
