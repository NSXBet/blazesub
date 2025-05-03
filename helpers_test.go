package blazesub_test

import (
	"sync"
	"testing"
	"time"

	"github.com/NSXBet/blazesub"
)

type SpyHandler struct {
	mutex    sync.Mutex
	Messages map[string][]*blazesub.Message
	delay    time.Duration
}

var _ blazesub.MessageHandler = &SpyHandler{}

func (h *SpyHandler) OnMessage(message *blazesub.Message) error {
	time.Sleep(h.delay)

	h.mutex.Lock()
	defer h.mutex.Unlock()

	if h.Messages == nil {
		h.Messages = make(map[string][]*blazesub.Message)
	}

	h.Messages[message.Topic] = append(h.Messages[message.Topic], message)

	return nil
}

func (h *SpyHandler) MessagesReceived() map[string][]*blazesub.Message {
	return h.Messages
}

func (h *SpyHandler) SetDelay(delay time.Duration) {
	h.delay = delay
}

func SpyMessageHandler(t *testing.T) *SpyHandler {
	t.Helper()

	return &SpyHandler{}
}
