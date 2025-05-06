package blazesub_test

import (
	"fmt"
	"testing"

	"github.com/NSXBet/blazesub"
	"github.com/stretchr/testify/require"
)

// mockMsgHandler2 is a mock message handler for benchmark testing.
type mockMsgHandler2 struct{}

func (m *mockMsgHandler2) OnMessage(_ *blazesub.Message[[]byte]) error {
	return nil
}

// BenchmarkSubscriptionOps benchmarks various subscription operations.
func BenchmarkSubscriptionOps(b *testing.B) {
	// Test different topic patterns for variety
	patterns := []string{
		"simple/topic",
		"wildcard/+/topic",
		"deep/nested/topic/structure",
		"deep/+/topic/#",
		"very/very/deep/nested/structure/with/many/segments",
		"#",
	}

	// Benchmark subscribing and immediate unsubscribing
	b.Run("SubscribeUnsubscribe", func(b *testing.B) {
		config := blazesub.Config{
			UseGoroutinePool: false,
		}
		bus, err := blazesub.NewBus(config)
		require.NoError(b, err)

		defer bus.Close()

		b.ResetTimer()

		for i := range b.N {
			pattern := patterns[i%len(patterns)]
			sub, serr := bus.Subscribe(pattern)
			require.NoError(b, serr)

			handler := &mockMsgHandler2{}
			sub.OnMessage(handler)

			require.NoError(b, sub.Unsubscribe())
		}
	})

	// Benchmark subscribing many at once then unsubscribing all
	b.Run("ManySubscribeThenUnsubscribe", func(b *testing.B) {
		b.StopTimer()

		config := blazesub.Config{
			UseGoroutinePool: false,
		}
		bus, err := blazesub.NewBus(config)
		require.NoError(b, err)

		defer bus.Close()

		const subscriptionsPerIteration = 1000

		b.StartTimer()

		for range b.N {
			b.StopTimer()
			// Create many subscriptions
			subs := make([]*blazesub.Subscription[[]byte], subscriptionsPerIteration)

			for j := range subscriptionsPerIteration {
				pattern := fmt.Sprintf("bench/%d/%s", j, patterns[j%len(patterns)])

				sub, serr := bus.Subscribe(pattern)
				require.NoError(b, serr)

				handler := &mockMsgHandler2{}
				sub.OnMessage(handler)

				subs[j] = sub
			}

			b.StartTimer()

			// Unsubscribe all (this is what we're actually benchmarking)
			for j := range subscriptionsPerIteration {
				require.NoError(b, subs[j].Unsubscribe())
			}
		}
	})

	// Benchmark wildcard matching performance with many subscriptions
	b.Run("WildcardMatching", func(b *testing.B) {
		b.StopTimer()

		config := blazesub.Config{
			UseGoroutinePool: false,
		}
		bus, err := blazesub.NewBus(config)
		require.NoError(b, err)

		defer bus.Close()

		// Create a mix of exact and wildcard subscriptions
		const numSubscriptions = 1000

		for i := range numSubscriptions {
			var pattern string

			switch {
			case i%5 == 0:
				// Every 5th is a wildcard subscription
				pattern = fmt.Sprintf("bench/%d/+/topic", i/5)
			case i%7 == 0:
				// Every 7th is a deep wildcard
				pattern = fmt.Sprintf("bench/%d/#", i/7)
			default:
				// Others are exact
				pattern = fmt.Sprintf("bench/%d/exact/topic", i)
			}

			sub, serr := bus.Subscribe(pattern)
			require.NoError(b, serr)

			handler := &mockMsgHandler2{}
			sub.OnMessage(handler)
		}

		b.StartTimer()

		// Benchmark publishing to topics that match different numbers of subscriptions
		for i := range b.N {
			switch i % 3 {
			case 0:
				// Match only one exact subscription
				bus.Publish(fmt.Sprintf("bench/%d/exact/topic", i%numSubscriptions), []byte("payload"))
			case 1:
				// Match wildcard + subscriptions
				bus.Publish(fmt.Sprintf("bench/%d/wildcard/topic", (i%200)*5), []byte("payload"))
			case 2:
				// Match # subscriptions
				bus.Publish(fmt.Sprintf("bench/%d/anything/goes/here", (i%142)*7), []byte("payload"))
			}
		}
	})
}
