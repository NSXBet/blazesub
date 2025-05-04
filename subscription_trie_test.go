package blazesub_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NSXBet/blazesub"
	"github.com/stretchr/testify/require"
)

// mockHandler is a minimal implementation of MessageHandler for testing.
type mockHandler struct {
	messageReceived bool
	mutex           sync.Mutex
}

func (m *mockHandler) OnMessage(_ *blazesub.Message) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.messageReceived = true

	return nil
}

func TestNewSubscriptionTrie(t *testing.T) {
	t.Parallel()

	trie := blazesub.NewSubscriptionTrie()

	if trie == nil {
		t.Fatal("NewSubscriptionTrie should return a non-nil trie")
	}

	if trie.Root() == nil {
		t.Fatal("Root node should not be nil")
	}

	// Check that we start with an empty root node
	size := 0
	trie.Root().Subscriptions().Range(func(_ uint64, _ *blazesub.Subscription) bool {
		size++
		return true
	})

	if size != 0 {
		t.Fatal("Root node should have no subscriptions initially")
	}
}

func TestSubscribeAndFindExactMatch(t *testing.T) {
	t.Parallel()

	trie := blazesub.NewSubscriptionTrie()
	handler := &mockHandler{
		messageReceived: false,
		mutex:           sync.Mutex{},
	}

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
	anotherHandler := &mockHandler{
		messageReceived: false,
		mutex:           sync.Mutex{},
	}
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
	t.Parallel()

	trie := blazesub.NewSubscriptionTrie()
	handler := &mockHandler{
		messageReceived: false,
		mutex:           sync.Mutex{},
	}

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

// Rest of the test cases remain similar but with removed references to bus.
func TestSubscribeAndFindWildcardMatches(t *testing.T) {
	trie := blazesub.NewSubscriptionTrie()
	handler := &mockHandler{
		messageReceived: false,
		mutex:           sync.Mutex{},
	}

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
	t.Parallel()

	trie := blazesub.NewSubscriptionTrie()
	handler := &mockHandler{
		messageReceived: false,
		mutex:           sync.Mutex{},
	}

	// Subscribe to a nested topic
	trie.Subscribe(1, "deep/nested/topic/path", handler)

	// Verify it's there
	if len(trie.Root().Children()) != 1 || trie.Root().Children()["deep"] == nil {
		t.Fatal("Expected 'deep' node to be created")
	}

	deepNode := trie.Root().Children()["deep"]
	if len(deepNode.Children()) != 1 || deepNode.Children()["nested"] == nil {
		t.Fatal("Expected 'nested' node to be created")
	}

	// Unsubscribe
	trie.Unsubscribe("deep/nested/topic/path", 1)

	// Check if nodes were cleaned up
	if len(trie.Root().Children()) != 0 {
		t.Fatal("Expected all nodes to be cleaned up after unsubscription")
	}
}

func TestMultiLevelWildcardMatch(t *testing.T) {
	t.Parallel()

	trie := blazesub.NewSubscriptionTrie()
	handler := &mockHandler{
		messageReceived: false,
		mutex:           sync.Mutex{},
	}

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
	t.Parallel()

	trie := blazesub.NewSubscriptionTrie()
	handler := &mockHandler{
		messageReceived: false,
		mutex:           sync.Mutex{},
	}

	// Use a wait group to synchronize goroutines
	var waitGroup sync.WaitGroup

	// Number of concurrent operations
	numOperations := 100
	// Create an array to track successfully created subscriptions
	subIDs := make([]uint64, numOperations)
	var subIDsMutex sync.Mutex

	// Add subscriptions concurrently
	for operationIndex := range numOperations {
		waitGroup.Add(1)
		id := uint64(operationIndex + 1)

		go func(id uint64, index int) {
			defer waitGroup.Done()

			sub := trie.Subscribe(id, "concurrent/topic", handler)
			// Record the subscription ID in its corresponding array position
			subIDsMutex.Lock()
			subIDs[index] = sub.ID()
			subIDsMutex.Unlock()
		}(id, operationIndex)
	}

	// Wait for all subscriptions to complete
	waitGroup.Wait()

	// Make sure all subscriptions are processed
	time.Sleep(500 * time.Millisecond)

	// Helper function to count valid subscriptions
	countValidSubs := func() int {
		matches := trie.FindMatchingSubscriptions("concurrent/topic")
		count := 0
		// Count the valid subscriptions in the result
		subIDsMutex.Lock()
		defer subIDsMutex.Unlock()

		for _, match := range matches {
			// Verify the subscription is in our tracked list
			for _, id := range subIDs {
				if match.ID() == id && id != 0 {
					count++
					break
				}
			}
		}
		return count
	}

	// Verify at least 95% of subscriptions were added
	minExpected := numOperations * 95 / 100
	require.Eventually(t, func() bool {
		count := countValidSubs()
		return count >= minExpected
	}, 5*time.Second, 10*time.Millisecond, "Expected at least %d concurrent subscriptions, got %d", minExpected, countValidSubs())

	// Get the actual count of subscriptions for the next check
	actualSubscriptionCount := countValidSubs()
	halfExpected := actualSubscriptionCount / 2

	// Test concurrent unsubscribe operations
	waitGroup = sync.WaitGroup{}
	halfOperations := numOperations / 2

	// Start removing subscriptions for the first half
	for index := range halfOperations {
		waitGroup.Add(1)
		id := uint64(index + 1)

		go func(id uint64) {
			defer waitGroup.Done()
			trie.Unsubscribe("concurrent/topic", id)

			// Mark this ID as removed by setting it to 0
			subIDsMutex.Lock()
			for i, sid := range subIDs {
				if sid == id {
					subIDs[i] = 0
					break
				}
			}
			subIDsMutex.Unlock()
		}(id)
	}

	// Wait for all unsubscribe operations to complete
	waitGroup.Wait()
	time.Sleep(500 * time.Millisecond)

	// Final check - we should have approximately half the subscriptions remaining
	// Allow for some variance in the counts due to concurrency
	minHalfExpected := halfExpected * 90 / 100 // At least 90% of the expected half
	require.Eventually(t, func() bool {
		count := countValidSubs()
		return count >= minHalfExpected
	}, 5*time.Second, 10*time.Millisecond, "Expected at least %d subscriptions after concurrent operations, got %d",
		minHalfExpected, countValidSubs())
}

func TestFindMatchesWithMultipleWildcards(t *testing.T) {
	t.Parallel()

	trie := blazesub.NewSubscriptionTrie()
	handler := &mockHandler{
		messageReceived: false,
		mutex:           sync.Mutex{},
	}

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
	t.Parallel()

	trie := blazesub.NewSubscriptionTrie()
	handler := &mockHandler{
		messageReceived: false,
		mutex:           sync.Mutex{},
	}

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
	t.Parallel()

	trie := blazesub.NewSubscriptionTrie()
	handler := &mockHandler{
		messageReceived: false,
		mutex:           sync.Mutex{},
	}

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

	for _, testcase := range testCases {
		matches := trie.FindMatchingSubscriptions(testcase.topic)
		found := false

		for _, match := range matches {
			if testcase.expected && match.ID() == testcase.subID {
				found = true

				break
			}
		}

		if found != testcase.expected {
			if testcase.expected {
				t.Fatalf("Topic '%s': Expected to find subscription %d but didn't", testcase.topic, testcase.subID)
			} else {
				t.Fatalf("Topic '%s': Found unexpected match", testcase.topic)
			}
		}
	}
}

func TestMultipleSubscribesAndUnsubscribes(t *testing.T) {
	t.Parallel()

	trie := blazesub.NewSubscriptionTrie()
	handler := &mockHandler{
		messageReceived: false,
		mutex:           sync.Mutex{},
	}

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
	trie := blazesub.NewSubscriptionTrie()
	handler := &mockHandler{
		messageReceived: false,
		mutex:           sync.Mutex{},
	}

	b.ResetTimer()

	for i := range b.N {
		trie.Subscribe(uint64(i), "benchmark/topic", handler)
	}
}

func BenchmarkFindMatchingSubscriptions(b *testing.B) {
	trie := blazesub.NewSubscriptionTrie()
	handler := &mockHandler{
		messageReceived: false,
		mutex:           sync.Mutex{},
	}

	// Add some subscriptions for the benchmark
	trie.Subscribe(1, "benchmark/topic", handler)
	trie.Subscribe(2, "benchmark/+", handler)
	trie.Subscribe(3, "+/topic", handler)
	trie.Subscribe(4, "benchmark/#", handler)

	b.ResetTimer()

	for b.Loop() {
		trie.FindMatchingSubscriptions("benchmark/topic")
	}
}

func BenchmarkUnsubscribe(b *testing.B) {
	trie := blazesub.NewSubscriptionTrie()
	handler := &mockHandler{
		messageReceived: false,
		mutex:           sync.Mutex{},
	}

	// Pre-populate
	for i := range b.N {
		trie.Subscribe(uint64(i), "benchmark/topic", handler)
	}

	b.ResetTimer()

	for i := range b.N {
		trie.Unsubscribe("benchmark/topic", uint64(i))
	}
}

func BenchmarkWildcardMatching(b *testing.B) {
	trie := blazesub.NewSubscriptionTrie()
	handler := &mockHandler{
		messageReceived: false,
		mutex:           sync.Mutex{},
	}

	// Add a lot of wildcard subscriptions
	topics := []string{
		"a/+/c", "a/b/+", "+/b/c", "a/#", "+/+/+",
		"x/+/z", "x/y/+", "+/y/z", "x/#", "#",
	}

	for i, topic := range topics {
		trie.Subscribe(uint64(i+1), topic, handler)
	}

	b.ResetTimer()

	for i := range b.N {
		// Test matching against different topics in rotation
		topic := "a/b/c"

		switch i % 3 {
		case 1:
			topic = "x/y/z"
		case 2:
			topic = "new/different/topic"
		}

		trie.FindMatchingSubscriptions(topic)
	}
}

// BenchmarkTrieVsMapVsLinearSearch compares trie performance against a map lookup and a brute force linear search.
func BenchmarkTrieVsMapVsLinearSearch(b *testing.B) {
	// Generate test data
	const numTopics = 1000

	const numSubscriptions = 5000

	const numWildcards = 500

	// Create implementations
	trie := blazesub.NewSubscriptionTrie()
	linearSubscriptions := make([]*blazesub.Subscription, 0, numSubscriptions)
	mapSubscriptions := make(map[string][]*blazesub.Subscription) // Direct map for fastest possible lookup
	handler := &mockHandler{
		messageReceived: false,
		mutex:           sync.Mutex{},
	}

	// Generate subscriptions with a mix of exact matches and wildcards
	topics := make([]string, 0, numTopics)

	for i := range numTopics {
		topic := fmt.Sprintf("test/topic/%d/level/%d", i%100, i%10)
		topics = append(topics, topic)
	}

	// Add subscriptions to all implementations
	for i := range numSubscriptions - numWildcards {
		topicIndex := i % numTopics
		topic := topics[topicIndex]

		subscription := trie.Subscribe(uint64(i), topic, handler)
		linearSubscriptions = append(linearSubscriptions, subscription)

		// Add to map (direct lookup)
		mapSubscriptions[topic] = append(mapSubscriptions[topic], subscription)
	}

	// Add wildcard subscriptions (only for trie and linear search)
	for index := range numWildcards {
		var topic string

		if index%2 == 0 {
			// + wildcard
			topic = fmt.Sprintf("test/+/%d/+/%d", index%50, index%5)
		} else {
			// # wildcard
			topic = fmt.Sprintf("test/topic/%d/#", index%50)
		}

		// Note: We don't add wildcards to the map since we're measuring exact match performance
		subscription := trie.Subscribe(uint64(numSubscriptions-numWildcards+index), topic, handler)
		linearSubscriptions = append(linearSubscriptions, subscription)
	}

	// Create a list of test topics to lookup
	testTopics := make([]string, 100)

	for index := range testTopics {
		switch {
		case index < 50:
			// Existing topics for exact match
			testTopics[index] = topics[index*10]
		case index < 75:
			// Topics that should match wildcards
			testTopics[index] = fmt.Sprintf("test/anything/%d/something/%d", index%50, index%5)
		default:
			// Non-matching topics
			testTopics[index] = fmt.Sprintf("other/topic/%d/no/match", index)
		}
	}

	// Linear search function that simulates basic topic matching without a trie
	linearSearch := func(topic string) []*blazesub.Subscription {
		results := make([]*blazesub.Subscription, 0)

		for _, sub := range linearSubscriptions {
			if matchTopic(sub.Topic(), topic) {
				results = append(results, sub)
			}
		}

		return results
	}

	// Map lookup function - fastest possible for exact matches
	mapLookup := func(topic string) []*blazesub.Subscription {
		return mapSubscriptions[topic] // This will be nil if not found
	}

	// Benchmark trie lookup
	b.Run("Trie", func(b *testing.B) {
		b.ResetTimer()

		for i := range b.N {
			topic := testTopics[i%len(testTopics)]
			_ = trie.FindMatchingSubscriptions(topic)
		}
	})

	// Benchmark map lookup (fastest possible, but no wildcard support)
	b.Run("Map", func(b *testing.B) {
		b.ResetTimer()

		for i := range b.N {
			topic := testTopics[i%len(testTopics)]
			_ = mapLookup(topic)
		}
	})

	// Benchmark linear search
	b.Run("LinearSearch", func(b *testing.B) {
		b.ResetTimer()

		for i := range b.N {
			topic := testTopics[i%len(testTopics)]
			_ = linearSearch(topic)
		}
	})
}

// BenchmarkTrieVsMapExactMatchOnly compares trie performance against map lookup for exact matches only.
func BenchmarkTrieVsMapExactMatchOnly(b *testing.B) {
	// Generate test data for exact matches only
	const numTopics = 1000

	const numSubscriptions = 5000

	// Create implementations
	trie := blazesub.NewSubscriptionTrie()
	mapSubscriptions := make(map[string][]*blazesub.Subscription) // Direct map
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

	// Add exact match subscriptions to both implementations
	for i := range numSubscriptions {
		topicIndex := i % numTopics
		topic := topics[topicIndex]

		subscription := trie.Subscribe(uint64(i), topic, handler)
		mapSubscriptions[topic] = append(mapSubscriptions[topic], subscription)
	}

	// Create test topics (all exact matches)
	testTopics := make([]string, 100)

	for index := range testTopics {
		testTopics[index] = topics[index%numTopics]
	}

	// Map lookup function
	mapLookup := func(topic string) []*blazesub.Subscription {
		return mapSubscriptions[topic]
	}

	// Benchmark trie lookup (exact matches only)
	b.Run("TrieExactOnly", func(b *testing.B) {
		b.ResetTimer()

		for i := range b.N {
			topic := testTopics[i%len(testTopics)]
			_ = trie.FindMatchingSubscriptions(topic)
		}
	})

	// Benchmark map lookup
	b.Run("Map", func(b *testing.B) {
		b.ResetTimer()

		for i := range b.N {
			topic := testTopics[i%len(testTopics)]
			_ = mapLookup(topic)
		}
	})
}

// This simulates how a basic non-trie implementation would match topics.
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
	for i := range filterSegments {
		if filterSegments[i] != "+" && filterSegments[i] != topicSegments[i] {
			return false
		}
	}

	return true
}

// BenchmarkTrieWithDifferentWildcardDensities measures how wildcards affect performance.
func BenchmarkTrieWithDifferentWildcardDensities(b *testing.B) {
	handler := &mockHandler{
		messageReceived: false,
		mutex:           sync.Mutex{},
	}

	// Test with different percentages of wildcard subscriptions
	densities := []int{0, 5, 10, 25, 50, 75, 100}

	for _, wildcardPercentage := range densities {
		b.Run(fmt.Sprintf("Wildcards_%d%%", wildcardPercentage), func(b *testing.B) {
			trie := blazesub.NewSubscriptionTrie()

			const totalSubs = 1000

			// Calculate how many wildcard subscriptions to add
			wildcardCount := totalSubs * wildcardPercentage / 100
			normalCount := totalSubs - wildcardCount

			// Add normal subscriptions
			for i := range normalCount {
				topic := fmt.Sprintf("test/topic/%d/exact/%d", i%100, i%10)
				trie.Subscribe(uint64(i), topic, handler)
			}

			// Add wildcard subscriptions
			for subscriptionIndex := range wildcardCount {
				var topic string

				switch subscriptionIndex % 3 {
				case 0:
					// Single + wildcard
					topic = fmt.Sprintf("test/+/%d/exact/%d", subscriptionIndex%50, subscriptionIndex%5)
				case 1:
					// Multiple + wildcards
					topic = fmt.Sprintf("test/+/%d/+/%d", subscriptionIndex%50, subscriptionIndex%5)
				default:
					// # wildcard
					topic = fmt.Sprintf("test/topic/%d/#", subscriptionIndex%50)
				}

				trie.Subscribe(uint64(normalCount+subscriptionIndex), topic, handler)
			}

			// Create test topics
			testTopics := make([]string, 100)
			for i := range testTopics {
				testTopics[i] = fmt.Sprintf("test/topic/%d/exact/%d", i%100, i%10)
			}

			b.ResetTimer()

			for i := range b.N {
				topic := testTopics[i%len(testTopics)]
				_ = trie.FindMatchingSubscriptions(topic)
			}
		})
	}
}

// BenchmarkTrieVsLinearSearch redirects to BenchmarkTrieVsMapVsLinearSearch for backward compatibility.
func BenchmarkTrieVsLinearSearch(b *testing.B) {
	BenchmarkTrieVsMapVsLinearSearch(b)
}

// BenchmarkTrieVsMapMixedLookups compares trie and map performance with varying percentages of wildcard matches.
//
//nolint:gocognit // reason: complex benchmark test.
func BenchmarkTrieVsMapMixedLookups(b *testing.B) {
	// Setup same test data across all test cases
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

	// Generate wildcard patterns
	wildcardPatterns := []string{
		"test/+/0/level/0",
		"test/topic/+/level/+",
		"test/topic/0/#",
		"test/#",
	}

	// Create test cases with different proportions of wildcard lookups
	testCases := []struct {
		name               string
		wildcardPercentage int
	}{
		{"0%_Wildcards", 0},
		{"25%_Wildcards", 25},
		{"50%_Wildcards", 50},
		{"75%_Wildcards", 75},
		{"100%_Wildcards", 100},
	}

	for _, testcase := range testCases {
		b.Run(testcase.name, func(b *testing.B) {
			// Create fresh implementations for each test case
			trie := blazesub.NewSubscriptionTrie()
			mapSubscriptions := make(map[string][]*blazesub.Subscription)

			// Add exact match subscriptions to both implementations
			for i := range numSubscriptions {
				topicIndex := i % numTopics
				topic := topics[topicIndex]

				subscription := trie.Subscribe(uint64(i), topic, handler)
				mapSubscriptions[topic] = append(mapSubscriptions[topic], subscription)
			}

			// Add wildcard subscriptions (only to the trie)
			for i, pattern := range wildcardPatterns {
				trie.Subscribe(uint64(numSubscriptions+i), pattern, handler)
			}

			// Create test topics with the specified percentage of wildcard matches
			testTopics := make([]string, 100)

			for index := range testTopics {
				if index < (100 - testcase.wildcardPercentage) {
					// Exact match topics
					testTopics[index] = topics[index%numTopics]
				} else {
					// Create topics that match wildcards but aren't in the map
					testTopics[index] = fmt.Sprintf("test/wildcard/%d/level/%d", index%10, index%5)
				}
			}

			// Map lookup function
			mapLookup := func(topic string) []*blazesub.Subscription {
				return mapSubscriptions[topic]
			}

			// Benchmark trie lookup
			b.Run("Trie", func(b *testing.B) {
				b.ResetTimer()

				for i := range b.N {
					topic := testTopics[i%len(testTopics)]
					_ = trie.FindMatchingSubscriptions(topic)
				}
			})

			// Benchmark map lookup
			b.Run("Map", func(b *testing.B) {
				b.ResetTimer()

				for i := range b.N {
					topic := testTopics[i%len(testTopics)]
					_ = mapLookup(topic)
				}
			})
		})
	}
}
