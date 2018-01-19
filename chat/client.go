package chat

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"github.com/rmattam/go-websocket-chat-demo/redis"
)

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	waitTimeout = time.Minute * 10
	log         = logrus.WithField("cmd", "go-websocket-chat-demo-chat")
)

//Hub struct
type Hub struct {
	channel string

	receive redis.Receiver
	write   redis.Writer
}

// message sent to us by the javascript client
type message struct {
	Handle string `json:"handle"`
	Text   string `json:"text"`
}

// SubsribeRedis : Initialize the redis routines required for pub sub.
func (c *Hub) SubsribeRedis(redisURL string, channel string) {
	redisPool, err := redis.NewRedisPoolFromURL(redisURL)
	if err != nil {
		log.WithField("url", redisURL).Fatal("Unable to create Redis pool")
	}
	c.channel = channel
	c.receive = redis.NewReceiver(redisPool, c.channel, ValidateRedisMessage)
	c.write = redis.NewWriter(redisPool, c.channel)
	go c.receive.Setup(redisURL)
	go c.write.Setup(redisURL)
}

//ValidateRedisMessage validates incoming redis messages on the chat channel.
func ValidateRedisMessage(data []byte) error {
	_, e := validateMessage(data)
	return e
}

// validateMessage so that we know it's valid JSON and contains a Handle and
// Text
func validateMessage(data []byte) (message, error) {
	var msg message

	if err := json.Unmarshal(data, &msg); err != nil {
		return msg, errors.Wrap(err, "Unmarshaling message")
	}

	if msg.Handle == "" && msg.Text == "" {
		return msg, errors.New("Message has no Handle or Text")
	}

	return msg, nil
}

// HandleWebsocket connection.
func HandleWebsocket(c *Hub, w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	defer func() {
		ws.Close()
	}()

	if err != nil {
		m := "Unable to upgrade to websockets"
		log.WithField("err", err).Println(m)
		http.Error(w, m, http.StatusBadRequest)
		return
	}

	c.receive.Register(ws)

	for {
		mt, data, err := ws.ReadMessage()
		l := log.WithFields(logrus.Fields{"mt": mt, "err": err})
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseGoingAway) || err == io.EOF {
				l.Info("Websocket closed!")
				break
			}
			l.WithField("data", data).Error("Error reading websocket message")
		}
		switch mt {
		case websocket.TextMessage:
			msg, err := validateMessage(data)
			if err != nil {
				l.WithFields(logrus.Fields{"data": data, "msg": msg, "err": err}).Error("Invalid Message")
				break
			}
			l.WithField("msg", msg).Info("message from client")
			c.write.Publish(data)
		default:
			l.WithField("data", data).Warning("Unknown Message!")
		}
	}

	c.receive.DeRegister(ws)
	ws.WriteMessage(websocket.CloseMessage, []byte{})
}
