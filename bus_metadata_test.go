package blazesub_test

import (
	"testing"
	"time"

	"github.com/NSXBet/blazesub"
	"github.com/stretchr/testify/require"
)

func TestPublishWithMetadata(t *testing.T) {
	t.Parallel()

	t.Run("single subscriber receives metadata", func(t *testing.T) {
		t.Parallel()

		// Create bus with default config
		bus, err := blazesub.NewBusWithDefaults()
		require.NoError(t, err)
		defer bus.Close()

		// Create a subscription
		subscription, err := bus.Subscribe("sensors/temperature")
		require.NoError(t, err)

		// Create message spy handler
		handler := SpyMessageHandler[[]byte](t)
		subscription.OnMessage(handler)

		// Create metadata
		metadata := map[string]any{
			"unit":      "celsius",
			"precision": 0.1,
			"device_id": "thermostat-living-room",
			"timestamp": time.Now().UnixNano(),
		}

		// Publish a message with metadata
		bus.Publish("sensors/temperature", []byte("22.5"), metadata)

		// Wait for message to be received
		require.Eventually(t, func() bool {
			messages := handler.MessagesReceived()
			return len(messages["sensors/temperature"]) > 0
		}, time.Second, time.Millisecond*10, "message was not received")

		// Verify metadata was received
		receivedMessages := handler.MessagesReceived()["sensors/temperature"]
		require.Len(t, receivedMessages, 1, "should have received exactly one message")

		receivedMessage := receivedMessages[0]
		require.Equal(t, []byte("22.5"), receivedMessage.Data, "message data should match")
		require.Equal(t, "sensors/temperature", receivedMessage.Topic, "message topic should match")

		// Verify all metadata fields
		require.NotNil(t, receivedMessage.Metadata, "metadata should not be nil")
		require.Equal(t, "celsius", receivedMessage.Metadata["unit"], "unit metadata should match")

		//nolint:testifylint // reason: it must match the float64 value
		require.EqualValues(t, 0.1, receivedMessage.Metadata["precision"], "precision metadata should match")
		require.Equal(
			t,
			"thermostat-living-room",
			receivedMessage.Metadata["device_id"],
			"device_id metadata should match",
		)
		require.NotZero(t, receivedMessage.Metadata["timestamp"], "timestamp metadata should be non-zero")
	})

	t.Run("multiple subscribers all receive same metadata", func(t *testing.T) {
		t.Parallel()

		// Create bus with default config
		bus, err := blazesub.NewBusWithDefaults()
		require.NoError(t, err)
		defer bus.Close()

		// Create multiple subscriptions with different patterns all matching the same topic
		exactSub, err := bus.Subscribe("home/sensors/temperature")
		require.NoError(t, err)

		plusSub, err := bus.Subscribe("home/+/temperature")
		require.NoError(t, err)

		hashSub, err := bus.Subscribe("home/#")
		require.NoError(t, err)

		// Create message spy handlers
		exactHandler := SpyMessageHandler[[]byte](t)
		plusHandler := SpyMessageHandler[[]byte](t)
		hashHandler := SpyMessageHandler[[]byte](t)

		exactSub.OnMessage(exactHandler)
		plusSub.OnMessage(plusHandler)
		hashSub.OnMessage(hashHandler)

		// Create complex metadata with various types
		metadata := map[string]any{
			"unit":   "celsius",
			"values": []float64{22.5, 22.6, 22.4}, // Array of values
			"device": map[string]any{ // Nested object
				"id":       "temp-sensor-123",
				"location": "living-room",
				"active":   true,
			},
			"timestamp": time.Now().Unix(),
			"valid":     true,
		}

		// Publish a message with metadata
		bus.Publish("home/sensors/temperature", []byte("22.5"), metadata)

		// Wait for all messages to be received
		require.Eventually(t, func() bool {
			return len(exactHandler.MessagesReceived()["home/sensors/temperature"]) > 0 &&
				len(plusHandler.MessagesReceived()["home/sensors/temperature"]) > 0 &&
				len(hashHandler.MessagesReceived()["home/sensors/temperature"]) > 0
		}, time.Second, time.Millisecond*10, "not all messages were received")

		// Verify metadata for exact match subscriber
		exactMsg := exactHandler.MessagesReceived()["home/sensors/temperature"][0]
		require.NotNil(t, exactMsg.Metadata, "metadata should not be nil for exact subscriber")
		require.Equal(t, "celsius", exactMsg.Metadata["unit"], "unit metadata should match for exact subscriber")
		require.Equal(
			t,
			[]float64{22.5, 22.6, 22.4},
			exactMsg.Metadata["values"],
			"values array should match for exact subscriber",
		)

		// Verify nested map
		deviceMap, ok := exactMsg.Metadata["device"].(map[string]any)
		require.True(t, ok, "device should be a map[string]any for exact subscriber")
		require.Equal(t, "temp-sensor-123", deviceMap["id"], "device id should match for exact subscriber")
		require.Equal(t, "living-room", deviceMap["location"], "device location should match for exact subscriber")
		require.Equal(t, true, deviceMap["active"], "device active should match for exact subscriber")

		require.Equal(t, true, exactMsg.Metadata["valid"], "valid flag should match for exact subscriber")

		// Verify metadata for + wildcard subscriber
		plusMsg := plusHandler.MessagesReceived()["home/sensors/temperature"][0]
		require.NotNil(t, plusMsg.Metadata, "metadata should not be nil for + subscriber")
		require.Equal(
			t,
			exactMsg.Metadata["unit"],
			plusMsg.Metadata["unit"],
			"unit metadata should match between subscribers",
		)
		require.Equal(
			t,
			exactMsg.Metadata["timestamp"],
			plusMsg.Metadata["timestamp"],
			"timestamp should match between subscribers",
		)

		// Verify metadata for # wildcard subscriber
		hashMsg := hashHandler.MessagesReceived()["home/sensors/temperature"][0]
		require.NotNil(t, hashMsg.Metadata, "metadata should not be nil for # subscriber")
		require.Equal(
			t,
			exactMsg.Metadata["unit"],
			hashMsg.Metadata["unit"],
			"unit metadata should match between subscribers",
		)
		require.Equal(
			t,
			exactMsg.Metadata["timestamp"],
			hashMsg.Metadata["timestamp"],
			"timestamp should match between subscribers",
		)
	})

	t.Run("publish without metadata sets nil metadata field", func(t *testing.T) {
		t.Parallel()

		// Create bus with default config
		bus, err := blazesub.NewBusWithDefaults()
		require.NoError(t, err)
		defer bus.Close()

		// Create a subscription
		subscription, err := bus.Subscribe("no-metadata/topic")
		require.NoError(t, err)

		// Create message spy handler
		handler := SpyMessageHandler[[]byte](t)
		subscription.OnMessage(handler)

		// Publish a message without metadata
		bus.Publish("no-metadata/topic", []byte("test-data"))

		// Wait for message to be received
		require.Eventually(t, func() bool {
			messages := handler.MessagesReceived()
			return len(messages["no-metadata/topic"]) > 0
		}, time.Second, time.Millisecond*10, "message was not received")

		// Verify metadata is nil
		receivedMessage := handler.MessagesReceived()["no-metadata/topic"][0]
		require.Nil(t, receivedMessage.Metadata, "metadata should be nil when not provided")
	})

	t.Run("empty metadata map is preserved", func(t *testing.T) {
		t.Parallel()

		// Create bus with default config
		bus, err := blazesub.NewBusWithDefaults()
		require.NoError(t, err)
		defer bus.Close()

		// Create a subscription
		subscription, err := bus.Subscribe("empty-metadata/topic")
		require.NoError(t, err)

		// Create message spy handler
		handler := SpyMessageHandler[[]byte](t)
		subscription.OnMessage(handler)

		// Publish a message with empty metadata map
		emptyMetadata := map[string]any{}
		bus.Publish("empty-metadata/topic", []byte("test-data"), emptyMetadata)

		// Wait for message to be received
		require.Eventually(t, func() bool {
			messages := handler.MessagesReceived()
			return len(messages["empty-metadata/topic"]) > 0
		}, time.Second, time.Millisecond*10, "message was not received")

		// Verify metadata is an empty map, not nil
		receivedMessage := handler.MessagesReceived()["empty-metadata/topic"][0]
		require.NotNil(t, receivedMessage.Metadata, "metadata should not be nil for empty map")
		require.Empty(t, receivedMessage.Metadata, "metadata should be an empty map")
	})
}
