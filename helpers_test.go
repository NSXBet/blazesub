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

type SpyHandler[T any] struct {
	messages *xsync.Map[string, []*blazesub.Message[T]]
	delay    time.Duration
}

var _ blazesub.MessageHandler[any] = (*SpyHandler[any])(nil)

func (h *SpyHandler[T]) OnMessage(message *blazesub.Message[T]) error {
	if h.delay > 0 {
		time.Sleep(h.delay)
	}

	if h.messages == nil {
		h.messages = xsync.NewMap[string, []*blazesub.Message[T]]()
	}

	// Get current messages for this topic or create empty slice
	messagesForTopic, _ := h.messages.LoadOrStore(message.Topic, make([]*blazesub.Message[T], 0))

	// Create a new slice with the message appended
	newMessages := make([]*blazesub.Message[T], 0, len(messagesForTopic)+1)
	newMessages = append(newMessages, messagesForTopic...)

	// Deep copy the message for the spy handler to avoid issues with pooled messages being reset
	copiedMsg := &blazesub.Message[T]{
		Topic:        message.Topic,
		UTCTimestamp: message.UTCTimestamp,
	}

	// Deep copy Data (assuming T is []byte for most tests, handle specifically if T can be other pointer types)
	// For the purpose of fixing tests, we'll assume T is often []byte or a value type.
	// If T is a pointer type itself, a more general deep copy mechanism might be needed,
	// but that's too complex for this immediate fix.
	if dataBytes, ok := any(message.Data).([]byte); ok {
		copiedData := make([]byte, len(dataBytes))
		copy(copiedData, dataBytes)
		copiedMsg.Data = any(copiedData).(T)
	} else {
		// For non-[]byte types, assume direct assignment is okay for now
		// This might not be a perfect deep copy for all generic types T
		copiedMsg.Data = message.Data
	}

	// Deep copy Metadata
	if message.Metadata != nil {
		copiedMsg.Metadata = make(map[string]any, len(message.Metadata))
		for k, v := range message.Metadata {
			// This is a shallow copy of metadata values. If metadata contains pointers, they won't be deep copied.
			// For current tests, this should be sufficient.
			copiedMsg.Metadata[k] = v
		}
	}

	newMessages = append(newMessages, copiedMsg)
	h.messages.Store(message.Topic, newMessages)

	return nil
}

func (h *SpyHandler[T]) MessagesReceived() map[string][]*blazesub.Message[T] {
	if h.messages == nil {
		return make(map[string][]*blazesub.Message[T])
	}

	// Create a copy of the map with deep copies of the message slices.
	// We can't use xsync.ToPlainMap because it doesn't deep copy the message slices.
	result := make(map[string][]*blazesub.Message[T])

	h.messages.Range(func(topic string, messages []*blazesub.Message[T]) bool {
		// Create a deep copy of the messages slice to avoid concurrent access issues
		messagesCopy := make([]*blazesub.Message[T], len(messages))
		copy(messagesCopy, messages)
		result[topic] = messagesCopy

		return true
	})

	return result
}

func (h *SpyHandler[T]) SetDelay(delay time.Duration) {
	h.delay = delay
}

// Reset clears all stored messages.
func (h *SpyHandler[T]) Reset() {
	h.messages = xsync.NewMap[string, []*blazesub.Message[T]]()
}

func SpyMessageHandler[T any](t *testing.T) *SpyHandler[T] {
	t.Helper()

	return &SpyHandler[T]{
		messages: xsync.NewMap[string, []*blazesub.Message[T]](),
		delay:    time.Millisecond * 10,
	}
}

type noOpHandler struct {
	MessageCount *atomic.Int64
}

var _ blazesub.MessageHandler[[]byte] = (*noOpHandler)(nil)

//nolint:revive // internal test tool.
func NoOpHandler(tb testing.TB) *noOpHandler {
	tb.Helper()

	return &noOpHandler{
		MessageCount: atomic.NewInt64(0),
	}
}

func (h *noOpHandler) OnMessage(_ *blazesub.Message[[]byte]) error {
	h.MessageCount.Add(1)

	return nil
}

type SharedCounterHandler struct {
	MessageCount *atomic.Int64
}

var _ blazesub.MessageHandler[[]byte] = (*SharedCounterHandler)(nil)

func (h *SharedCounterHandler) OnMessage(_ *blazesub.Message[[]byte]) error {
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

type LatencyMeasureHandler[T any] struct {
	done chan struct{}
}

var _ blazesub.MessageHandler[any] = (*LatencyMeasureHandler[any])(nil)

func (h *LatencyMeasureHandler[T]) OnMessage(*blazesub.Message[T]) error {
	select {
	case h.done <- struct{}{}:
	default:
	}

	return nil
}
