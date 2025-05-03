package blazesub

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestBusWithRegistryPool tests the Bus with the integrated RegistryPool
func TestBusWithRegistryPool(t *testing.T) {
	// Create a bus with a registry pool
	config := NewConfig()
	config.RegistryPoolSize = 10 // Use 4 registries

	bus, err := NewBus(config)
	require.NoError(t, err)
	defer bus.Close()

	// Subscribe to several topics
	const numSubscriptions = 1000
	var receivedCounts [numSubscriptions]int32
	var subscriptions [numSubscriptions]*Subscription

	for i := 0; i < numSubscriptions; i++ {
		// Create a mix of exact and wildcard subscriptions
		var topic string
		if i%3 == 0 {
			topic = "bustest/+"
		} else if i%5 == 0 {
			topic = "bustest/#"
		} else {
			topic = fmt.Sprintf("bustest/topic%d", i)
		}

		sub, err := bus.Subscribe(topic)
		require.NoError(t, err)

		// Set up handler to count received messages
		index := i // Capture i for the closure
		sub.OnMessage(HandlerFunc(func(msg *Message) error {
			atomic.AddInt32(&receivedCounts[index], 1)
			return nil
		}))

		subscriptions[i] = sub
	}

	// Publish messages to different topics
	for i := 0; i < 1000; i++ {
		bus.Publish(fmt.Sprintf("bustest/topic%d", i), []byte("hello"))
	}

	// Give some time for processing
	time.Sleep(100 * time.Millisecond)

	// Verify some messages were received by the appropriate subscriptions
	// Exact matches should receive messages
	require.Greater(t, atomic.LoadInt32(&receivedCounts[1]), int32(0),
		"Subscription for exact topic should have received messages")

	// Wildcard subscriptions should receive messages too
	require.Greater(t, atomic.LoadInt32(&receivedCounts[0]), int32(0),
		"Subscription with + wildcard should have received messages")
	require.Greater(t, atomic.LoadInt32(&receivedCounts[5]), int32(0),
		"Subscription with # wildcard should have received messages")

	// Test unsubscribe
	require.NoError(t, subscriptions[1].Unsubscribe())

	// Clear counts
	for i := range receivedCounts {
		atomic.StoreInt32(&receivedCounts[i], 0)
	}

	// Publish again
	bus.Publish("bustest/topicA", []byte("after unsubscribe"))

	// Give some time for processing
	time.Sleep(100 * time.Millisecond)

	// The unsubscribed subscription should not receive messages
	require.Equal(t, int32(0), atomic.LoadInt32(&receivedCounts[1]),
		"Unsubscribed subscription should not receive messages")
}

// TestBusParallelPublish tests parallel publishing with the Bus and RegistryPool
func TestBusParallelPublish(t *testing.T) {
	t.Skip("skipping test in short mode")
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	// Create a bus with a registry pool
	config := NewConfig()
	config.RegistryPoolSize = 4

	bus, err := NewBus(config)
	require.NoError(t, err)
	defer bus.Close()

	runTestBus(t, bus, "Parallel")
}

func runTestBus(t *testing.T, bus *Bus, name string) {
	// Publish messages in parallel
	const (
		numPublishers  = 50
		messagesPerPub = 1000
	)

	// Subscribe to topics with wildcards to test matching
	var receivedCount int64
	var messagesMutex sync.Mutex
	receivedMessages := make(map[string]int)

	for i := 0; i < messagesPerPub; i++ {
		filter := fmt.Sprintf("parallel/topic%d/test", i)
		if i%3 == 0 {
			filter = "parallel/+/test"
		} else if i%5 == 0 {
			filter = "parallel/#"
		}
		sub, err := bus.Subscribe(filter)
		require.NoError(t, err)

		sub.OnMessage(HandlerFunc(func(msg *Message) error {
			atomic.AddInt64(&receivedCount, 1)
			messagesMutex.Lock()
			receivedMessages[msg.Topic]++
			messagesMutex.Unlock()
			return nil
		}))
	}

	var wg sync.WaitGroup
	wg.Add(numPublishers)

	start := time.Now()
	for i := 0; i < numPublishers; i++ {
		go func(pubID int) {
			defer wg.Done()
			for j := 0; j < messagesPerPub; j++ {
				topic := fmt.Sprintf("parallel/topic%d/test", pubID)
				bus.Publish(topic, []byte("parallel test"))
			}
		}(i)
	}

	wg.Wait()

	// Give some time for processing to complete
	time.Sleep(200 * time.Millisecond)

	duration := time.Since(start)
	totalMessages := numPublishers * messagesPerPub

	// All messages should have been received
	receivedTotal := atomic.LoadInt64(&receivedCount)

	t.Logf("[%s] Published %d messages in %v", name, totalMessages, duration)
	t.Logf("[%s] Received %d messages", name, receivedTotal)
	t.Logf("[%s] Throughput: %.2f msgs/sec", name, float64(totalMessages)/duration.Seconds())

	// Verify all topics received messages
	messagesMutex.Lock()
	numTopics := len(receivedMessages)
	messagesMutex.Unlock()

	require.Equal(t, numPublishers, numTopics,
		"Should have received messages for all topics")
}

// Add additional test for sequential vs parallel execution
func TestSequentialVsParallelExecution(t *testing.T) {
	// Create two buses with different config
	configSequential := NewConfig()
	configSequential.RegistryPoolSize = 1

	configParallel := NewConfig()
	configParallel.RegistryPoolSize = 8

	buses := []*Bus{}
	busNames := []string{"Sequential", "Parallel"}

	bus1, err := NewBus(configSequential)
	require.NoError(t, err)
	defer bus1.Close()
	buses = append(buses, bus1)

	bus2, err := NewBus(configParallel)
	require.NoError(t, err)
	defer bus2.Close()
	buses = append(buses, bus2)

	for i, bus := range buses {
		t.Run(busNames[i], func(t *testing.T) {
			runTestBus(t, bus, busNames[i])
		})
	}
}

// HandlerFunc is a helper type to implement the MessageHandler interface
type HandlerFunc func(message *Message) error

func (h HandlerFunc) OnMessage(message *Message) error {
	return h(message)
}
