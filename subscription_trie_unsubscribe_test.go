package blazesub

import (
	"strconv"
	"sync"
	"testing"
)

func TestTrieUnsubscribeTable(t *testing.T) {
	handler := &mockHandler{
		messageReceived: false,
		mutex:           sync.Mutex{},
	}

	tests := []struct {
		name          string
		subscriptions []struct {
			id    uint64
			topic string
		}
		unsubscribe []struct {
			id    uint64
			topic string
		}
		expectedMatches map[string]int // topic -> expected match count
		expectedEmpty   bool           // whether the trie should be empty after unsubscribes
	}{
		{
			name: "unsubscribe single subscription",
			subscriptions: []struct {
				id    uint64
				topic string
			}{
				{1, "test/topic"},
			},
			unsubscribe: []struct {
				id    uint64
				topic string
			}{
				{1, "test/topic"},
			},
			expectedMatches: map[string]int{
				"test/topic": 0,
			},
			expectedEmpty: true,
		},
		{
			name: "unsubscribe one of multiple subscriptions",
			subscriptions: []struct {
				id    uint64
				topic string
			}{
				{1, "test/topic"},
				{2, "test/topic"},
				{3, "test/topic"},
			},
			unsubscribe: []struct {
				id    uint64
				topic string
			}{
				{2, "test/topic"},
			},
			expectedMatches: map[string]int{
				"test/topic": 2, // 2 remain
			},
			expectedEmpty: false,
		},
		{
			name: "unsubscribe all subscriptions",
			subscriptions: []struct {
				id    uint64
				topic string
			}{
				{1, "test/topic1"},
				{2, "test/topic2"},
				{3, "test/topic3"},
				{4, "test/topic/nested"},
				{5, "other/path"},
			},
			unsubscribe: []struct {
				id    uint64
				topic string
			}{
				{1, "test/topic1"},
				{2, "test/topic2"},
				{3, "test/topic3"},
				{4, "test/topic/nested"},
				{5, "other/path"},
			},
			expectedMatches: map[string]int{
				"test/topic1":       0,
				"test/topic2":       0,
				"test/topic3":       0,
				"test/topic/nested": 0,
				"other/path":        0,
			},
			expectedEmpty: true,
		},
		{
			name: "unsubscribe with wildcards",
			subscriptions: []struct {
				id    uint64
				topic string
			}{
				{1, "test/+/topic"},
				{2, "test/#"},
				{3, "other/path"},
			},
			unsubscribe: []struct {
				id    uint64
				topic string
			}{
				{1, "test/+/topic"},
				{2, "test/#"},
			},
			expectedMatches: map[string]int{
				"test/any/topic":         0, // "test/+/topic" was unsubscribed
				"test/deep/nested/topic": 0, // "test/#" was unsubscribed
				"other/path":             1, // still subscribed
			},
			expectedEmpty: false,
		},
		{
			name: "unsubscribe complex hierarchical structure",
			subscriptions: []struct {
				id    uint64
				topic string
			}{
				{1, "a/b/c"},
				{2, "a/b/d"},
				{3, "a/e/f"},
				{4, "g/h/i"},
			},
			unsubscribe: []struct {
				id    uint64
				topic string
			}{
				{1, "a/b/c"},
				{2, "a/b/d"},
			},
			expectedMatches: map[string]int{
				"a/b/c": 0,
				"a/b/d": 0,
				"a/e/f": 1,
				"g/h/i": 1,
			},
			expectedEmpty: false,
		},
		{
			name: "unsubscribe invalid IDs",
			subscriptions: []struct {
				id    uint64
				topic string
			}{
				{1, "test/topic"},
			},
			unsubscribe: []struct {
				id    uint64
				topic string
			}{
				{999, "test/topic"}, // Non-existent ID
			},
			expectedMatches: map[string]int{
				"test/topic": 1, // Still exists
			},
			expectedEmpty: false,
		},
		{
			name: "unsubscribe non-existent topics",
			subscriptions: []struct {
				id    uint64
				topic string
			}{
				{1, "test/topic"},
			},
			unsubscribe: []struct {
				id    uint64
				topic string
			}{
				{1, "nonexistent/topic"}, // Non-existent topic
			},
			expectedMatches: map[string]int{
				"test/topic": 1, // Still exists
			},
			expectedEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trie := NewSubscriptionTrie()

			// Add subscriptions
			for _, sub := range tt.subscriptions {
				trie.Subscribe(sub.id, sub.topic, handler)
			}

			// Verify subscriptions were added
			for _, sub := range tt.subscriptions {
				matches := trie.FindMatchingSubscriptions(sub.topic)
				if len(matches) < 1 {
					t.Fatalf("Expected at least one subscription for topic %s before unsubscribe", sub.topic)
				}
			}

			// Unsubscribe
			for _, unsub := range tt.unsubscribe {
				trie.Unsubscribe(unsub.topic, unsub.id)
			}

			// Check expected matches
			for topic, expectedCount := range tt.expectedMatches {
				matches := trie.FindMatchingSubscriptions(topic)
				if len(matches) != expectedCount {
					t.Fatalf("Expected %d matches for topic %s after unsubscribe, got %d",
						expectedCount, topic, len(matches))
				}
			}

			// Check if trie is empty when expected
			if tt.expectedEmpty {
				if len(trie.root.children) != 0 {
					t.Fatalf("Expected trie to be empty after unsubscribe, but it still has %d child nodes",
						len(trie.root.children))
				}
			}
		})
	}
}

// TestUnsubscribeMaintainsMemory tests to see if the unsubscribe function correctly cleans up memory
func TestUnsubscribeMaintainsMemory(t *testing.T) {
	handler := &mockHandler{
		messageReceived: false,
		mutex:           sync.Mutex{},
	}
	trie := NewSubscriptionTrie()

	// Add a lot of subscriptions with a common prefix
	commonPrefix := "test/shared/prefix"
	subscriptions := make(map[uint64]string)

	// Create 1000 subscriptions with the same prefix but different endings
	for i := uint64(1); i <= 1000; i++ {
		topic := commonPrefix + "/" + strconv.FormatUint(i, 10)
		subscriptions[i] = topic
		trie.Subscribe(i, topic, handler)
	}

	// Unsubscribe half of them
	for i := uint64(1); i <= 500; i++ {
		trie.Unsubscribe(subscriptions[i], i)
	}

	// Check that the prefix nodes still exist
	currentNode := trie.root
	for _, segment := range []string{"test", "shared", "prefix"} {
		if child, exists := currentNode.children[segment]; exists {
			currentNode = child
		} else {
			t.Fatalf("Prefix node '%s' was removed even though some subscriptions still use it", segment)
		}
	}

	// Unsubscribe the rest
	for i := uint64(501); i <= 1000; i++ {
		trie.Unsubscribe(subscriptions[i], i)
	}

	// Now the entire trie should be empty
	if len(trie.root.children) != 0 {
		t.Fatalf("Trie should be empty after all subscriptions are removed")
	}
}

// TestResubscribePerformance tests the performance implications of unsubscribing and resubscribing
func TestResubscribePerformance(t *testing.T) {
	handler := &mockHandler{
		messageReceived: false,
		mutex:           sync.Mutex{},
	}
	trie := NewSubscriptionTrie()

	// First, let's add 100 subscriptions
	for i := uint64(1); i <= 100; i++ {
		topic := "test/topic/" + strconv.FormatUint(i, 10)
		trie.Subscribe(i, topic, handler)
	}

	// Now unsubscribe all of them
	for i := uint64(1); i <= 100; i++ {
		topic := "test/topic/" + strconv.FormatUint(i, 10)
		trie.Unsubscribe(topic, i)
	}

	// The trie should be empty
	if len(trie.root.children) != 0 {
		t.Fatalf("Trie should be empty after all subscriptions are removed")
	}

	// Resubscribe to the same topics
	for i := uint64(1); i <= 100; i++ {
		topic := "test/topic/" + strconv.FormatUint(i, 10)
		trie.Subscribe(i+1000, topic, handler) // Use different IDs
	}

	// Verify all subscriptions work
	for i := uint64(1); i <= 100; i++ {
		topic := "test/topic/" + strconv.FormatUint(i, 10)
		matches := trie.FindMatchingSubscriptions(topic)
		if len(matches) != 1 {
			t.Fatalf("Expected 1 match for resubscribed topic %s, got %d", topic, len(matches))
		}
	}
}
