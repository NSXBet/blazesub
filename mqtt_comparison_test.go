package blazesub_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/NSXBet/blazesub"
	mochimqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/packets"
	"github.com/stretchr/testify/require"
)

type mockMsgHandler struct {
	messageReceived bool
	mutex           sync.Mutex
}

func (m *mockMsgHandler) OnMessage(_ *blazesub.Message) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.messageReceived = true

	return nil
}

// BenchmarkBusVsMQTT compares the performance of the blazesub Bus vs embedded MQTT server.
func BenchmarkBusVsMQTT(b *testing.B) {
	// Common benchmark parameters
	const (
		numTopics        = 1000
		numSubscriptions = 5000
	)

	exactTopics := make([]string, numTopics)
	for i := 0; i < numTopics; i++ {
		exactTopics[i] = fmt.Sprintf("test/topic/%d/exact", i)
	}

	// Generate some wildcard topics
	wildcardTopics := []string{
		"test/+/0/level",
		"test/topic/+/level",
		"test/topic/0/#",
		"test/#",
		"+/topic/10/level",
	}

	// Mix of topics to publish to (80% exact, 20% wildcard matches)
	publishTopics := make([]string, 100)
	for i := range publishTopics {
		if i < 80 {
			// Exact match
			publishTopics[i] = exactTopics[i%numTopics]
		} else {
			// Topic that matches wildcards
			publishTopics[i] = fmt.Sprintf("test/wildcard/%d/level", i%20)
		}
	}

	// Payload for publishing
	payload := []byte("test message payload")

	// Run benchmark for BlazeSub Bus
	b.Run("BlazeSub", func(b *testing.B) {
		// Create a new bus
		bus, err := blazesub.NewBusWithDefaults()
		require.NoError(b, err)
		defer func() {
			require.NoError(b, bus.Close())
		}()

		// Create subscriptions - exact matches
		for i := 0; i < numSubscriptions-5; i++ {
			topicIndex := i % numTopics
			topic := exactTopics[topicIndex]
			sub, err := bus.Subscribe(topic)
			require.NoError(b, err)

			// Set a handler
			handler := &mockMsgHandler{
				messageReceived: false,
				mutex:           sync.Mutex{},
			}
			sub.OnMessage(handler)
		}

		// Add some wildcard subscriptions
		for _, wildcardTopic := range wildcardTopics {
			sub, err := bus.Subscribe(wildcardTopic)
			require.NoError(b, err)

			// Set a handler
			handler := &mockMsgHandler{
				messageReceived: false,
				mutex:           sync.Mutex{},
			}
			sub.OnMessage(handler)
		}

		// Give a moment for subscriptions to settle
		time.Sleep(100 * time.Millisecond)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			topic := publishTopics[i%len(publishTopics)]
			bus.Publish(topic, payload)
		}
	})

	// Run benchmark for MQTT Server
	b.Run("MQTT", func(b *testing.B) {
		// Create a new MQTT server
		mqttServer := RunMQTTServer(b)
		noop := func(cl *mochimqtt.Client, sub packets.Subscription, pk packets.Packet) {}

		// Create subscriptions - exact matches
		for i := 0; i < numSubscriptions-5; i++ {
			topicIndex := i % numTopics
			topic := exactTopics[topicIndex]

			err := mqttServer.Subscribe(topic, i, noop)
			require.NoError(b, err)
		}

		// Add some wildcard subscriptions
		for i, wildcardTopic := range wildcardTopics {
			err := mqttServer.Subscribe(wildcardTopic, numSubscriptions+i-5, noop)
			require.NoError(b, err)
		}

		// Give a moment for subscriptions to settle
		time.Sleep(100 * time.Millisecond)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			topic := publishTopics[i%len(publishTopics)]

			// Publish directly using the server
			err := mqttServer.Publish(topic, payload, false, 0)
			if err != nil {
				b.Fatalf("Failed to publish: %v", err)
			}
		}
	})
}

// BenchmarkBusVsMQTTConcurrent compares the performance of blazesub Bus vs MQTT server under concurrent load.
func BenchmarkBusVsMQTTConcurrent(b *testing.B) {
	// Common benchmark parameters
	const numTopics = 100

	// Create topics
	topics := make([]string, numTopics)
	for i := 0; i < numTopics; i++ {
		topics[i] = fmt.Sprintf("test/concurrent/%d", i)
	}

	// Payload for publishing
	payload := []byte("test message payload")

	// Benchmark for BlazeSub Bus
	b.Run("BlazeSub_Concurrent", func(b *testing.B) {
		// Create a new bus
		bus, err := blazesub.NewBusWithDefaults()
		require.NoError(b, err)
		defer func() {
			require.NoError(b, bus.Close())
		}()

		// Create subscriptions
		for _, topic := range topics {
			sub, err := bus.Subscribe(topic)
			require.NoError(b, err)

			// Set a handler
			handler := &mockMsgHandler{
				messageReceived: false,
				mutex:           sync.Mutex{},
			}
			sub.OnMessage(handler)
		}

		b.ResetTimer()

		b.RunParallel(func(pb *testing.PB) {
			counter := 0
			for pb.Next() {
				topic := topics[counter%numTopics]
				bus.Publish(topic, payload)
				counter++
			}
		})
	})

	// Benchmark for MQTT Server
	b.Run("MQTT_Concurrent", func(b *testing.B) {
		// Create a new MQTT server
		mqttServer := RunMQTTServer(b)
		noop := func(cl *mochimqtt.Client, sub packets.Subscription, pk packets.Packet) {}

		// Create subscriptions
		for _, topic := range topics {
			err := mqttServer.Subscribe(topic, 0, noop)
			require.NoError(b, err)
		}

		// Give a moment for subscriptions to settle
		time.Sleep(100 * time.Millisecond)

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			counter := 0
			for pb.Next() {
				topic := topics[counter%numTopics]

				// Use the server to publish
				err := mqttServer.Publish(topic, payload, false, 0)
				if err != nil {
					b.Fatalf("Failed to publish: %v", err)
				}

				counter++
			}
		})
	})
}

// BenchmarkBusVsMQTTSubscribeUnsubscribe compares the performance of subscribe/unsubscribe operations.
func BenchmarkBusVsMQTTSubscribeUnsubscribe(b *testing.B) {
	// Benchmark for BlazeSub Bus
	b.Run("BlazeSub_SubscribeUnsubscribe", func(b *testing.B) {
		// Create a new bus
		bus, err := blazesub.NewBusWithDefaults()
		require.NoError(b, err)
		defer func() {
			require.NoError(b, bus.Close())
		}()

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			topic := fmt.Sprintf("test/sub/%d", i)

			// Subscribe
			sub, err := bus.Subscribe(topic)
			if err != nil {
				b.Fatalf("Failed to subscribe: %v", err)
			}

			// Set handler
			handler := &mockMsgHandler{
				messageReceived: false,
				mutex:           sync.Mutex{},
			}
			sub.OnMessage(handler)

			// Unsubscribe
			err = sub.Unsubscribe()
			if err != nil {
				b.Fatalf("Failed to unsubscribe: %v", err)
			}
		}
	})

	// Benchmark for MQTT Server
	b.Run("MQTT_SubscribeUnsubscribe", func(b *testing.B) {
		// Create a new MQTT server
		mqttServer := RunMQTTServer(b)
		noop := func(cl *mochimqtt.Client, sub packets.Subscription, pk packets.Packet) {}

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			topic := fmt.Sprintf("test/sub/%d", i)

			// Subscribe
			err := mqttServer.Subscribe(topic, i, noop)
			if err != nil {
				b.Fatalf("Failed to subscribe: %v", err)
			}

			// Unsubscribe
			err = mqttServer.Unsubscribe(topic, i)
			if err != nil {
				b.Fatalf("Failed to unsubscribe: %v", err)
			}
		}
	})
}
