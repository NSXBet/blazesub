package blazesub

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRegistryPool(t *testing.T) {
	// Create a registry pool with 4 registries
	pool := NewRegistryPool(4)
	require.Equal(t, 4, pool.Count(), "Pool should have 4 registries")

	// Add some subscriptions
	const numSubscriptions = 100
	for i := 0; i < numSubscriptions; i++ {
		var filter string
		if i%3 == 0 {
			filter = fmt.Sprintf("pooltest/+/%d", i%10)
		} else if i%7 == 0 {
			filter = "pooltest/#"
		} else {
			filter = fmt.Sprintf("pooltest/%d/%d", i%10, i%5)
		}

		sub := &Subscription{id: uint64(i)}
		err := pool.RegisterSubscription(filter, i, sub)
		require.NoError(t, err)
	}

	// Verify that subscriptions are matched correctly
	t.Run("MatchSubscriptions", func(t *testing.T) {
		var matchedCount int
		callback := func(ctx context.Context, sub *Subscription) error {
			matchedCount++
			return nil
		}

		err := pool.ExecuteSubscriptions("pooltest/1/2", callback)
		require.NoError(t, err)
		require.True(t, matchedCount > 0, "Should have matched at least one subscription")
	})

	// Test round-robin distribution across registries
	t.Run("RoundRobinDistribution", func(t *testing.T) {
		// Create a registry pool with multiple registries
		pool := NewRegistryPool(3)

		// Track unique registry indices used
		usedIndices := make(map[uint64]bool)

		// Execute subscriptions multiple times
		for i := 0; i < 10; i++ {
			pool.ExecuteSubscriptions("test/topic", func(ctx context.Context, sub *Subscription) error {
				return nil
			})

			// Store the index that was used
			usedIndices[pool.nextIndex%uint64(pool.count)] = true
		}

		// Verify that multiple registries were used
		require.Greater(t, len(usedIndices), 1,
			"Should have used multiple registries in round-robin fashion")
	})

	// Test subscription removal
	t.Run("RemoveSubscription", func(t *testing.T) {
		// Create a fresh pool
		pool := NewRegistryPool(2)

		// Add a subscription to be removed
		sub := &Subscription{id: 42}
		pool.RegisterSubscription("remove/test", 42, sub)

		// Verify it exists
		var matched bool
		pool.ExecuteSubscriptions("remove/test", func(ctx context.Context, s *Subscription) error {
			if s.id == 42 {
				matched = true
			}
			return nil
		})
		require.True(t, matched, "Subscription should exist before removal")

		// Remove it
		pool.RemoveSubscription(42)

		// Verify it's gone
		matched = false
		pool.ExecuteSubscriptions("remove/test", func(ctx context.Context, s *Subscription) error {
			if s.id == 42 {
				matched = true
			}
			return nil
		})
		require.False(t, matched, "Subscription should not exist after removal")
	})
}

func TestRegistryPoolParallelExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	const (
		NumSubscriptions = 2000
		NumTopics        = 100
		NumIterations    = 500
		PoolSize         = 4
	)

	// Create topics for testing
	topics := make([]string, NumTopics)
	for i := 0; i < NumTopics; i++ {
		topics[i] = fmt.Sprintf("pooltest/%d/%d", i%10, i%5)
	}

	// Create a registry pool
	pool := NewRegistryPool(PoolSize)

	// Add subscriptions
	for i := 0; i < NumSubscriptions; i++ {
		var filter string
		if i%3 == 0 {
			filter = fmt.Sprintf("pooltest/+/%d", i%10)
		} else if i%7 == 0 {
			filter = "pooltest/#"
		} else {
			filter = fmt.Sprintf("pooltest/%d/%d", i%10, i%5)
		}

		sub := &Subscription{id: uint64(i)}
		require.NoError(t, pool.RegisterSubscription(filter, i, sub))
	}

	// Test regular round-robin execution
	t.Run("RoundRobinExecution", func(t *testing.T) {
		var matchedCount int64
		callback := func(ctx context.Context, sub *Subscription) error {
			atomic.AddInt64(&matchedCount, 1)
			return nil
		}

		start := time.Now()
		for i := 0; i < NumIterations; i++ {
			topic := topics[i%NumTopics]
			require.NoError(t, pool.ExecuteSubscriptions(topic, callback))
		}
		duration := time.Since(start)

		t.Logf("Round-robin execution matched %d subscriptions in %v",
			matchedCount, duration)
		t.Logf("Throughput: %.2f ops/sec",
			float64(NumIterations)/duration.Seconds())
	})

	// Test parallel execution across all registries
	t.Run("ParallelExecution", func(t *testing.T) {
		var matchedCount int64
		callback := func(ctx context.Context, sub *Subscription) error {
			atomic.AddInt64(&matchedCount, 1)
			return nil
		}

		start := time.Now()
		for i := 0; i < NumIterations; i++ {
			topic := topics[i%NumTopics]
			require.NoError(t, pool.ExecuteSubscriptionsParallel(topic, callback))
		}
		duration := time.Since(start)

		t.Logf("Parallel execution matched %d subscriptions in %v",
			matchedCount, duration)
		t.Logf("Throughput: %.2f ops/sec",
			float64(NumIterations)/duration.Seconds())
	})
}

// TestRegistryPoolScalability tests how the registry pool scales with different numbers of registries
func TestRegistryPoolScalability(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	const (
		NumSubscriptions = 5000
		NumTopics        = 200
		NumIterations    = 100
		MaxPoolSize      = 8
	)

	// Create topics for testing
	topics := make([]string, NumTopics)
	for i := 0; i < NumTopics; i++ {
		topics[i] = fmt.Sprintf("scaletest/%d/%d", i%10, i%5)
	}

	// Create subscriptions
	subscriptions := make(map[int]*Subscription)
	subscriptionFilters := make(map[int]string)

	for i := 0; i < NumSubscriptions; i++ {
		var filter string
		if i%3 == 0 {
			filter = fmt.Sprintf("scaletest/+/%d", i%10)
		} else if i%7 == 0 {
			filter = "scaletest/#"
		} else {
			filter = fmt.Sprintf("scaletest/%d/%d", i%10, i%5)
		}

		subscriptions[i] = &Subscription{id: uint64(i)}
		subscriptionFilters[i] = filter
	}

	// Test with different pool sizes
	poolSizes := []int{1, 2, 4, 8}
	results := make(map[int]time.Duration)

	for _, size := range poolSizes {
		pool := NewRegistryPool(size)

		// Add subscriptions
		for i := 0; i < NumSubscriptions; i++ {
			require.NoError(t, pool.RegisterSubscription(
				subscriptionFilters[i], i, subscriptions[i]))
		}

		// Measure performance
		var matchedCount int64
		callback := func(ctx context.Context, sub *Subscription) error {
			atomic.AddInt64(&matchedCount, 1)
			return nil
		}

		// Run with parallelism
		start := time.Now()
		for i := 0; i < NumIterations; i++ {
			topic := topics[i%NumTopics]
			require.NoError(t, pool.ExecuteSubscriptionsParallel(topic, callback))
		}
		duration := time.Since(start)
		results[size] = duration

		t.Logf("Pool size %d: %v (%.2f ops/sec)",
			size, duration, float64(NumIterations)/duration.Seconds())
	}

	// Compare scalability
	baseTime := results[1]
	for _, size := range poolSizes[1:] {
		speedup := float64(baseTime) / float64(results[size])
		t.Logf("Speedup with %d registries vs 1: %.2fx", size, speedup)

		// Ideally, we'd see linear scaling, but realistically there are diminishing returns
		// We don't assert this as it's hardware dependent
	}
}

// BenchmarkRegistryPool benchmarks different aspects of the registry pool
func BenchmarkRegistryPool(b *testing.B) {
	// Setup a pool with 4 registries and 1000 subscriptions
	const (
		PoolSize         = 4
		NumSubscriptions = 1000
	)

	pool := NewRegistryPool(PoolSize)

	// Add subscriptions
	for i := 0; i < NumSubscriptions; i++ {
		var filter string
		if i%3 == 0 {
			filter = fmt.Sprintf("bench/+/%d", i%10)
		} else if i%7 == 0 {
			filter = "bench/#"
		} else {
			filter = fmt.Sprintf("bench/%d/%d", i%10, i%5)
		}

		sub := &Subscription{id: uint64(i)}
		pool.RegisterSubscription(filter, i, sub)
	}

	// Test topics
	topics := make([]string, 10)
	for i := range 10 {
		topics[i] = fmt.Sprintf("bench/%d/%d", i, i%5)
	}

	// Benchmark round-robin execution
	b.Run("RoundRobin", func(b *testing.B) {
		callback := func(ctx context.Context, sub *Subscription) error {
			return nil
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			topic := topics[i%len(topics)]
			pool.ExecuteSubscriptions(topic, callback)
		}
	})

	// Benchmark parallel execution
	b.Run("Parallel", func(b *testing.B) {
		callback := func(ctx context.Context, sub *Subscription) error {
			return nil
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			topic := topics[i%len(topics)]
			pool.ExecuteSubscriptionsParallel(topic, callback)
		}
	})
}
