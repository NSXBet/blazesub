package blazesub_test

import (
	"sync"
	"testing"
	"time"

	"github.com/NSXBet/blazesub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestControlledInflightMessages provides a more controlled environment
// to verify the InflightMessagesCount with higher precision.
func TestControlledInflightMessages(t *testing.T) {
	t.Parallel()

	t.Run("should accurately track inflight count with synchronized handlers", func(t *testing.T) {
		t.Parallel()
		// Create a bus with default configuration
		bus, err := blazesub.NewBusWithDefaults()
		require.NoError(t, err)

		// Create controlled test handlers with explicit synchronization points
		const subscriberCount = 5

		handlers := make([]*ControlledHandler, subscriberCount)
		for i := range subscriberCount {
			handlers[i] = NewControlledHandler(t)
			sub, subErr := bus.Subscribe("controlled-topic")
			require.NoError(t, subErr)
			sub.OnMessage(handlers[i])
		}

		// Verify initial count is zero
		assert.Equal(t, uint64(0), bus.InflightMessagesCount())

		// Publish a message - handlers will receive it but won't process until we allow it
		bus.Publish("controlled-topic", []byte("test-data"))

		// Wait for all handlers to receive the message but not process it
		for i, h := range handlers {
			require.True(t, h.WaitForMessageReceived(200*time.Millisecond),
				"Handler %d did not receive message within timeout", i)
		}

		// At this point, all handlers have received the message but haven't processed it
		// The inflight count should be exactly subscriberCount
		inflightCount := bus.InflightMessagesCount()
		assert.Equal(t, uint64(subscriberCount), inflightCount,
			"Expected exactly %d inflight messages, got %d", subscriberCount, inflightCount)

		// Now release half the handlers to process their messages
		for i := range subscriberCount / 2 {
			handlers[i].AllowProcessing()
		}

		// Wait a bit for those handlers to complete processing
		time.Sleep(50 * time.Millisecond)

		// The inflight count should now be exactly the remaining handlers
		inflightCount = bus.InflightMessagesCount()
		expectedRemaining := uint64(subscriberCount - subscriberCount/2)
		assert.Equal(t, expectedRemaining, inflightCount,
			"Expected exactly %d inflight messages after releasing half, got %d",
			expectedRemaining, inflightCount)

		// Now release the rest of the handlers
		for i := subscriberCount / 2; i < subscriberCount; i++ {
			handlers[i].AllowProcessing()
		}

		// Verify count returns to zero after all handlers finish
		require.Eventually(t, func() bool {
			return bus.InflightMessagesCount() == 0
		}, 200*time.Millisecond, 5*time.Millisecond)
	})

	t.Run("should accurately track exact inflight messages with staggered start/finish", func(t *testing.T) {
		t.Parallel()
		// Create a bus with default configuration
		bus, err := blazesub.NewBusWithDefaults()
		require.NoError(t, err)

		// Create a controlled handler that uses a channel to control when message processing completes
		const messageCount = 10

		handler := NewControlledHandler(t)
		sub, err := bus.Subscribe("sequential-topic")
		require.NoError(t, err)
		sub.OnMessage(handler)

		// Verify initial count is zero
		assert.Equal(t, uint64(0), bus.InflightMessagesCount())

		// Publish messages one at a time, verifying the count after each publish
		for i := range messageCount {
			bus.Publish("sequential-topic", []byte("test-data"))

			// Wait for the handler to receive the message
			require.True(t, handler.WaitForMessageReceived(200*time.Millisecond),
				"Handler did not receive message %d within timeout", i)

			// Verify inflight count increases exactly by 1
			expectedCount := uint64(i + 1)
			assert.Equal(t, expectedCount, bus.InflightMessagesCount(),
				"Expected exactly %d inflight messages after publishing %d messages, got %d",
				expectedCount, i+1, bus.InflightMessagesCount())
		}

		// Now release the messages for processing one by one
		for i := range messageCount {
			handler.AllowProcessing()

			// Wait a bit for the message to be processed
			time.Sleep(20 * time.Millisecond)

			// Verify inflight count decreases exactly by 1
			expectedCount := uint64(messageCount - (i + 1))
			assert.Equal(t, expectedCount, bus.InflightMessagesCount(),
				"Expected exactly %d inflight messages after processing %d messages, got %d",
				expectedCount, i+1, bus.InflightMessagesCount())
		}
	})

	t.Run("should accurately track inflight messages in wildcard subscriptions", func(t *testing.T) {
		t.Parallel()
		// Create a bus with default configuration
		bus, err := blazesub.NewBusWithDefaults()
		require.NoError(t, err)

		// Create a controlled handler
		handler := NewControlledHandler(t)
		sub, err := bus.Subscribe("wild/+/topic")
		require.NoError(t, err)
		sub.OnMessage(handler)

		// Verify initial count is zero
		assert.Equal(t, uint64(0), bus.InflightMessagesCount())

		// Publish several messages to different matching topics
		topicCount := 3

		bus.Publish("wild/1/topic", []byte("data-1"))
		bus.Publish("wild/2/topic", []byte("data-2"))
		bus.Publish("wild/3/topic", []byte("data-3"))

		// Wait for all messages to be received but not processed
		for i := range topicCount {
			require.True(t, handler.WaitForMessageReceived(200*time.Millisecond),
				"Handler did not receive message %d within timeout", i)
		}

		// The inflight count should be exactly the number of published messages
		assert.Equal(t, uint64(topicCount), bus.InflightMessagesCount(),
			"Expected exactly %d inflight messages, got %d",
			topicCount, bus.InflightMessagesCount())

		// Now release all messages for processing
		for range topicCount {
			handler.AllowProcessing()
		}

		// Verify count returns to zero after all processing completes
		require.Eventually(t, func() bool {
			return bus.InflightMessagesCount() == 0
		}, 200*time.Millisecond, 5*time.Millisecond)
	})
}

// ControlledHandler is a message handler that allows precise control over
// when a message is considered "processed".
type ControlledHandler struct {
	t *testing.T

	// Signal when a message is received
	receivedSignal chan struct{}

	// Gate that blocks processing until allowed
	processingAllowed chan struct{}

	// Track number of messages waiting to be processed
	pendingMessages int
	mu              sync.Mutex
}

func NewControlledHandler(t *testing.T) *ControlledHandler {
	return &ControlledHandler{
		t:                 t,
		receivedSignal:    make(chan struct{}, 100), // Buffer to avoid blocking
		processingAllowed: make(chan struct{}),      // Unbuffered to block processing
	}
}

// OnMessage implements blazesub.MessageHandler.
func (h *ControlledHandler) OnMessage(_ *blazesub.Message[[]byte]) error {
	// Signal that we received a message
	h.mu.Lock()
	h.pendingMessages++
	h.mu.Unlock()

	// Signal that a message was received
	h.receivedSignal <- struct{}{}

	// Block until processing is allowed
	<-h.processingAllowed

	// Message is now processed
	h.mu.Lock()
	h.pendingMessages--
	h.mu.Unlock()

	return nil
}

// WaitForMessageReceived waits for a message to be received within the timeout.
func (h *ControlledHandler) WaitForMessageReceived(timeout time.Duration) bool {
	select {
	case <-h.receivedSignal:
		return true
	case <-time.After(timeout):
		return false
	}
}

// AllowProcessing allows one message to complete processing.
func (h *ControlledHandler) AllowProcessing() {
	h.processingAllowed <- struct{}{}
}

// PendingMessages returns the number of messages waiting to be processed.
func (h *ControlledHandler) PendingMessages() int {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.pendingMessages
}
