package blazesub

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

// mockHandler is a minimal implementation of MessageHandler for testing
type mockHandler struct {
	messageReceived bool
	mutex           sync.Mutex
}

func (m *mockHandler) OnMessage(message *Message) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.messageReceived = true
	return nil
}

func (m *mockHandler) wasMessageReceived() bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.messageReceived
}

// TestSubscription is a test implementation of Subscription
type TestSubscription struct {
	id                uint64
	topic             string
	handler           MessageHandler
	unsubscribeFn     func() error
	unsubscribeCalled bool
}

func (s *TestSubscription) ID() uint64 {
	return s.id
}

func (s *TestSubscription) Topic() string {
	return s.topic
}

func (s *TestSubscription) SetUnsubscribeFunc(fn func() error) {
	s.unsubscribeFn = fn
}

func (s *TestSubscription) Unsubscribe() error {
	s.unsubscribeCalled = true
	if s.unsubscribeFn != nil {
		return s.unsubscribeFn()
	}
	return nil
}

func TestNewSubscriptionTrie(t *testing.T) {
	trie := NewSubscriptionTrie()

	if trie == nil {
		t.Fatal("NewSubscriptionTrie should return a non-nil trie")
	}

	if trie.root == nil {
		t.Fatal("Trie root should be initialized")
	}

	if len(trie.root.children) != 0 {
		t.Fatal("Root node should have no children initially")
	}

	if len(trie.root.subscriptions) != 0 {
		t.Fatal("Root node should have no subscriptions initially")
	}
}

func TestSubscribeAndFindExactMatch(t *testing.T) {
	trie := NewSubscriptionTrie()
	handler := &mockHandler{}

	// Subscribe to a topic
	subscription := trie.Subscribe(1, "test/topic", handler)

	if subscription == nil {
		t.Fatal("Subscribe should return a non-nil subscription")
	}

	if subscription.ID() != 1 {
		t.Fatalf("Subscription ID should be 1, got %d", subscription.ID())
	}

	if subscription.Topic() != "test/topic" {
		t.Fatalf("Subscription topic should be 'test/topic', got %s", subscription.Topic())
	}

	// Find matching subscriptions
	matches := trie.FindMatchingSubscriptions("test/topic")

	if len(matches) != 1 {
		t.Fatalf("Expected 1 matching subscription, got %d", len(matches))
	}

	if matches[0].ID() != 1 {
		t.Fatalf("Expected subscription ID 1, got %d", matches[0].ID())
	}

	// Subscribe another handler to the same topic
	anotherHandler := &mockHandler{}
	anotherSub := trie.Subscribe(2, "test/topic", anotherHandler)

	if anotherSub == nil {
		t.Fatal("Subscribe should return a non-nil subscription")
	}

	// Now we should have two subscriptions for the same topic
	matches = trie.FindMatchingSubscriptions("test/topic")
	if len(matches) != 2 {
		t.Fatalf("Expected 2 matching subscriptions, got %d", len(matches))
	}
}

func TestUnsubscribe(t *testing.T) {
	trie := NewSubscriptionTrie()
	handler := &mockHandler{}

	// Subscribe to a topic
	subscription := trie.Subscribe(1, "test/topic", handler)

	// Verify it's there
	matches := trie.FindMatchingSubscriptions("test/topic")
	if len(matches) != 1 {
		t.Fatalf("Expected 1 matching subscription before unsubscribe, got %d", len(matches))
	}

	// Unsubscribe using the subscription's method
	err := subscription.Unsubscribe()
	if err != nil {
		t.Fatalf("Unsubscribe returned error: %v", err)
	}

	// Verify it's gone
	matches = trie.FindMatchingSubscriptions("test/topic")
	if len(matches) != 0 {
		t.Fatalf("Expected 0 matching subscriptions after unsubscribe, got %d", len(matches))
	}

	// Test unsubscribing directly from trie
	trie.Subscribe(2, "test/topic", handler)
	trie.Unsubscribe("test/topic", 2)

	matches = trie.FindMatchingSubscriptions("test/topic")
	if len(matches) != 0 {
		t.Fatalf("Expected 0 matching subscriptions after direct unsubscribe, got %d", len(matches))
	}
}

// Rest of the test cases remain similar but with removed references to bus
func TestSubscribeAndFindWildcardMatches(t *testing.T) {
	trie := NewSubscriptionTrie()
	handler := &mockHandler{}

	// Create subscriptions with wildcards
	trie.Subscribe(1, "test/+/topic", handler)  // + wildcard
	trie.Subscribe(2, "test/#", handler)        // # wildcard
	trie.Subscribe(3, "other/topic", handler)   // no match
	trie.Subscribe(4, "test/specific", handler) // specific match

	// Test + wildcard - should match only "test/+/topic" pattern
	matches := trie.FindMatchingSubscriptions("test/anything/topic")
	if len(matches) != 2 { // Should match both "test/+/topic" and "test/#"
		t.Fatalf("Expected 2 matching subscriptions for +, got %d", len(matches))
	}

	// Test if we get the right IDs back
	ids := make(map[uint64]bool)
	for _, sub := range matches {
		ids[sub.ID()] = true
	}
	if !ids[1] || !ids[2] {
		t.Fatalf("Expected to find subscriptions 1 and 2, but found: %v", ids)
	}

	// Test # wildcard
	matches = trie.FindMatchingSubscriptions("test/anything/else/topic")
	if len(matches) != 1 {
		t.Fatalf("Expected 1 matching subscription for #, got %d", len(matches))
	}
	if matches[0].ID() != 2 { // Should only match "test/#"
		t.Fatalf("Expected subscription ID 2, got %d", matches[0].ID())
	}

	// Test specific match
	matches = trie.FindMatchingSubscriptions("test/specific")
	if len(matches) != 2 { // Should match both "test/specific" and "test/#"
		t.Fatalf("Expected 2 matching subscriptions, got %d", len(matches))
	}

	// Verify the correct IDs
	ids = make(map[uint64]bool)
	for _, sub := range matches {
		ids[sub.ID()] = true
	}
	if !ids[2] || !ids[4] {
		t.Fatalf("Expected to find subscriptions 2 and 4, but found: %v", ids)
	}

	// Test no match
	matches = trie.FindMatchingSubscriptions("no/match")
	if len(matches) != 0 {
		t.Fatalf("Expected 0 matching subscriptions, got %d", len(matches))
	}
}

func TestCleanupEmptyNodes(t *testing.T) {
	trie := NewSubscriptionTrie()
	handler := &mockHandler{}

	// Subscribe to a nested topic
	trie.Subscribe(1, "deep/nested/topic/path", handler)

	// Verify it's there
	if len(trie.root.children) != 1 || trie.root.children["deep"] == nil {
		t.Fatal("Expected 'deep' node to be created")
	}

	deepNode := trie.root.children["deep"]
	if len(deepNode.children) != 1 || deepNode.children["nested"] == nil {
		t.Fatal("Expected 'nested' node to be created")
	}

	// Unsubscribe
	trie.Unsubscribe("deep/nested/topic/path", 1)

	// Check if nodes were cleaned up
	if len(trie.root.children) != 0 {
		t.Fatal("Expected all nodes to be cleaned up after unsubscription")
	}
}

func TestMultiLevelWildcardMatch(t *testing.T) {
	trie := NewSubscriptionTrie()
	handler := &mockHandler{}

	// Subscribe with multi-level wildcard
	trie.Subscribe(1, "a/+/+/d", handler) // Matches a/?/?/d

	// Test matches
	testCases := []struct {
		topic    string
		expected int
	}{
		{"a/b/c/d", 1},       // Should match
		{"a/any/thing/d", 1}, // Should match
		{"a/b/c/d/e", 0},     // Should not match (extra level)
		{"a/b/c", 0},         // Should not match (missing level)
		{"different", 0},     // Should not match
	}

	for _, tc := range testCases {
		matches := trie.FindMatchingSubscriptions(tc.topic)
		if len(matches) != tc.expected {
			t.Fatalf("Topic '%s': Expected %d matches, got %d", tc.topic, tc.expected, len(matches))
		}
	}
}

func TestConcurrentAccess(t *testing.T) {
	trie := NewSubscriptionTrie()
	handler := &mockHandler{}

	// Use a wait group to synchronize goroutines
	var wg sync.WaitGroup
	// Number of concurrent operations
	numOperations := 100

	// Add subscriptions concurrently
	for i := 0; i < numOperations; i++ {
		wg.Add(1)
		go func(id uint64) {
			defer wg.Done()
			trie.Subscribe(id, "concurrent/topic", handler)
		}(uint64(i + 1))
	}

	// Wait for all subscriptions to complete
	wg.Wait()

	// Verify all subscriptions were added
	matches := trie.FindMatchingSubscriptions("concurrent/topic")
	if len(matches) != numOperations {
		t.Fatalf("Expected %d concurrent subscriptions, got %d", numOperations, len(matches))
	}

	// Test concurrent unsubscribe and find operations
	wg = sync.WaitGroup{}

	// Start removing subscriptions
	for i := 0; i < numOperations/2; i++ {
		wg.Add(1)
		go func(id uint64) {
			defer wg.Done()
			trie.Unsubscribe("concurrent/topic", id)
		}(uint64(i + 1))
	}

	// Concurrently find matches
	var findResults []int
	var findMutex sync.Mutex
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			matches := trie.FindMatchingSubscriptions("concurrent/topic")

			findMutex.Lock()
			findResults = append(findResults, len(matches))
			findMutex.Unlock()
		}()
	}

	// Wait for all operations to complete
	wg.Wait()

	// Final check - we should have exactly half the subscriptions remaining
	matches = trie.FindMatchingSubscriptions("concurrent/topic")
	if len(matches) != numOperations/2 {
		t.Fatalf("Expected %d subscriptions after concurrent operations, got %d",
			numOperations/2, len(matches))
	}
}

func TestFindMatchesWithMultipleWildcards(t *testing.T) {
	trie := NewSubscriptionTrie()
	handler := &mockHandler{}

	// Add subscriptions with various wildcards
	trie.Subscribe(1, "a/+/c", handler) // Matches a/?/c
	trie.Subscribe(2, "a/b/+", handler) // Matches a/b/?
	trie.Subscribe(3, "+/b/c", handler) // Matches ?/b/c
	trie.Subscribe(4, "a/#", handler)   // Matches a/*
	trie.Subscribe(5, "+/+/+", handler) // Matches ?/?/?

	testCases := []struct {
		topic    string
		expected int // Number of expected matches
	}{
		{"a/b/c", 5},   // Should match all 5 patterns
		{"a/x/c", 3},   // Should match a/+/c, a/#, +/+/+
		{"a/b/x", 3},   // Should match a/b/+, a/#, +/+/+
		{"x/b/c", 2},   // Should match +/b/c, +/+/+
		{"a/b/c/d", 1}, // Should match only a/#
		{"x/y/z", 1},   // Should match only +/+/+
	}

	for _, tc := range testCases {
		matches := trie.FindMatchingSubscriptions(tc.topic)
		if len(matches) != tc.expected {
			t.Fatalf("Topic '%s': Expected %d matches, got %d", tc.topic, tc.expected, len(matches))
		}
	}
}

func TestWildcardAtDifferentPositions(t *testing.T) {
	trie := NewSubscriptionTrie()
	handler := &mockHandler{}

	// Test # wildcard at different positions
	trie.Subscribe(1, "#", handler)     // Matches everything
	trie.Subscribe(2, "a/#", handler)   // Matches a/*
	trie.Subscribe(3, "a/b/#", handler) // Matches a/b/*

	// Test + wildcard at different positions
	trie.Subscribe(4, "+", handler)     // Matches any single-segment topic
	trie.Subscribe(5, "+/b", handler)   // Matches ?/b
	trie.Subscribe(6, "a/+/c", handler) // Matches a/?/c

	testCases := []struct {
		topic    string
		expected int
	}{
		{"x", 2},       // Matches # and +
		{"a", 3},       // Matches #, a/# and +
		{"a/b", 4},     // Matches #, a/#, a/b/# and +/b
		{"a/b/c", 4},   // Matches #, a/#, a/b/# and a/+/c
		{"a/b/c/d", 3}, // Matches #, a/#, a/b/#
		{"a/x/c", 3},   // Matches #, a/# and a/+/c
	}

	for _, tc := range testCases {
		matches := trie.FindMatchingSubscriptions(tc.topic)
		if len(matches) != tc.expected {
			t.Fatalf("Topic '%s': Expected %d matches, got %d", tc.topic, tc.expected, len(matches))
		}
	}
}

func TestEmptySegments(t *testing.T) {
	trie := NewSubscriptionTrie()
	handler := &mockHandler{}

	// Test topics with empty segments
	trie.Subscribe(1, "a//c", handler) // Has empty middle segment
	trie.Subscribe(2, "/b/c", handler) // Starts with empty segment
	trie.Subscribe(3, "a/b/", handler) // Ends with empty segment

	testCases := []struct {
		topic    string
		subID    uint64
		expected bool
	}{
		{"a//c", 1, true},
		{"/b/c", 2, true},
		{"a/b/", 3, true},
		{"a/b/c", 0, false},
	}

	for _, tc := range testCases {
		matches := trie.FindMatchingSubscriptions(tc.topic)
		found := false
		for _, match := range matches {
			if tc.expected && match.ID() == tc.subID {
				found = true
				break
			}
		}
		if found != tc.expected {
			if tc.expected {
				t.Fatalf("Topic '%s': Expected to find subscription %d but didn't", tc.topic, tc.subID)
			} else {
				t.Fatalf("Topic '%s': Found unexpected match", tc.topic)
			}
		}
	}
}

func TestMultipleSubscribesAndUnsubscribes(t *testing.T) {
	trie := NewSubscriptionTrie()
	handler := &mockHandler{}

	// Add a bunch of subscriptions
	for i := uint64(1); i <= 10; i++ {
		topic := "test/topic" + string(rune('0'+i))
		trie.Subscribe(i, topic, handler)
	}

	// Verify they all exist
	for i := uint64(1); i <= 10; i++ {
		topic := "test/topic" + string(rune('0'+i))
		matches := trie.FindMatchingSubscriptions(topic)
		if len(matches) != 1 {
			t.Fatalf("Expected 1 match for topic %s, got %d", topic, len(matches))
		}
	}

	// Remove odd-numbered subscriptions
	for i := uint64(1); i <= 10; i += 2 {
		topic := "test/topic" + string(rune('0'+i))
		trie.Unsubscribe(topic, i)
	}

	// Verify only even-numbered subscriptions remain
	for i := uint64(1); i <= 10; i++ {
		topic := "test/topic" + string(rune('0'+i))
		matches := trie.FindMatchingSubscriptions(topic)

		if i%2 == 0 { // Even, should still exist
			if len(matches) != 1 {
				t.Fatalf("Expected 1 match for topic %s, got %d", topic, len(matches))
			}
		} else { // Odd, should be gone
			if len(matches) != 0 {
				t.Fatalf("Expected 0 matches for topic %s, got %d", topic, len(matches))
			}
		}
	}
}

func BenchmarkSubscribe(b *testing.B) {
	trie := NewSubscriptionTrie()
	handler := &mockHandler{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		trie.Subscribe(uint64(i), "benchmark/topic", handler)
	}
}

func BenchmarkFindMatchingSubscriptions(b *testing.B) {
	trie := NewSubscriptionTrie()
	handler := &mockHandler{}

	// Add some subscriptions for the benchmark
	trie.Subscribe(1, "benchmark/topic", handler)
	trie.Subscribe(2, "benchmark/+", handler)
	trie.Subscribe(3, "+/topic", handler)
	trie.Subscribe(4, "benchmark/#", handler)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		trie.FindMatchingSubscriptions("benchmark/topic")
	}
}

func BenchmarkUnsubscribe(b *testing.B) {
	trie := NewSubscriptionTrie()
	handler := &mockHandler{}

	// Pre-populate
	for i := 0; i < b.N; i++ {
		trie.Subscribe(uint64(i), "benchmark/topic", handler)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		trie.Unsubscribe("benchmark/topic", uint64(i))
	}
}

func BenchmarkWildcardMatching(b *testing.B) {
	trie := NewSubscriptionTrie()
	handler := &mockHandler{}

	// Add a lot of wildcard subscriptions
	topics := []string{
		"a/+/c", "a/b/+", "+/b/c", "a/#", "+/+/+",
		"x/+/z", "x/y/+", "+/y/z", "x/#", "#",
	}

	for i, topic := range topics {
		trie.Subscribe(uint64(i+1), topic, handler)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Test matching against different topics in rotation
		topic := "a/b/c"
		if i%3 == 1 {
			topic = "x/y/z"
		} else if i%3 == 2 {
			topic = "new/different/topic"
		}
		trie.FindMatchingSubscriptions(topic)
	}
}

// BenchmarkTrieVsLinearSearch compares trie performance against a brute force linear search
func BenchmarkTrieVsLinearSearch(b *testing.B) {
	// Generate test data
	const numTopics = 1000
	const numSubscriptions = 5000
	const numWildcards = 500

	// Create both implementations
	trie := NewSubscriptionTrie()
	linearSubscriptions := make([]*Subscription, 0, numSubscriptions)
	handler := &mockHandler{}

	// Generate subscriptions with a mix of exact matches and wildcards
	topics := make([]string, 0, numTopics)
	for i := 0; i < numTopics; i++ {
		topic := fmt.Sprintf("test/topic/%d/level/%d", i%100, i%10)
		topics = append(topics, topic)
	}

	// Add subscriptions to both implementations
	for i := 0; i < numSubscriptions-numWildcards; i++ {
		topicIndex := i % numTopics
		subscription := trie.Subscribe(uint64(i), topics[topicIndex], handler)
		linearSubscriptions = append(linearSubscriptions, subscription)
	}

	// Add wildcard subscriptions
	for i := 0; i < numWildcards; i++ {
		var topic string
		if i%2 == 0 {
			// + wildcard
			topic = fmt.Sprintf("test/+/%d/+/%d", i%50, i%5)
		} else {
			// # wildcard
			topic = fmt.Sprintf("test/topic/%d/#", i%50)
		}
		subscription := trie.Subscribe(uint64(numSubscriptions-numWildcards+i), topic, handler)
		linearSubscriptions = append(linearSubscriptions, subscription)
	}

	// Create a list of test topics to lookup
	testTopics := make([]string, 100)
	for i := 0; i < len(testTopics); i++ {
		if i < 50 {
			// Existing topics
			testTopics[i] = topics[i*10]
		} else if i < 75 {
			// Topics that should match wildcards
			testTopics[i] = fmt.Sprintf("test/anything/%d/something/%d", i%50, i%5)
		} else {
			// Non-matching topics
			testTopics[i] = fmt.Sprintf("other/topic/%d/no/match", i)
		}
	}

	// Linear search function that simulates basic topic matching without a trie
	linearSearch := func(topic string) []*Subscription {
		results := make([]*Subscription, 0)
		for _, sub := range linearSubscriptions {
			if matchTopic(sub.Topic(), topic) {
				results = append(results, sub)
			}
		}
		return results
	}

	// Benchmark trie lookup
	b.Run("Trie", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			topic := testTopics[i%len(testTopics)]
			_ = trie.FindMatchingSubscriptions(topic)
		}
	})

	// Benchmark linear search
	b.Run("LinearSearch", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			topic := testTopics[i%len(testTopics)]
			_ = linearSearch(topic)
		}
	})
}

// matchTopic is a simple helper function that checks if a subscription matches a topic
// This simulates how a basic non-trie implementation would match topics
func matchTopic(filter, topic string) bool {
	// Split topic and filter into segments
	topicSegments := strings.Split(topic, "/")
	filterSegments := strings.Split(filter, "/")

	// Check if there's a multi-level wildcard (#)
	for i, segment := range filterSegments {
		if segment == "#" {
			// # must be the last segment and matches all remaining segments
			return i == len(filterSegments)-1
		}
	}

	// If no # wildcard and segment counts don't match, no match
	if len(topicSegments) != len(filterSegments) {
		return false
	}

	// Check each segment
	for i := 0; i < len(filterSegments); i++ {
		if filterSegments[i] != "+" && filterSegments[i] != topicSegments[i] {
			return false
		}
	}

	return true
}

// BenchmarkTrieWithDifferentWildcardDensities measures how wildcards affect performance
func BenchmarkTrieWithDifferentWildcardDensities(b *testing.B) {
	handler := &mockHandler{}

	// Test with different percentages of wildcard subscriptions
	densities := []int{0, 5, 10, 25, 50, 75, 100}

	for _, wildcardPercentage := range densities {
		b.Run(fmt.Sprintf("Wildcards_%d%%", wildcardPercentage), func(b *testing.B) {
			trie := NewSubscriptionTrie()
			const totalSubs = 1000

			// Calculate how many wildcard subscriptions to add
			wildcardCount := totalSubs * wildcardPercentage / 100
			normalCount := totalSubs - wildcardCount

			// Add normal subscriptions
			for i := 0; i < normalCount; i++ {
				topic := fmt.Sprintf("test/topic/%d/exact/%d", i%100, i%10)
				trie.Subscribe(uint64(i), topic, handler)
			}

			// Add wildcard subscriptions
			for i := 0; i < wildcardCount; i++ {
				var topic string
				if i%3 == 0 {
					// Single + wildcard
					topic = fmt.Sprintf("test/+/%d/exact/%d", i%50, i%5)
				} else if i%3 == 1 {
					// Multiple + wildcards
					topic = fmt.Sprintf("test/+/%d/+/%d", i%50, i%5)
				} else {
					// # wildcard
					topic = fmt.Sprintf("test/topic/%d/#", i%50)
				}
				trie.Subscribe(uint64(normalCount+i), topic, handler)
			}

			// Create test topics
			testTopics := make([]string, 100)
			for i := 0; i < len(testTopics); i++ {
				testTopics[i] = fmt.Sprintf("test/topic/%d/exact/%d", i%100, i%10)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				topic := testTopics[i%len(testTopics)]
				_ = trie.FindMatchingSubscriptions(topic)
			}
		})
	}
}
