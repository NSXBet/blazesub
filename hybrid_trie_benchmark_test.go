//nolint:testpackage // reason: we need to test private methods
package blazesub

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/puzpuzpuz/xsync/v4"
)

type mockHandler struct {
	messageReceived bool
	mutex           sync.Mutex
}

func (m *mockHandler) OnMessage(_ *Message) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.messageReceived = true

	return nil
}

// BenchmarkHybridTrieExactMatch benchmarks the performance of the hybrid trie for exact match lookups.
func BenchmarkHybridTrieExactMatch(b *testing.B) {
	// Generate test data for exact matches only
	const numTopics = 1000

	const numSubscriptions = 5000

	// Create test implementation
	trie := NewSubscriptionTrie()
	handler := &mockHandler{
		messageReceived: false,
		mutex:           sync.Mutex{},
	}

	// Generate topics
	topics := make([]string, 0, numTopics)

	for i := range numTopics {
		topic := fmt.Sprintf("test/topic/%d/level/%d", i%100, i%10)
		topics = append(topics, topic)
	}

	// Add exact match subscriptions
	for i := range numSubscriptions {
		topicIndex := i % numTopics
		topic := topics[topicIndex]
		trie.Subscribe(uint64(i), topic, handler)
	}

	// Create test topics (all exact matches)
	testTopics := make([]string, 100)
	for i := range testTopics {
		testTopics[i] = topics[i%numTopics]
	}

	b.ResetTimer()

	for i := range b.N {
		topic := testTopics[i%len(testTopics)]
		_ = trie.FindMatchingSubscriptions(topic)
	}
}

// BenchmarkHybridVsOriginalTrie compares our hybrid implementation to a version without the exactMatches optimization.
//
//nolint:gocognit // reason: complex benchmark test.
func BenchmarkHybridVsOriginalTrie(b *testing.B) {
	// Create data shared across both benchmarks
	const numTopics = 1000

	const numSubscriptions = 5000

	handler := &mockHandler{
		messageReceived: false,
		mutex:           sync.Mutex{},
	}

	// Generate topics
	topics := make([]string, 0, numTopics)

	for i := range numTopics {
		topic := fmt.Sprintf("test/topic/%d/level/%d", i%100, i%10)
		topics = append(topics, topic)
	}

	// Benchmark hybrid trie implementation (with exactMatches map)
	b.Run("HybridTrie", func(b *testing.B) {
		trie := NewSubscriptionTrie()

		// Populate subscriptions
		for i := range numSubscriptions {
			topicIndex := i % numTopics
			topic := topics[topicIndex]
			trie.Subscribe(uint64(i), topic, handler)
		}

		// Create test topics
		testTopics := make([]string, 100)
		for i := range testTopics {
			testTopics[i] = topics[i%numTopics]
		}

		b.ResetTimer()

		for i := range b.N {
			topic := testTopics[i%len(testTopics)]
			_ = trie.FindMatchingSubscriptions(topic)
		}
	})

	// Benchmark an equivalent of the original trie (using only the trie structure, not the exactMatches map)
	b.Run("OriginalTrie", func(b *testing.B) {
		// Create an emulated "original" trie by disabling the exactMatches optimization
		trie := &SubscriptionTrie{
			root: &TrieNode{
				segment:       "",
				children:      xsync.NewMap[string, *TrieNode](),
				subscriptions: xsync.NewMap[uint64, *Subscription](),
			},
		}

		// Populate subscriptions (adding only to the trie, not the map)
		for index := range numSubscriptions {
			topicIndex := index % numTopics
			topic := topics[topicIndex]

			// Create a new subscription
			subscription := &Subscription{
				id:            uint64(index),
				topic:         topic,
				handler:       handler,
				unsubscribeFn: nil,
			}

			// Manually add to trie (simulating original behavior)
			segments := strings.Split(topic, "/")

			currentNode := trie.root

			for _, segment := range segments {
				children, exists := currentNode.children.Load(segment)
				if !exists {
					children = &TrieNode{
						segment:       segment,
						children:      xsync.NewMap[string, *TrieNode](),
						subscriptions: xsync.NewMap[uint64, *Subscription](),
					}
					currentNode.children.Store(segment, children)
				}

				currentNode = children
			}

			// Add the subscription to the final node's map
			currentNode.subscriptions.Store(uint64(index), subscription)
		}

		// Create test topics
		testTopics := make([]string, 100)
		for i := range testTopics {
			testTopics[i] = topics[i%numTopics]
		}

		b.ResetTimer()

		for i := range b.N {
			topic := testTopics[i%len(testTopics)]

			// Manually emulate the original FindMatchingSubscriptions
			resultMap := make(map[uint64]*Subscription)
			segments := strings.Split(topic, "/")

			findMatches(trie.root, segments, 0, resultMap)

			result := make([]*Subscription, 0, len(resultMap))
			for _, sub := range resultMap {
				result = append(result, sub)
			}

			_ = result
		}
	})
}

// BenchmarkMixedWorkload benchmarks the hybrid trie with a mix of exact and wildcard lookups.
func BenchmarkMixedWorkload(b *testing.B) {
	// Generate test data
	const numTopics = 1000

	const numSubscriptions = 5000
	// Commented out to fix unused const error
	// const numWildcardSubs = 50

	// Create implementations
	trie := NewSubscriptionTrie()
	handler := &mockHandler{
		messageReceived: false,
		mutex:           sync.Mutex{},
	}

	// Generate topics
	topics := make([]string, 0, numTopics)

	for i := range numTopics {
		topic := fmt.Sprintf("test/topic/%d/level/%d", i%100, i%10)
		topics = append(topics, topic)
	}

	// Add exact match subscriptions
	for i := range numSubscriptions {
		topicIndex := i % numTopics
		topic := topics[topicIndex]
		trie.Subscribe(uint64(i), topic, handler)
	}

	// Add some wildcard subscriptions
	wildcardPatterns := []string{
		"test/+/0/level/0",
		"test/topic/+/level/+",
		"test/topic/0/#",
		"test/#",
		"+/topic/10/level/5",
	}

	for i, pattern := range wildcardPatterns {
		trie.Subscribe(uint64(numSubscriptions+i), pattern, handler)
	}

	// Create mixed test workload (80% exact matches, 20% topics that match wildcards)
	testTopics := make([]string, 100)
	for i := range testTopics {
		if i < 80 {
			// Exact match
			testTopics[i] = topics[i%numTopics]
		} else {
			// Topic that matches wildcards
			testTopics[i] = fmt.Sprintf("test/wildcard/%d/level/%d", i%10, i%5)
		}
	}

	b.ResetTimer()

	for i := range b.N {
		topic := testTopics[i%len(testTopics)]
		_ = trie.FindMatchingSubscriptions(topic)
	}
}
