package blazesub_test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/NSXBet/blazesub"
	mochimqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

type SpyHandler struct {
	messages *xsync.Map[string, []*blazesub.Message]
	delay    time.Duration
}

var _ blazesub.MessageHandler = (*SpyHandler)(nil)

func (h *SpyHandler) OnMessage(message *blazesub.Message) error {
	if h.delay > 0 {
		time.Sleep(h.delay)
	}

	if h.messages == nil {
		h.messages = xsync.NewMap[string, []*blazesub.Message]()
	}

	// Get current messages for this topic or create empty slice
	messagesForTopic, _ := h.messages.LoadOrStore(message.Topic, make([]*blazesub.Message, 0))

	// Create a new slice with the message appended
	newMessages := append(messagesForTopic, message)
	h.messages.Store(message.Topic, newMessages)

	return nil
}

func (h *SpyHandler) MessagesReceived() map[string][]*blazesub.Message {
	if h.messages == nil {
		return make(map[string][]*blazesub.Message)
	}

	// Create a copy of the map with deep copies of the message slices.
	// We can't use xsync.ToPlainMap because it doesn't deep copy the message slices.
	result := make(map[string][]*blazesub.Message)

	h.messages.Range(func(topic string, messages []*blazesub.Message) bool {
		// Create a deep copy of the messages slice to avoid concurrent access issues
		messagesCopy := make([]*blazesub.Message, len(messages))
		copy(messagesCopy, messages)
		result[topic] = messagesCopy
		return true
	})

	return result
}

func (h *SpyHandler) SetDelay(delay time.Duration) {
	h.delay = delay
}

// Reset clears all stored messages.
func (h *SpyHandler) Reset() {
	h.messages = xsync.NewMap[string, []*blazesub.Message]()
}

func SpyMessageHandler(t *testing.T) *SpyHandler {
	t.Helper()

	return &SpyHandler{
		messages: xsync.NewMap[string, []*blazesub.Message](),
		delay:    time.Millisecond * 10,
	}
}

type noOpHandler struct {
	MessageCount *atomic.Int64
}

var _ blazesub.MessageHandler = (*noOpHandler)(nil)

//nolint:revive // internal test tool.
func NoOpHandler(tb testing.TB) *noOpHandler {
	tb.Helper()

	return &noOpHandler{
		MessageCount: atomic.NewInt64(0),
	}
}

func (h *noOpHandler) OnMessage(_ *blazesub.Message) error {
	h.MessageCount.Add(1)

	return nil
}

func RunMQTTServer(tb testing.TB) *mochimqtt.Server {
	tb.Helper()

	level := new(slog.LevelVar)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
	level.Set(slog.LevelError)

	server := mochimqtt.New(
		&mochimqtt.Options{
			InlineClient: true,
			Capabilities: &mochimqtt.Capabilities{
				MaximumMessageExpiryInterval: 60,
				RetainAvailable:              0,
				MaximumClientWritesPending:   100,
				MaximumInflight:              100,
			},
			Logger: logger,
		},
	)

	// Allow all connections.
	_ = server.AddHook(new(auth.AllowHook), nil)

	addr := "0.0.0.0:1883"
	socket := listeners.NewTCP(listeners.Config{ID: "t1", Address: addr})

	require.NoError(tb, server.AddListener(socket))

	go func() {
		if err := server.Serve(); err != nil {
			tb.Fatalf("error running server: %s", err)
		}
	}()

	tb.Cleanup(func() {
		server.Close()
	})

	return server
}
