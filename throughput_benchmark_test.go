package blazesub_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/NSXBet/blazesub"
	"github.com/stretchr/testify/require"
)

// ThroughputHandler is a minimal message handler that just counts messages received
type ThroughputHandler struct {
	count atomic.Int64
}

func (h *ThroughputHandler) OnMessage(msg *blazesub.Message) error {
	h.count.Add(1)
	return nil
}

// BenchmarkThroughputWith1000Subscribers measures messages per second with 1000 subscribers
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
			for i := 0; i < subscriberCount; i++ {
				subscription, err := bus.Subscribe(subTopic)
				require.NoError(b, err)
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

			// Record start time for our own throughput calculation
			startTime := time.Now()

			// Run the benchmark
			for i := 0; i < b.N; i++ {
				bus.Publish(pubTopic, payload)
			}

			// Wait for message processing to complete (conservative wait time)
			time.Sleep(500 * time.Millisecond)

			// Calculate actual throughput (messages per second)
			elapsedSeconds := time.Since(startTime).Seconds()
			messagesSent := b.N
			messagesReceived := handler.count.Load()

			// Because each message goes to 1000 subscribers, we multiply by 1000
			messagesPerSecond := float64(messagesSent) * subscriberCount / elapsedSeconds

			// Report custom metrics
			b.ReportMetric(messagesPerSecond, "msg/s")
			b.ReportMetric(float64(messagesReceived)/float64(messagesSent*subscriberCount)*100, "delivery_%")

			// Also log for easy viewing
			b.Logf(
				"Throughput: %.2f msgs/sec, Total: %d sent, %d received in %.2f seconds",
				messagesPerSecond,
				messagesSent*subscriberCount,
				messagesReceived,
				elapsedSeconds,
			)
		})
	}
}
