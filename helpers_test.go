package blazesub_test

import (
	"sync"
	"testing"
	"time"

	"github.com/NSXBet/blazesub"
	"go.uber.org/atomic"
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

// Reset clears all stored messages
func (h *SpyHandler) Reset() {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.Messages = make(map[string][]*blazesub.Message)
}

func SpyMessageHandler(t *testing.T) *SpyHandler {
	t.Helper()

	return &SpyHandler{}
}

type noOpHandler struct {
	MessageCount *atomic.Int64
}

var _ blazesub.MessageHandler = &noOpHandler{}

func NoOpHandler(tb testing.TB) *noOpHandler {
	tb.Helper()

	return &noOpHandler{
		MessageCount: atomic.NewInt64(0),
	}
}

func (h *noOpHandler) OnMessage(message *blazesub.Message) error {
	h.MessageCount.Add(1)

	return nil
}
