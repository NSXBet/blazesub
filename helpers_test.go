package blazesub_test

import (
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/NSXBet/blazesub"
	mochimqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

type SpyHandler struct {
	mutex    sync.Mutex
	Messages map[string][]*blazesub.Message
	delay    time.Duration
}

var _ blazesub.MessageHandler = (*SpyHandler)(nil)

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
	h.mutex.Lock()
	defer h.mutex.Unlock()

	return h.Messages
}

func (h *SpyHandler) SetDelay(delay time.Duration) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.delay = delay
}

// Reset clears all stored messages.
func (h *SpyHandler) Reset() {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.Messages = make(map[string][]*blazesub.Message)
}

func SpyMessageHandler(t *testing.T) *SpyHandler {
	t.Helper()

	return &SpyHandler{
		Messages: make(map[string][]*blazesub.Message),
		delay:    time.Millisecond * 10,
		mutex:    sync.Mutex{},
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
