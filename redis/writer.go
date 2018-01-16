package redis

import (
	"github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
	"github.com/pkg/errors"
)

// Writer publishes messages to the Redis CHANNEL
type Writer struct {
	pool *redis.Pool

	channel  string
	messages chan []byte
}

//NewWriter creates a new redis writer and returns it
func NewWriter(pool *redis.Pool, channel string) Writer {
	return Writer{
		pool:     pool,
		channel:  channel,
		messages: make(chan []byte, 10000),
	}
}

//Setup spawns the redis writer service
func (rw *Writer) Setup(redisURL string) {
	for {
		waited, err := waitForAvailability(redisURL, waitTimeout, nil, nil)
		if !waited || err != nil {
			log.WithFields(logrus.Fields{"waitTimeout": waitTimeout, "err": err}).Fatal("Redis not available by timeout!")
		}
		err = rw.Run()
		if err == nil {
			break
		}
		log.Error(err)
	}
}

// Run the main redisWriter loop that publishes incoming messages to Redis.
func (rw *Writer) Run() error {
	conn := rw.pool.Get()
	defer conn.Close()

	for data := range rw.messages {
		if err := rw.writeToRedis(conn, data); err != nil {
			rw.Publish(data) // attempt to redeliver later
			return err
		}
	}
	return nil
}

func (rw *Writer) writeToRedis(conn redis.Conn, data []byte) error {
	if err := conn.Send("PUBLISH", rw.channel, data); err != nil {
		return errors.Wrap(err, "Unable to publish message to Redis")
	}
	if err := conn.Flush(); err != nil {
		return errors.Wrap(err, "Unable to flush published message to Redis")
	}
	return nil
}

// Publish to Redis via channel.
func (rw *Writer) Publish(data []byte) {
	rw.messages <- data
}
