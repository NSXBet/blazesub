package blazesub_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/NSXBet/blazesub"
	"github.com/stretchr/testify/require"
)

type PubsubTestPair struct {
	topic   string
	payload []byte
}

func ptp(t *testing.T, values ...any) []PubsubTestPair {
	if len(values)%2 != 0 {
		t.Fatalf("values must be a list of topic and payload pairs")
	}

	ptp := []PubsubTestPair{}
	for i := 0; i < len(values); i += 2 {
		ptp = append(ptp, PubsubTestPair{
			topic:   values[i].(string),
			payload: values[i+1].([]byte),
		})
	}

	return ptp
}

func sub(t *testing.T, topics ...string) []string {
	t.Helper()

	if len(topics) == 0 {
		t.Fatalf("topics must be provided")
	}

	return topics
}

type want map[string][][]byte

type PubsubTestCase struct {
	name      string
	publish   []PubsubTestPair
	subscribe []string
	want      want
}

func TestCanPublishAndSubscribe(t *testing.T) {
	cases := []PubsubTestCase{
		{
			name:      "can publish and subscribe to a single topic",
			publish:   ptp(t, "test", []byte("test-data")),
			subscribe: sub(t, "test"),
			want:      want{"test": {[]byte("test-data")}},
		},
		{
			name: "can publish and subscribe to multiple topics",
			publish: ptp(
				t,
				"test", []byte("test-data"),
				"test2", []byte("test-data2"),
			),
			subscribe: sub(t, "test", "test2"),
			want: want{
				"test":  {[]byte("test-data")},
				"test2": {[]byte("test-data2")},
			},
		},
		{
			name: "can publish and subscribe to topics with slashes",
			publish: ptp(
				t,
				"test/1", []byte("test-data"),
				"test/2", []byte("test-data2"),
			),
			subscribe: sub(t, "test/1", "test/2"),
			want: want{
				"test/1": {[]byte("test-data")},
				"test/2": {[]byte("test-data2")},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bus, err := blazesub.NewBusWithDefaults()
			require.NoError(t, err)

			messageHandler := SpyMessageHandler(t)
			for _, topic := range c.subscribe {
				subscription, err := bus.Subscribe(topic)
				require.NoError(t, err)
				require.NotNil(t, subscription)

				subscription.OnMessage(messageHandler)
			}

			for _, pair := range c.publish {
				bus.Publish(pair.topic, pair.payload)
			}

			require.Eventually(t, func() bool {
				return len(messageHandler.MessagesReceived()) == len(c.want)
			}, time.Second, time.Millisecond*10)

			for _, pair := range c.publish {
				require.Len(
					t,
					messageHandler.MessagesReceived()[pair.topic],
					len(c.want[pair.topic]),
				)
				for i, payload := range c.want[pair.topic] {
					require.Equal(
						t,
						payload,
						messageHandler.MessagesReceived()[pair.topic][i].Data,
					)
				}
			}
		})
	}
}

func TestSlowMessageHandler(t *testing.T) {
	bus, err := blazesub.NewBusWithDefaults()
	require.NoError(t, err)

	slowHandler := SpyMessageHandler(t)
	slowHandler.SetDelay(50 * time.Millisecond)

	subscription, err := bus.Subscribe("test")
	require.NoError(t, err)
	require.NotNil(t, subscription)
	subscription.OnMessage(slowHandler)

	fastHandler := SpyMessageHandler(t)

	subscription2, err := bus.Subscribe("test")
	require.NoError(t, err)
	require.NotNil(t, subscription2)
	subscription2.OnMessage(fastHandler)

	bus.Publish("test", []byte("test-data"))

	require.Eventually(t, func() bool {
		return len(fastHandler.MessagesReceived()["test"]) == 1
	}, time.Millisecond*10, time.Millisecond*1)

	require.Equal(t, []byte("test-data"), fastHandler.MessagesReceived()["test"][0].Data)

	// Ensure that closing waits for all messages to be processed.
	bus.Close()

	require.Eventually(t, func() bool {
		return len(slowHandler.MessagesReceived()["test"]) == 1
	}, time.Second, time.Millisecond*10)

	require.Equal(t, 1, len(slowHandler.MessagesReceived()["test"]))
	require.Equal(t, []byte("test-data"), slowHandler.MessagesReceived()["test"][0].Data)
}

func BenchmarkPublishAndSubscribe(b *testing.B) {
	const (
		topics      = 10
		subscribers = 100
	)

	bus, err := blazesub.NewBus(
		blazesub.Config{
			WorkerCount:      10000,
			MaxBlockingTasks: 1000000,
			PreAlloc:         true,
		},
	)
	require.NoError(b, err)

	payload := []byte("test-data")

	messageHandler := NoOpHandler(b)

	for i := range subscribers {
		topic := fmt.Sprintf("test/%d", i%topics)
		subs, err := bus.Subscribe(topic)
		require.NoError(b, err)

		subs.OnMessage(messageHandler)
	}

	publishedMessages := 0

	for b.Loop() {
		publishedMessages++

		topic := fmt.Sprintf("test/%d", publishedMessages%topics)
		bus.Publish(topic, payload)
	}

	bus.Close()

	expectedMessages := publishedMessages * (subscribers / topics)

	require.Eventually(
		b,
		func() bool {
			return int(messageHandler.MessageCount.Load()) == expectedMessages
		},
		time.Second*3,
		time.Millisecond*10,
		"expected %d messages to be published, but got %d",
		expectedMessages,
		messageHandler.MessageCount.Load(),
	)
}

func BenchmarkPublishAndSubscribeWithWildcards_To_Multiple_Subscribers(b *testing.B) {
	subscribersTestCases := []int{10, 100, 1000}

	payload := []byte("test-data")

	for _, subscribers := range subscribersTestCases {
		b.Run(fmt.Sprintf("subscriberCount=%d", subscribers), func(b *testing.B) {
			bus, err := blazesub.NewBusWithDefaults()
			require.NoError(b, err)

			messageHandler := NoOpHandler(b)

			expectedMessages := int64(0)

			for range subscribers {
				subs, err := bus.Subscribe("test/+")
				require.NoError(b, err)

				subs.OnMessage(messageHandler)

				subs2, err := bus.Subscribe("test/#")
				require.NoError(b, err)

				subs2.OnMessage(messageHandler)
			}

			for b.Loop() {
				expectedMessages += int64(subscribers) * 2

				bus.Publish("test/something", payload)
			}

			bus.Close()

			require.Eventually(b, func() bool {
				return messageHandler.MessageCount.Load() == expectedMessages
			}, time.Second*3, time.Millisecond*10)
		})
	}
}

// TestBusUnsubscribe tests that unsubscribing from the bus properly removes subscriptions
func TestBusUnsubscribe(t *testing.T) {
	bus, err := blazesub.NewBusWithDefaults()
	require.NoError(t, err)

	// Create a subscription
	subscription, err := bus.Subscribe("test/topic")
	require.NoError(t, err)
	require.NotNil(t, subscription)

	// Add a handler to ensure it can receive messages
	handler := SpyMessageHandler(t)
	subscription.OnMessage(handler)

	// Publish a message to confirm it works
	bus.Publish("test/topic", []byte("test-data"))

	require.Eventually(t, func() bool {
		return len(handler.MessagesReceived()["test/topic"]) == 1
	}, time.Second, time.Millisecond*10)

	// Unsubscribe
	err = subscription.Unsubscribe()
	require.NoError(t, err)

	// Publish again to confirm unsubscription worked
	bus.Publish("test/topic", []byte("test-data-after-unsub"))

	// Wait a bit to make sure no new messages are received
	time.Sleep(time.Millisecond * 50)

	// Should still have only the first message
	require.Len(t, handler.MessagesReceived()["test/topic"], 1)
	require.Equal(t, []byte("test-data"), handler.MessagesReceived()["test/topic"][0].Data)
}

// TestBusMultipleUnsubscribe tests unsubscribing multiple subscriptions
func TestBusMultipleUnsubscribe(t *testing.T) {
	bus, err := blazesub.NewBusWithDefaults()
	require.NoError(t, err)

	// Create several subscriptions
	handler := SpyMessageHandler(t)
	var subscriptions []*blazesub.Subscription
	for i := 0; i < 5; i++ {
		topic := fmt.Sprintf("test/topic/%d", i)
		subscription, err := bus.Subscribe(topic)
		require.NoError(t, err)
		subscription.OnMessage(handler)
		subscriptions = append(subscriptions, subscription)
	}

	// Publish to all topics
	for i := 0; i < 5; i++ {
		topic := fmt.Sprintf("test/topic/%d", i)
		bus.Publish(topic, []byte(fmt.Sprintf("data-%d", i)))
	}

	// Wait for all messages
	require.Eventually(t, func() bool {
		count := 0
		for i := 0; i < 5; i++ {
			topic := fmt.Sprintf("test/topic/%d", i)
			count += len(handler.MessagesReceived()[topic])
		}
		return count == 5
	}, time.Second, time.Millisecond*10)

	// Unsubscribe from odd-numbered topics
	for i := 1; i < 5; i += 2 {
		err := subscriptions[i].Unsubscribe()
		require.NoError(t, err)
	}

	// Reset the handler
	handler.Reset()

	// Publish to all topics again
	for i := 0; i < 5; i++ {
		topic := fmt.Sprintf("test/topic/%d", i)
		bus.Publish(topic, []byte(fmt.Sprintf("data-%d-after", i)))
	}

	// Wait a bit for messages to be processed
	time.Sleep(time.Millisecond * 50)

	// Check that we only received messages for even-numbered topics (0, 2, 4)
	for i := 0; i < 5; i++ {
		topic := fmt.Sprintf("test/topic/%d", i)
		if i%2 == 0 { // Even topics should have messages
			require.Eventually(t, func() bool {
				return len(handler.MessagesReceived()[topic]) == 1
			}, time.Second, time.Millisecond*10)
			require.Equal(t, []byte(fmt.Sprintf("data-%d-after", i)), handler.MessagesReceived()[topic][0].Data)
		} else { // Odd topics should have no messages
			require.Empty(t, handler.MessagesReceived()[topic])
		}
	}
}

// TestBusWildcardUnsubscribe tests unsubscribing from wildcard topics
func TestBusWildcardUnsubscribe(t *testing.T) {
	bus, err := blazesub.NewBusWithDefaults()
	require.NoError(t, err)

	// Create wildcard subscriptions
	handler1 := SpyMessageHandler(t)
	handler2 := SpyMessageHandler(t)

	// Single-level wildcard
	subscription1, err := bus.Subscribe("test/+/topic")
	require.NoError(t, err)
	subscription1.OnMessage(handler1)

	// Multi-level wildcard
	subscription2, err := bus.Subscribe("test/#")
	require.NoError(t, err)
	subscription2.OnMessage(handler2)

	// Publish messages that match both wildcards
	bus.Publish("test/any/topic", []byte("matches-plus"))
	bus.Publish("test/deep/nested/topic", []byte("matches-hash"))

	// Wait for both handlers to receive their messages
	require.Eventually(t, func() bool {
		return len(handler1.MessagesReceived()["test/any/topic"]) == 1
	}, time.Second, time.Millisecond*10)
	require.Eventually(t, func() bool {
		return len(handler2.MessagesReceived()["test/any/topic"]) == 1 &&
			len(handler2.MessagesReceived()["test/deep/nested/topic"]) == 1
	}, time.Second, time.Millisecond*10)

	// Unsubscribe from the single-level wildcard
	err = subscription1.Unsubscribe()
	require.NoError(t, err)

	// Reset handlers
	handler1.Reset()
	handler2.Reset()

	// Publish the same messages again
	bus.Publish("test/any/topic", []byte("after-unsub-plus"))
	bus.Publish("test/deep/nested/topic", []byte("after-unsub-hash"))

	// Wait a bit
	time.Sleep(time.Millisecond * 50)

	// Handler1 should receive no messages
	require.Empty(t, handler1.MessagesReceived()["test/any/topic"])

	// Handler2 should still receive both messages
	require.Eventually(t, func() bool {
		return len(handler2.MessagesReceived()["test/any/topic"]) == 1 &&
			len(handler2.MessagesReceived()["test/deep/nested/topic"]) == 1
	}, time.Second, time.Millisecond*10)
	require.Equal(t, []byte("after-unsub-plus"), handler2.MessagesReceived()["test/any/topic"][0].Data)
	require.Equal(t, []byte("after-unsub-hash"), handler2.MessagesReceived()["test/deep/nested/topic"][0].Data)

	// Unsubscribe from the multi-level wildcard
	err = subscription2.Unsubscribe()
	require.NoError(t, err)

	// Reset handlers again
	handler1.Reset()
	handler2.Reset()

	// Publish again
	bus.Publish("test/any/topic", []byte("final-message"))
	bus.Publish("test/deep/nested/topic", []byte("final-message"))

	// Wait a bit
	time.Sleep(time.Millisecond * 50)

	// Both handlers should receive no messages
	require.Empty(t, handler1.MessagesReceived())
	require.Empty(t, handler2.MessagesReceived())
}

// TestBusUnsubscribePerformance tests the performance impact of frequent subscribe/unsubscribe operations
func TestBusUnsubscribePerformance(t *testing.T) {
	bus, err := blazesub.NewBusWithDefaults()
	require.NoError(t, err)

	// Create a simple message handler
	handler := SpyMessageHandler(t)

	// Measure time to create 1000 subscriptions
	startSubscribe := time.Now()
	var subscriptions []*blazesub.Subscription

	for i := 0; i < 1000; i++ {
		topic := fmt.Sprintf("test/topic/%d", i%100) // 100 unique topics
		sub, err := bus.Subscribe(topic)
		require.NoError(t, err)
		sub.OnMessage(handler)
		subscriptions = append(subscriptions, sub)
	}

	subscribeTime := time.Since(startSubscribe)

	// Measure time to unsubscribe from all subscriptions
	startUnsubscribe := time.Now()
	for _, sub := range subscriptions {
		err := sub.Unsubscribe()
		require.NoError(t, err)
	}
	unsubscribeTime := time.Since(startUnsubscribe)

	// Resubscribe to the same topics
	startResubscribe := time.Now()
	for i := 0; i < 1000; i++ {
		topic := fmt.Sprintf("test/topic/%d", i%100)
		sub, err := bus.Subscribe(topic)
		require.NoError(t, err)
		sub.OnMessage(handler)
	}
	resubscribeTime := time.Since(startResubscribe)

	// Log performance metrics
	t.Logf("Subscribe time for 1000 topics: %v", subscribeTime)
	t.Logf("Unsubscribe time for 1000 topics: %v", unsubscribeTime)
	t.Logf("Resubscribe time for 1000 topics: %v", resubscribeTime)

	// Simple sanity check that resubscribe doesn't take significantly longer
	// than the initial subscribe (which would indicate memory fragmentation issues)
	if resubscribeTime > subscribeTime*2 {
		t.Errorf("Resubscribe time (%v) is significantly longer than initial subscribe time (%v)",
			resubscribeTime, subscribeTime)
	}
}
