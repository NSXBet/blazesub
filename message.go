package blazesub

import "time"

type Message struct {
	Topic        string
	Data         []byte
	UTCTimestamp time.Time
}

func NewMessage(topic string, data []byte) *Message {
	return &Message{
		Topic:        topic,
		Data:         data,
		UTCTimestamp: time.Now().UTC(),
	}
}
