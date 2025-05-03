package blazesub

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// BenchmarkWildcardLookupFrequency benchmarks the impact of different wildcard lookup frequencies
func BenchmarkWildcardLookupFrequency(b *testing.B) {
	// Test with different frequencies of wildcard lookups
	frequencies := []int{0, 25, 50, 75, 100}

	for _, frequency := range frequencies {
		b.Run(fmt.Sprintf("WildcardLookup_%d%%", frequency), func(b *testing.B) {
			// Setup: Create a subscription trie with a mix of exact and wildcard subscriptions
			trie := NewSubscriptionTrie()
			handler := &mockHandler{
				messageReceived: false,
				mutex:           sync.Mutex{},
			}

			// Add 1000 exact match subscriptions
			for i := range 1000 {
				topic := fmt.Sprintf("test/topic/%d/exact", i)
				trie.Subscribe(uint64(i), topic, handler)
			}

			// Add 20 wildcard subscriptions
			for i := range 20 {
				var topic string
				if i%2 == 0 {
					topic = fmt.Sprintf("test/+/%d/+", i)
				} else {
					topic = fmt.Sprintf("test/topic/%d/#", i)
				}
				trie.Subscribe(uint64(1000+i), topic, handler)
			}

			// Create test topics with the specified frequency of wildcard lookups
			testTopics := make([]string, 100)
			for i := range testTopics {
				if i < (100 - frequency) {
					// Exact match topic (already in the trie)
					testTopics[i] = fmt.Sprintf("test/topic/%d/exact", i%1000)
				} else {
					// Topic that matches wildcards
					testTopics[i] = fmt.Sprintf("test/wildcard/%d/lookup", i%20)
				}
			}

			b.ResetTimer()

			for i := range b.N {
				topic := testTopics[i%len(testTopics)]
				_ = trie.FindMatchingSubscriptions(topic)
			}
		})
	}
}

// BenchmarkWildcardCounter measures how the atomic counter affects concurrent performance
func BenchmarkWildcardCounter(b *testing.B) {
	// Create a trie with some subscriptions
	trie := NewSubscriptionTrie()
	handler := &mockHandler{
		messageReceived: false,
		mutex:           sync.Mutex{},
	}

	// Add 1000 exact match subscriptions
	for i := range 1000 {
		topic := fmt.Sprintf("test/topic/%d/exact", i)
		trie.Subscribe(uint64(i), topic, handler)
	}

	// Add 50 wildcard subscriptions
	for i := range 50 {
		var topic string
		if i%2 == 0 {
			topic = fmt.Sprintf("test/+/%d/exact", i)
		} else {
			topic = fmt.Sprintf("test/topic/%d/#", i)
		}
		trie.Subscribe(uint64(1000+i), topic, handler)
	}

	// Create a mix of topics for lookup
	exactTopics := make([]string, 50)
	wildcardTopics := make([]string, 50)

	for i := range 50 {
		exactTopics[i] = fmt.Sprintf("test/topic/%d/exact", i)
		wildcardTopics[i] = fmt.Sprintf("test/wildcard/%d/match", i)
	}

	// Benchmark concurrent lookups with a mix of exact and wildcard matches
	b.Run("MixedLookups", func(b *testing.B) {
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			counter := 0
			for pb.Next() {
				var topic string
				if counter%2 == 0 {
					topic = exactTopics[counter%50]
				} else {
					topic = wildcardTopics[counter%50]
				}
				counter++
				_ = trie.FindMatchingSubscriptions(topic)
			}
		})
	})

	// Benchmark concurrent subscribe/unsubscribe of wildcard topics
	b.Run("WildcardSubscribeUnsubscribe", func(b *testing.B) {
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			counter := 0
			for pb.Next() {
				id := uint64(2000 + counter)
				topic := fmt.Sprintf("benchmark/+/wildcard/%d", counter)

				// Subscribe
				sub := trie.Subscribe(id, topic, handler)

				// And immediately unsubscribe
				require.NoError(b, sub.Unsubscribe())

				counter++
			}
		})
	})
}
