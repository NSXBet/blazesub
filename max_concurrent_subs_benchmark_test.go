package blazesub_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/NSXBet/blazesub"
	"github.com/stretchr/testify/require"
)

// BenchmarkMaxConcurrentSubscriptions tests the impact of the MaxConcurrentSubscriptions config
// parameter on performance for different subscriber counts.
func BenchmarkMaxConcurrentSubscriptions(b *testing.B) {
	// Test with worker pool and direct goroutines
	poolModes := []struct {
		name          string
		useWorkerPool bool
	}{
		{"WorkerPool", true},
		{"DirectGoroutines", false},
	}

	// Test different subscription loads
	subscriberCounts := []int{10, 100, 500, 1000, 2000, 5000}

	for _, mode := range poolModes {
		for _, subCount := range subscriberCounts {
			for _, maxConcurrent := range []int{subCount / 2, subCount, subCount * 2} {
				// Only run tests where maxConcurrent makes sense for the subscriber count
				// (no point testing maxConcurrent=5000 with only 10 subscribers)
				if maxConcurrent > subCount*2 {
					continue
				}

				benchName := fmt.Sprintf("%s/Subscribers=%d/MaxConcurrent=%d",
					mode.name, subCount, maxConcurrent)

				b.Run(benchName, func(b *testing.B) {
					// Configure the bus with the specific max concurrent subscriptions value
					config := blazesub.Config{
						WorkerCount:                20000,
						PreAlloc:                   true,
						MaxBlockingTasks:           100000,
						MaxConcurrentSubscriptions: maxConcurrent,
						UseGoroutinePool:           mode.useWorkerPool,
					}

					bus, err := blazesub.NewBus(config)
					require.NoError(b, err)
					defer bus.Close()

					// Create a handler that does minimal work
					handler := &NoOpAtomicHandler{}

					// Subscribe to the same topic multiple times
					// This simulates having many subscribers for a single topic
					const topic = "test/topic/with/many/subscribers"
					for range subCount {
						subscription, serr := bus.Subscribe(topic)
						require.NoError(b, serr)
						subscription.OnMessage(handler)
					}

					// Give a moment for subscriptions to settle and be cached
					time.Sleep(100 * time.Millisecond)

					// The payload to publish
					payload := []byte("test-payload")

					// Reset timer before the benchmark loop
					b.ResetTimer()
					b.ReportAllocs()

					// Run the benchmark - publish to the topic with many subscribers
					for range b.N {
						bus.Publish(topic, payload)
					}

					// Wait for any pending message deliveries to complete
					b.StopTimer()
					time.Sleep(250 * time.Millisecond)
				})
			}
		}
	}
}

// Small load benchmark for detailed analysis.
func BenchmarkMaxConcurrentSubscriptionsDetailed(b *testing.B) {
	// More granular values for detailed analysis
	subscriberCount := 1000 // Fixed at 1000 subscribers

	for _, usePool := range []bool{true, false} {
		poolMode := "DirectGoroutines"
		if usePool {
			poolMode = "WorkerPool"
		}

		for _, maxConcurrent := range []int{500, 1000, 2000} {
			benchName := fmt.Sprintf("%s/Subscribers=%d/MaxConcurrent=%d",
				poolMode, subscriberCount, maxConcurrent)

			b.Run(benchName, func(b *testing.B) {
				config := blazesub.Config{
					WorkerCount:                20000,
					PreAlloc:                   true,
					MaxBlockingTasks:           100000,
					MaxConcurrentSubscriptions: maxConcurrent,
					UseGoroutinePool:           usePool,
				}

				bus, err := blazesub.NewBus(config)
				require.NoError(b, err)
				defer bus.Close()

				handler := &NoOpAtomicHandler{}
				const topic = "test/topic/with/many/subscribers"

				for range subscriberCount {
					subscription, serr := bus.Subscribe(topic)
					require.NoError(b, serr)
					subscription.OnMessage(handler)
				}

				time.Sleep(100 * time.Millisecond)
				payload := []byte("test-payload")

				b.ResetTimer()
				b.ReportAllocs()

				for range b.N {
					bus.Publish(topic, payload)
				}

				b.StopTimer()
				time.Sleep(250 * time.Millisecond)
			})
		}
	}
}
