package blazesub_test

import (
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NSXBet/blazesub"
	"github.com/stretchr/testify/require"
)

// ThroughputHandler is a minimal message handler that just counts messages received.
type ThroughputHandler struct {
	count atomic.Int64
}

func (h *ThroughputHandler) OnMessage(_ *blazesub.Message[[]byte]) error {
	h.count.Add(1)
	return nil
}

// BenchmarkThroughputWith1000Subscribers measures messages per second with 1000 subscribers.
func BenchmarkThroughputWith1000Subscribers(b *testing.B) {
	tests := []struct {
		name          string
		topicType     string // "direct" or "wildcard"
		useWorkerPool bool
	}{
		{"DirectMatch_DirectGoroutines", "direct", false},
		{"DirectMatch_WorkerPool", "direct", true},
		{"WildcardMatch_DirectGoroutines", "wildcard", false},
		{"WildcardMatch_WorkerPool", "wildcard", true},
	}

	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
			// Configure optimally based on our findings
			maxConcurrent := 750
			if test.useWorkerPool {
				maxConcurrent = 5
			}

			config := blazesub.Config{
				WorkerCount:      20000,
				PreAlloc:         true,
				MaxBlockingTasks: 100000,
				UseGoroutinePool: test.useWorkerPool,
				// Set optimal MaxConcurrentSubscriptions value based on mode
				MaxConcurrentSubscriptions: maxConcurrent,
			}

			bus, err := blazesub.NewBus(config)
			require.NoError(b, err)
			defer bus.Close()

			// Create handler and counters
			handler := &ThroughputHandler{}

			// Select topic configuration based on the test case
			var subTopic, pubTopic string

			if test.topicType == "direct" {
				// Direct match test - all subscribers on same topic
				subTopic = "test/direct/match/topic"
				pubTopic = "test/direct/match/topic"
			} else {
				// Wildcard match test - wildcard subscribers that match the publish topic
				subTopic = "test/+/match/#"            // Wildcard pattern
				pubTopic = "test/wildcard/match/topic" // Will match the wildcard
			}

			// Create 1000 subscribers
			const subscriberCount = 1000
			for range subscriberCount {
				subscription, serr := bus.Subscribe(subTopic)
				require.NoError(b, serr)
				subscription.OnMessage(handler)
			}

			// Give a moment for subscriptions to settle
			time.Sleep(100 * time.Millisecond)

			// Create test message
			payload := []byte("test-payload")

			// Reset timer for the actual benchmark
			b.ResetTimer()
			b.ReportAllocs()

			// Start with a fresh count
			handler.count.Store(0)

			// The clock runs until every published message has been delivered
			// to all subscriber handlers, so the reported throughput is
			// end-to-end delivery throughput, not publish-side dispatch.
			startTime := time.Now()

			// Run the benchmark
			for range b.N {
				bus.Publish(pubTopic, payload)
			}

			// Wait until every published message reached every subscriber.
			target := int64(b.N) * subscriberCount
			deadline := time.Now().Add(60 * time.Second)
			for handler.count.Load() < target {
				if time.Now().After(deadline) {
					b.Fatalf(
						"delivery incomplete: %d/%d deliveries after 60s",
						handler.count.Load(),
						target,
					)
				}
				runtime.Gosched()
			}

			// Calculate actual throughput (message deliveries per second)
			elapsedSeconds := time.Since(startTime).Seconds()
			deliveries := handler.count.Load()
			messagesPerSecond := float64(deliveries) / elapsedSeconds

			// Report custom metrics
			b.ReportMetric(messagesPerSecond, "msg/s")

			// Also log for easy viewing
			b.Logf(
				"Throughput: %.2f msgs/sec, Total: %d deliveries in %.2f seconds",
				messagesPerSecond,
				deliveries,
				elapsedSeconds,
			)
		})
	}
}
