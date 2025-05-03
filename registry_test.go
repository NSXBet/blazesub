package blazesub

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMatchTopic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	tests := []struct {
		name   string
		filter string
		topic  string
		want   bool
	}{
		// Exact matches (including case insensitivity)
		{"exact match", "user/created", "user/created", true},
		{"exact match with different case", "user/created", "User/Created", true},
		{"exact match with all caps", "USER/CREATED", "user/created", true},
		{"exact match with mixed case", "User/Created", "uSer/cReAteD", true},
		{"exact mismatch", "user/created", "user/deleted", false},

		// Single-level wildcard (+) tests
		{"+ wildcard single level", "users/+/profile", "users/123/profile", true},
		{"+ wildcard at beginning", "+/topic", "prefix/topic", true},
		{"+ wildcard at end", "users/profile/+", "users/profile/settings", true},
		{"+ wildcard with empty segment", "users/+/profile", "users//profile", false}, // Invalid topic, so no match
		{"multiple consecutive + wildcards", "users/+/+/+", "users/a/b/c", true},
		{"multiple + wildcards separated", "users/+/profile/+/settings", "users/123/profile/456/settings", true},
		{"+ wildcard no match - wrong level", "+/user", "user", false},
		{"+ wildcard no match - wrong segment", "users/+/profile", "users/123/settings", false},
		{"+ wildcard partial segment", "users/+/profile", "users/123profile", false},

		// Multi-level wildcard (#) tests
		{"# wildcard matches exact level", "users/#", "users/123", true},
		{"# wildcard matches multiple levels", "users/#", "users/123/profile/settings", true},
		{"# wildcard matches empty extra level", "users/#", "users/", false}, // Invalid topic, so no match
		{"# wildcard with prefix", "users/123/#", "users/123/profile", true},
		{"# wildcard with multi-level prefix", "users/123/profile/#", "users/123/profile/settings/notifications", true},
		{"# wildcard no match - wrong prefix", "users/123/#", "users/456/profile", false},
		{"# wildcard no match - wrong level", "users/#", "user", false},
		{"# wildcard no match - wrong topic structure", "users/#", "clients/123", false},

		// Combined + and # wildcard tests
		{"+ then # wildcards", "users/+/#", "users/123/profile", true},
		{"+ then # wildcards multi-level", "users/+/#", "users/123/profile/settings", true},
		{"multiple + then # wildcards", "users/+/+/#", "users/123/456/profile", true},
		{"+ in middle with # at end", "users/+/profile/#", "users/123/profile/settings", true},
		{
			"+ in middle with # at end multi-level",
			"users/+/profile/#",
			"users/123/profile/settings/notifications",
			true,
		},
		{"+ and # no match - wrong segments", "users/+/profile/#", "users/123/settings/profile", false},

		// Length mismatch tests
		{"too short without wildcard", "a/b/c", "a/b", false},
		{"too long without wildcard", "a/b", "a/b/c", false},
		{"exact length match", "a/b/c", "a/b/c", true},
		{"different length but # wildcard", "a/b/#", "a/b/c/d/e", true},
		{"different length with + but no #", "a/+/c", "a/b/c/d", false},

		// Complex path tests
		{
			"complex path exact match",
			"users/profile/settings/notifications",
			"users/profile/settings/notifications",
			true,
		},
		{"complex path with + wildcard", "users/+/settings/+/status", "users/johndoe/settings/email/status", true},
		{"complex path with # wildcard", "devices/+/sensors/#", "devices/livingroom/sensors/temperature/celsius", true},
		{"complex path partial match", "users/+/settings", "users/johndoe/profile", false},

		// Edge cases
		{"single segment exact match", "topic", "topic", true},
		{"single segment wildcard", "+", "topic", true},
		{"single segment multi-level wildcard", "#", "topic/subtopic", true},
		{"single segment multi-level wildcard", "#", "a/very/deep/path/structure", true},
		{"empty topic no match", "topic", "", false},

		// Special characters in topics
		{"numbers in topic", "device/+/reading", "device/123/reading", true},
		{"dashes in topic", "region/+/city", "region/north-america/city", true},
		{"underscores in topic", "user_type/+", "user_type/admin", true},
	}

	registry := NewRegistry()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := registry.matchTopic(tt.filter, tt.topic); got != tt.want {
				t.Errorf(
					"registry.matchTopic() = %v, want %v for filter=%q, topic=%q",
					got,
					tt.want,
					tt.filter,
					tt.topic,
				)
			}
		})
	}
}

func TestRegistry(t *testing.T) {
	// Create registry
	registry := NewRegistry()

	// Create test subscriptions
	sub1 := &Subscription{id: 1}
	sub2 := &Subscription{id: 2}
	sub3 := &Subscription{id: 3}
	sub4 := &Subscription{id: 4}
	sub5 := &Subscription{id: 5}

	// Register subscriptions
	require.NoError(t, registry.RegisterSubscription("user/created", 1, sub1))
	require.NoError(t, registry.RegisterSubscription("matches/+/scores", 2, sub2))
	require.NoError(t, registry.RegisterSubscription("matches/#", 3, sub3))
	require.NoError(t, registry.RegisterSubscription("matches/123/scores/final", 4, sub4))
	require.NoError(t, registry.RegisterSubscription("other/topic", 5, sub5))

	// Test invalid filter
	require.Error(t, registry.RegisterSubscription("invalid/+filter", 6, sub1))

	// Test executing subscriptions
	t.Run("Execute exact match", func(t *testing.T) {
		var matchedIDs []uint64
		require.NoError(
			t,
			registry.ExecuteSubscriptions("user/created", func(ctx context.Context, sub *Subscription) error {
				matchedIDs = append(matchedIDs, sub.id)
				return nil
			}),
		)
		expected := []uint64{1}
		require.Equal(t, matchedIDs, expected)
	})

	t.Run("Execute + wildcard match", func(t *testing.T) {
		var matchedIDs []uint64
		require.NoError(
			t,
			registry.ExecuteSubscriptions("matches/123/scores", func(ctx context.Context, sub *Subscription) error {
				matchedIDs = append(matchedIDs, sub.id)
				return nil
			}),
		)
		expected := []uint64{2, 3} // matches/+/scores and matches/#
		slices.Sort(matchedIDs)
		require.Equal(t, expected, matchedIDs)
	})

	t.Run("Execute # wildcard match", func(t *testing.T) {
		var matchedIDs []uint64
		require.NoError(
			t,
			registry.ExecuteSubscriptions(
				"matches/123/scores/final",
				func(ctx context.Context, sub *Subscription) error {
					matchedIDs = append(matchedIDs, sub.id)
					return nil
				},
			),
		)
		expected := []uint64{3, 4} // matches/# and matches/123/scores/final

		slices.Sort(matchedIDs)
		require.Equal(t, matchedIDs, expected)
	})

	t.Run("Execute no match", func(t *testing.T) {
		var matchedIDs []uint64
		require.NoError(
			t,
			registry.ExecuteSubscriptions("no/match/here", func(ctx context.Context, sub *Subscription) error {
				matchedIDs = append(matchedIDs, sub.id)
				return nil
			}),
		)
		require.Len(t, matchedIDs, 0)
	})

	// Test removing a subscription
	t.Run("Remove subscription", func(t *testing.T) {
		registry.RemoveSubscription(3) // Remove matches/#
		var matchedIDs []uint64
		require.NoError(
			t,
			registry.ExecuteSubscriptions("matches/123/scores", func(ctx context.Context, sub *Subscription) error {
				matchedIDs = append(matchedIDs, sub.id)
				return nil
			}),
		)
		expected := []uint64{2} // Only matches/+/scores should remain
		require.Equal(t, expected, matchedIDs)
	})
}

func BenchmarkRegistryExecuteSubscriptionsExactMatch(b *testing.B) {
	registry := NewRegistry()

	// Register 100 subscriptions with exact matches
	for i := range 100 {
		filter := fmt.Sprintf("topic/%d", i)
		require.NoError(b, registry.RegisterSubscription(filter, i, &Subscription{id: uint64(i)}), "filter: %s", filter)
	}

	// Add some wildcard subscriptions too
	require.NoError(b, registry.RegisterSubscription("topic/+", 1000, &Subscription{id: 1000}))
	require.NoError(b, registry.RegisterSubscription("topic/#", 1001, &Subscription{id: 1001}))

	cb := func(ctx context.Context, sub *Subscription) error {
		return nil
	}
	b.ResetTimer()
	for b.Loop() {
		require.NoError(b, registry.ExecuteSubscriptions("topic/1", cb))
	}
}

func BenchmarkRegistryExecuteSubscriptionsWildcardMatch(b *testing.B) {
	registry := NewRegistry()

	// Register 100 exact subscriptions
	for i := range 100 {
		filter := fmt.Sprintf("topic/%d", i)
		require.NoError(b, registry.RegisterSubscription(filter, i, &Subscription{id: uint64(i)}), "filter: %s", filter)
	}

	// Add 50 wildcard subscriptions
	for i := range 50 {
		filter := fmt.Sprintf("wildcard/+/%d", i)
		require.NoError(
			b,
			registry.RegisterSubscription(filter, i+100, &Subscription{id: uint64(i + 100)}),
			"filter: %s",
			filter,
		)
	}

	cb := func(ctx context.Context, sub *Subscription) error {
		return nil
	}

	// Topic that matches only wildcards
	b.ResetTimer()

	for b.Loop() {
		require.NoError(b, registry.ExecuteSubscriptions("wildcard/foo/1", cb))
	}
}

func TestRegistryConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	const (
		NumSubscriptions = 1000
		NumTopics        = 100
		PublishPerTopic  = 10
		TotalPublishes   = NumTopics * PublishPerTopic
	)

	// Create registry
	registry := NewRegistry()

	// Track the message counts
	receivedMessages := make(map[string]int)
	var receivedLock sync.RWMutex

	// Create subscriptions
	startTime := time.Now()
	for i := 0; i < NumSubscriptions; i++ {
		var filter string
		if i%5 == 0 {
			// Every 5th subscription gets a single-level wildcard filter
			filter = fmt.Sprintf("concurrent/+/%d", i%10)
		} else if i%20 == 0 {
			// Every 20th subscription gets a multi-level wildcard filter
			filter = "concurrent/#"
		} else {
			// Regular subscriptions get exact topic matches
			filter = fmt.Sprintf("concurrent/%d/%d", i%NumTopics, i%10)
		}

		sub := &Subscription{id: uint64(i)}
		require.NoError(t, registry.RegisterSubscription(filter, i, sub))
	}
	setupDuration := time.Since(startTime)
	t.Logf("Time to register %d subscriptions: %v", NumSubscriptions, setupDuration)

	// Function to track message received
	callback := func(ctx context.Context, sub *Subscription) error {
		receivedLock.Lock()
		defer receivedLock.Unlock()
		key := fmt.Sprintf("subscription-%d", sub.id)
		receivedMessages[key]++
		return nil
	}

	// Create wait group for publishing goroutines
	var wg sync.WaitGroup
	wg.Add(NumTopics)

	// Publish messages concurrently
	publishStart := time.Now()
	for i := 0; i < NumTopics; i++ {
		go func(topicID int) {
			defer wg.Done()
			for j := 0; j < PublishPerTopic; j++ {
				topic := fmt.Sprintf("concurrent/%d/%d", topicID, j)
				err := registry.ExecuteSubscriptions(topic, callback)
				require.NoError(t, err)
			}
		}(i)
	}

	// Wait for all publishers to complete
	wg.Wait()
	publishDuration := time.Since(publishStart)

	// Count total messages received
	receivedLock.RLock()
	defer receivedLock.RUnlock()

	var total int
	for _, count := range receivedMessages {
		total += count
	}

	// We should have more received messages than published messages
	// due to wildcard subscriptions matching multiple topics
	require.Greater(t, total, TotalPublishes,
		"Should have received more messages than published due to wildcard matching")

	// Verify that messages were distributed across subscriptions
	require.Greater(t, len(receivedMessages), NumSubscriptions/4,
		"Messages should be distributed to at least 25% of subscriptions")

	t.Logf("Total messages published: %d", TotalPublishes)
	t.Logf("Total messages received by all subscriptions: %d", total)
	t.Logf("Number of subscriptions that received messages: %d", len(receivedMessages))

	// Calculate throughput
	messagesPerSecond := float64(TotalPublishes) / publishDuration.Seconds()
	t.Logf("Throughput: %.2f messages/second", messagesPerSecond)
	t.Logf("Average time per message: %v", publishDuration/time.Duration(TotalPublishes))
	t.Logf("Time to publish %d messages across %d topics: %v", TotalPublishes, NumTopics, publishDuration)
}

func TestRegistryHighContention(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	const (
		NumOperations  = 5000
		NumConcurrent  = 20
		OperationsEach = NumOperations / NumConcurrent
	)

	// Create registry
	registry := NewRegistry()

	// Create channels to coordinate goroutines
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(NumConcurrent)

	// Setup goroutines that will perform mixed operations on the registry
	for i := 0; i < NumConcurrent; i++ {
		go func(workerID int) {
			defer wg.Done()

			// Wait for start signal
			<-start

			// Each worker performs a mix of operations
			for j := 0; j < OperationsEach; j++ {
				opType := j % 5
				id := workerID*OperationsEach + j

				switch opType {
				case 0:
					// Register a subscription
					filter := fmt.Sprintf("highcontention/wild/%d/+", workerID)
					sub := &Subscription{id: uint64(id)}
					err := registry.RegisterSubscription(filter, id, sub)
					require.NoError(t, err)
				case 1:
					// Register a subscription
					filter := fmt.Sprintf("highcontention/exact/%d", workerID)
					sub := &Subscription{id: uint64(id)}
					err := registry.RegisterSubscription(filter, id, sub)
					require.NoError(t, err)

				case 2:
					// Remove a subscription (from a previous iteration)
					if j > 10 {
						prevID := workerID*OperationsEach + (j - 10)
						registry.RemoveSubscription(prevID)
					}
				case 3:
					// Execute subscriptions on exact match
					topic := fmt.Sprintf("highcontention/exact/%d", workerID)
					err := registry.ExecuteSubscriptions(topic, func(ctx context.Context, sub *Subscription) error {
						return nil
					})
					require.NoError(t, err)
				case 4:
					// Execute subscriptions on wildcard match
					topic := fmt.Sprintf("highcontention/wild/%d/wildcard", workerID%NumConcurrent)
					err := registry.ExecuteSubscriptions(topic, func(ctx context.Context, sub *Subscription) error {
						return nil
					})
					require.NoError(t, err)
				}
			}
		}(i)
	}

	// Start all goroutines at once for maximum contention
	startTime := time.Now()
	close(start)

	// Wait for all operations to complete
	wg.Wait()
	duration := time.Since(startTime)

	// Report performance metrics
	t.Logf("Completed %d concurrent operations across %d goroutines in %v", NumOperations, NumConcurrent, duration)
	opsPerSecond := float64(NumOperations) / duration.Seconds()
	t.Logf("Throughput: %.2f operations/second", opsPerSecond)
	t.Logf("Average time per operation: %v", duration/time.Duration(NumOperations))
}

// TestMultipleRegistries tests whether using multiple registries in parallel
// increases throughput compared to a single registry.
func TestMultipleRegistries(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	const (
		NumSubscriptions = 2000 // Total subscriptions across all registries
		NumTopics        = 100  // Number of topics to publish
		NumIterations    = 500  // Number of publish operations
		NumRegistries    = 4    // Number of registries to use for parallel version
	)

	// Function to create a slice of test topics
	createTopics := func(count int) []string {
		topics := make([]string, count)
		for i := 0; i < count; i++ {
			topics[i] = fmt.Sprintf("multitest/%d/%d", i%10, i%5)
		}
		return topics
	}

	// Create topics for testing
	topics := createTopics(NumTopics)

	// Create a single registry with all subscriptions
	singleRegistry := NewRegistry()
	addSubscriptionsToRegistry(t, singleRegistry, 0, NumSubscriptions, "multitest")

	// Create multiple registries with subscriptions distributed among them
	multiRegistries := make([]Registry, NumRegistries)
	subsPerRegistry := NumSubscriptions / NumRegistries

	for i := 0; i < NumRegistries; i++ {
		multiRegistries[i] = NewRegistry()
		// Add a portion of subscriptions to each registry
		startIdx := i * subsPerRegistry
		endIdx := startIdx + subsPerRegistry
		if i == NumRegistries-1 {
			// Add any remaining subscriptions to the last registry
			endIdx = NumSubscriptions
		}
		addSubscriptionsToRegistry(t, multiRegistries[i], startIdx, endIdx, "multitest")
	}

	// Function to run multiple publish operations on a single registry
	runSingleRegistry := func() time.Duration {
		start := time.Now()

		var matchedCount int64
		var matchCountLock sync.Mutex

		callback := func(ctx context.Context, sub *Subscription) error {
			matchCountLock.Lock()
			matchedCount++
			matchCountLock.Unlock()
			return nil
		}

		for i := 0; i < NumIterations; i++ {
			topic := topics[i%NumTopics]
			err := singleRegistry.ExecuteSubscriptions(topic, callback)
			require.NoError(t, err)
		}

		duration := time.Since(start)
		t.Logf("Single registry matched %d subscriptions total", matchedCount)
		return duration
	}

	// Function to run multiple publish operations spread across multiple registries
	runMultiRegistries := func() time.Duration {
		start := time.Now()

		var wg sync.WaitGroup
		var matchedCount int64

		callback := func(ctx context.Context, sub *Subscription) error {
			atomic.AddInt64(&matchedCount, 1)
			return nil
		}

		// Calculate operations per registry
		opsPerRegistry := NumIterations / NumRegistries

		// Launch a goroutine for each registry
		for i := 0; i < NumRegistries; i++ {
			wg.Add(1)
			go func(regIdx int) {
				defer wg.Done()
				registry := multiRegistries[regIdx]

				startOp := regIdx * opsPerRegistry
				endOp := startOp + opsPerRegistry
				if regIdx == NumRegistries-1 {
					endOp = NumIterations // Ensure we run all operations
				}

				for j := startOp; j < endOp; j++ {
					topic := topics[j%NumTopics]
					err := registry.ExecuteSubscriptions(topic, callback)
					if err != nil {
						t.Errorf("Error executing subscriptions: %v", err)
						return
					}
				}
			}(i)
		}

		wg.Wait()
		duration := time.Since(start)
		t.Logf("Multiple registries matched %d subscriptions total", matchedCount)
		return duration
	}

	// Run the tests
	singleDuration := runSingleRegistry()
	multiDuration := runMultiRegistries()

	// Report results
	t.Logf("Single registry: %v (%.2f ops/sec)",
		singleDuration, float64(NumIterations)/singleDuration.Seconds())
	t.Logf("Multiple registries: %v (%.2f ops/sec)",
		multiDuration, float64(NumIterations)/multiDuration.Seconds())

	// Calculate speedup
	speedup := float64(singleDuration) / float64(multiDuration)
	t.Logf("Speedup from multiple registries: %.2fx", speedup)

	// We expect multi-registry approach to be faster, but don't enforce it in the test
	// since it depends on hardware, workload, and runtime conditions
	if speedup > 1.0 {
		t.Logf("Multiple registries approach is faster (%.2fx speedup)", speedup)
	} else {
		t.Logf("Single registry approach is faster in this test (%.2fx slowdown)", 1/speedup)
		t.Log("This can happen due to test overhead or when subscriptions are very simple")
	}
}

// Helper function to add subscriptions to a registry
func addSubscriptionsToRegistry(t *testing.T, registry Registry, startIdx, endIdx int, topicPrefix string) {
	for i := startIdx; i < endIdx; i++ {
		var filter string
		if i%3 == 0 {
			// Every 3rd subscription gets a single-level wildcard filter
			filter = fmt.Sprintf("%s/+/%d", topicPrefix, i%10)
		} else if i%7 == 0 {
			// Every 7th subscription gets a multi-level wildcard filter
			filter = fmt.Sprintf("%s/#", topicPrefix)
		} else {
			// Regular subscriptions get exact topic matches
			filter = fmt.Sprintf("%s/%d/%d", topicPrefix, i%10, i%5)
		}

		sub := &Subscription{id: uint64(i)}
		require.NoError(t, registry.RegisterSubscription(filter, i, sub))
	}
}
