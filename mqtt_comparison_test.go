package blazesub_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/NSXBet/blazesub"
	mochimqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/packets"
	"github.com/stretchr/testify/require"
)

type mockMsgHandler struct{}

func (m *mockMsgHandler) OnMessage(_ *blazesub.Message[[]byte]) error {
	return nil
}

// BenchmarkBusVsMQTT compares the performance of the blazesub Bus vs embedded MQTT server.
//
//nolint:gocognit // reason: benchmark test code.
func BenchmarkBusVsMQTT(b *testing.B) {
	// Common benchmark parameters
	const (
		numTopics        = 1000
		numSubscriptions = 5000
	)

	exactTopics := make([]string, numTopics)
	for i := range numTopics {
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

	// Run benchmark for BlazeSub Bus with worker pool
	b.Run("BlazeSub_WorkerPool", func(b *testing.B) {
		// Create a new bus with worker pool
		config := blazesub.Config{
			WorkerCount:      20000,
			PreAlloc:         true,
			MaxBlockingTasks: 100000,
			UseGoroutinePool: true, // Use worker pool
		}
		bus, err := blazesub.NewBus(config)
		require.NoError(b, err)

		defer func() {
			require.NoError(b, bus.Close())
		}()

		// Create subscriptions - exact matches
		for i := range numSubscriptions - 5 {
			topicIndex := i % numTopics
			topic := exactTopics[topicIndex]
			sub, serr := bus.Subscribe(topic)
			require.NoError(b, serr)

			// Set a handler
			handler := &mockMsgHandler{}
			sub.OnMessage(handler)
		}

		// Add some wildcard subscriptions
		for _, wildcardTopic := range wildcardTopics {
			sub, serr := bus.Subscribe(wildcardTopic)
			require.NoError(b, serr)

			// Set a handler
			handler := &mockMsgHandler{}
			sub.OnMessage(handler)
		}

		// Give a moment for subscriptions to settle
		time.Sleep(100 * time.Millisecond)

		b.ResetTimer()

		for i := range b.N {
			topic := publishTopics[i%len(publishTopics)]
			bus.Publish(topic, payload)
		}
	})

	// Run benchmark for BlazeSub Bus with direct goroutines
	b.Run("BlazeSub_DirectGoroutines", func(b *testing.B) {
		// Create a new bus with direct goroutines
		config := blazesub.Config{
			WorkerCount:      20000,
			PreAlloc:         true,
			MaxBlockingTasks: 100000,
			UseGoroutinePool: false, // Use direct goroutines
		}
		bus, err := blazesub.NewBus(config)
		require.NoError(b, err)

		defer func() {
			require.NoError(b, bus.Close())
		}()

		// Create subscriptions - exact matches
		for i := range numSubscriptions - 5 {
			topicIndex := i % numTopics
			topic := exactTopics[topicIndex]
			sub, serr := bus.Subscribe(topic)
			require.NoError(b, serr)

			// Set a handler
			handler := &mockMsgHandler{}
			sub.OnMessage(handler)
		}

		// Add some wildcard subscriptions
		for _, wildcardTopic := range wildcardTopics {
			sub, serr := bus.Subscribe(wildcardTopic)
			require.NoError(b, serr)

			// Set a handler
			handler := &mockMsgHandler{}
			sub.OnMessage(handler)
		}

		// Give a moment for subscriptions to settle
		time.Sleep(100 * time.Millisecond)

		b.ResetTimer()

		for i := range b.N {
			topic := publishTopics[i%len(publishTopics)]
			bus.Publish(topic, payload)
		}
	})

	// Run benchmark for MQTT Server
	b.Run("MQTT", func(b *testing.B) {
		// Create a new MQTT server
		mqttServer := RunMQTTServer(b)
		noop := func(*mochimqtt.Client, packets.Subscription, packets.Packet) {}

		// Create subscriptions - exact matches
		for i := range numSubscriptions - 5 {
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

		for i := range b.N {
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
	for i := range numTopics {
		topics[i] = fmt.Sprintf("test/concurrent/%d", i)
	}

	// Payload for publishing
	payload := []byte("test message payload")

	// Benchmark for BlazeSub Bus with worker pool
	b.Run("BlazeSub_WorkerPool_Concurrent", func(b *testing.B) {
		// Create a new bus with worker pool
		config := blazesub.Config{
			WorkerCount:      20000,
			PreAlloc:         true,
			MaxBlockingTasks: 100000,
			UseGoroutinePool: true, // Use worker pool
		}
		bus, err := blazesub.NewBus(config)
		require.NoError(b, err)

		defer func() {
			require.NoError(b, bus.Close())
		}()

		// Create subscriptions
		for _, topic := range topics {
			sub, serr := bus.Subscribe(topic)
			require.NoError(b, serr)

			// Set a handler
			handler := &mockMsgHandler{}
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

	// Benchmark for BlazeSub Bus with direct goroutines
	b.Run("BlazeSub_DirectGoroutines_Concurrent", func(b *testing.B) {
		// Create a new bus with direct goroutines
		config := blazesub.Config{
			WorkerCount:      20000,
			PreAlloc:         true,
			MaxBlockingTasks: 100000,
			UseGoroutinePool: false, // Use direct goroutines
		}
		bus, err := blazesub.NewBus(config)
		require.NoError(b, err)

		defer func() {
			require.NoError(b, bus.Close())
		}()

		// Create subscriptions
		for _, topic := range topics {
			sub, serr := bus.Subscribe(topic)
			require.NoError(b, serr)

			// Set a handler
			handler := &mockMsgHandler{}
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
		noop := func(*mochimqtt.Client, packets.Subscription, packets.Packet) {}

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
//
//nolint:gocognit // reason: benchmark test code.
func BenchmarkBusVsMQTTSubscribeUnsubscribe(b *testing.B) {
	// Benchmark for BlazeSub Bus with worker pool
	b.Run("BlazeSub_WorkerPool_SubscribeUnsubscribe", func(b *testing.B) {
		// Create a new bus with worker pool
		config := blazesub.Config{
			WorkerCount:      20000,
			PreAlloc:         true,
			MaxBlockingTasks: 100000,
			UseGoroutinePool: true, // Use worker pool
		}
		bus, err := blazesub.NewBus(config)
		require.NoError(b, err)

		defer func() {
			require.NoError(b, bus.Close())
		}()

		b.ResetTimer()

		for i := range b.N {
			topic := fmt.Sprintf("test/sub/%d", i)

			// Subscribe
			sub, serr := bus.Subscribe(topic)
			if serr != nil {
				b.Fatalf("Failed to subscribe: %v", serr)
			}

			// Set a handler
			handler := &mockMsgHandler{}
			sub.OnMessage(handler)

			// Unsubscribe
			err = sub.Unsubscribe()
			if err != nil {
				b.Fatalf("Failed to unsubscribe: %v", err)
			}
		}
	})

	// Benchmark for BlazeSub Bus with direct goroutines
	b.Run("BlazeSub_DirectGoroutines_SubscribeUnsubscribe", func(b *testing.B) {
		// Create a new bus with direct goroutines
		config := blazesub.Config{
			WorkerCount:      20000,
			PreAlloc:         true,
			MaxBlockingTasks: 100000,
			UseGoroutinePool: false, // Use direct goroutines
		}
		bus, err := blazesub.NewBus(config)
		require.NoError(b, err)

		defer func() {
			require.NoError(b, bus.Close())
		}()

		b.ResetTimer()

		for i := range b.N {
			topic := fmt.Sprintf("test/sub/%d", i)

			// Subscribe
			sub, serr := bus.Subscribe(topic)
			if serr != nil {
				b.Fatalf("Failed to subscribe: %v", serr)
			}

			// Set a handler
			handler := &mockMsgHandler{}
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
		noop := func(*mochimqtt.Client, packets.Subscription, packets.Packet) {}

		b.ResetTimer()

		for i := range b.N {
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
