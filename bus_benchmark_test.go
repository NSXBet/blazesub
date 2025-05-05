package blazesub_test

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NSXBet/blazesub"
	"github.com/stretchr/testify/require"
)

// using atomic operations to be thread-safe without locking.
type NoOpAtomicHandler struct {
	count atomic.Int64
}

func (h *NoOpAtomicHandler) OnMessage(*blazesub.ByteMessage) error {
	h.count.Add(1)
	return nil
}

// BenchmarkPoolVsGoroutines compares worker pool against direct goroutines.
func BenchmarkPoolVsGoroutines(b *testing.B) {
	benchmarkCases := []struct {
		name           string
		subscriptions  int
		useWorkerPool  bool
		batchThreshold int
	}{
		{"SmallLoad_WorkerPool", 10, true, 10},
		{"SmallLoad_Goroutines", 10, false, 10},
		{"MediumLoad_WorkerPool", 100, true, 10},
		{"MediumLoad_Goroutines", 100, false, 10},
		{"LargeLoad_WorkerPool", 1000, true, 10},
		{"LargeLoad_Goroutines", 1000, false, 10},
		{"LargeLoad_BatchThreshold5_WorkerPool", 1000, true, 5},
		{"LargeLoad_BatchThreshold5_Goroutines", 1000, false, 5},
		{"LargeLoad_BatchThreshold20_WorkerPool", 1000, true, 20},
		{"LargeLoad_BatchThreshold20_Goroutines", 1000, false, 20},
	}

	for _, bc := range benchmarkCases {
		b.Run(bc.name, func(b *testing.B) {
			// Configure the bus based on the benchmark case
			config := blazesub.Config{
				WorkerCount:                20000,
				PreAlloc:                   true,
				MaxBlockingTasks:           100000,
				MaxConcurrentSubscriptions: bc.batchThreshold,
				UseGoroutinePool:           bc.useWorkerPool,
			}

			bus, err := blazesub.NewBus(config)
			require.NoError(b, err)
			defer bus.Close()

			// Create a handler and subscriptions
			handler := &NoOpAtomicHandler{}
			topics := map[string]struct{}{}

			// Create several subscriptions with some duplication
			// to simulate real-world scenarios
			for i := range bc.subscriptions {
				topic := fmt.Sprintf("test/topic/%d", i%100) // 100 unique topics max
				topics[topic] = struct{}{}

				subscription, serr := bus.Subscribe(topic)
				require.NoError(b, serr)
				subscription.OnMessage(handler)
			}

			// Create list of topics for publishing
			topicList := make([]string, 0, len(topics))
			for topic := range topics {
				topicList = append(topicList, topic)
			}

			payload := []byte("test-payload")
			topicCount := len(topicList)

			// Reset the timer for accurate measurement
			b.ResetTimer()
			b.ReportAllocs()

			for i := range b.N {
				// Cycle through topics
				topic := topicList[i%topicCount]
				bus.Publish(topic, payload)
			}

			// Wait a bit for message handlers to finish
			// This is necessary because we're using goroutines
			b.StopTimer()
			time.Sleep(100 * time.Millisecond)
		})
	}
}

// BenchmarkConcurrentPoolVsGoroutines tests concurrent publishing performance.
func BenchmarkConcurrentPoolVsGoroutines(b *testing.B) {
	benchmarkCases := []struct {
		name          string
		subscriptions int
		useWorkerPool bool
	}{
		{"MediumLoad_WorkerPool", 100, true},
		{"MediumLoad_Goroutines", 100, false},
		{"LargeLoad_WorkerPool", 1000, true},
		{"LargeLoad_Goroutines", 1000, false},
	}

	for _, bc := range benchmarkCases {
		b.Run(bc.name, func(b *testing.B) {
			// Configure the bus
			config := blazesub.Config{
				WorkerCount:                20000,
				PreAlloc:                   true,
				MaxBlockingTasks:           100000,
				MaxConcurrentSubscriptions: 10,
				UseGoroutinePool:           bc.useWorkerPool,
			}

			bus, err := blazesub.NewBus(config)
			require.NoError(b, err)
			defer bus.Close()

			// Create a handler and subscriptions
			handler := &NoOpAtomicHandler{}

			// Create several topics
			topics := make([]string, 100)
			for i := range topics {
				topics[i] = fmt.Sprintf("test/concurrent/%d", i)
			}

			// Subscribe to all topics
			for i := range bc.subscriptions {
				topicIndex := i % len(topics)
				subscription, serr := bus.Subscribe(topics[topicIndex])
				require.NoError(b, serr)
				subscription.OnMessage(handler)
			}

			payload := []byte("test-payload")

			// Reset the timer for accurate measurement
			b.ResetTimer()
			b.ReportAllocs()

			// Use RunParallel for concurrent load testing
			b.RunParallel(func(pb *testing.PB) {
				counter := 0
				for pb.Next() {
					topicIndex := counter % len(topics)
					bus.Publish(topics[topicIndex], payload)

					counter++
				}
			})

			// Wait a bit for message handlers to finish
			b.StopTimer()
			time.Sleep(100 * time.Millisecond)
		})
	}
}

// BenchmarkLatencyPoolVsGoroutines measures the latency of message delivery.
func BenchmarkLatencyPoolVsGoroutines(b *testing.B) {
	benchmarkCases := []struct {
		name          string
		useWorkerPool bool
	}{
		{"WorkerPool", true},
		{"Goroutines", false},
	}

	for _, bc := range benchmarkCases {
		b.Run(bc.name, func(b *testing.B) {
			// Configure the bus
			config := blazesub.Config{
				WorkerCount:                20000,
				PreAlloc:                   true,
				MaxBlockingTasks:           100000,
				MaxConcurrentSubscriptions: 10,
				UseGoroutinePool:           bc.useWorkerPool,
			}

			bus, err := blazesub.NewBus(config)
			require.NoError(b, err)
			defer bus.Close()

			// Channel to signal message receipt
			done := make(chan struct{}, 1)

			// Create a specialized handler that signals when a message is received
			latencyMeasureHandler := &LatencyMeasureHandler[[]byte]{
				done: done,
			}

			// Subscribe to a topic
			subscription, err := bus.Subscribe("test/latency")
			require.NoError(b, err)
			subscription.OnMessage(latencyMeasureHandler)

			payload := []byte("test-payload")

			// Reset the timer for accurate measurement
			b.ResetTimer()
			b.ReportAllocs()

			for range b.N {
				// Start timing
				start := time.Now()

				// Publish the message
				bus.Publish("test/latency", payload)

				// Wait for message to be received
				<-done

				// Record the latency
				elapsed := time.Since(start)
				b.ReportMetric(float64(elapsed.Nanoseconds())/1e3, "µs/op") // Report in microseconds
			}
		})
	}
}
