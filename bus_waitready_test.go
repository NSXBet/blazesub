package blazesub_test

import (
	"testing"
	"time"

	"github.com/NSXBet/blazesub"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

func TestWaitReady(t *testing.T) {
	t.Parallel()

	t.Run("DirectGoroutines_AlwaysReady", func(t *testing.T) {
		t.Parallel()

		// Create a bus with direct goroutines (no worker pool)
		config := blazesub.Config{
			UseGoroutinePool: false,
		}

		bus, err := blazesub.NewBus(config)
		require.NoError(t, err)
		defer bus.Close()

		// Bus with direct goroutines should always be ready immediately
		err = bus.WaitReady(time.Millisecond * 10)
		require.NoError(t, err, "bus with direct goroutines should be ready immediately")
	})

	t.Run("WorkerPool_BecomesReady", func(t *testing.T) {
		t.Parallel()

		// Create a bus with worker pool
		config := blazesub.Config{
			UseGoroutinePool: true,
			WorkerCount:      1000,
		}

		bus, err := blazesub.NewBus(config)
		require.NoError(t, err)
		defer bus.Close()

		// Bus with worker pool should become ready within a reasonable time
		err = bus.WaitReady(time.Second * 2)
		require.NoError(t, err, "bus with worker pool should become ready within timeout")
	})

	t.Run("Timeout_ReturnsError", func(t *testing.T) {
		t.Parallel()

		// Skip in CI environments to prevent false positives
		if testing.Short() {
			t.Skip("skipping timeout test in short mode")
		}

		// Create a bus with worker pool but a very short timeout
		config := blazesub.Config{
			UseGoroutinePool: true,
			WorkerCount:      1000000, // Large worker count to potentially cause a delay
		}

		bus, err := blazesub.NewBus(config)
		require.NoError(t, err)
		defer bus.Close()

		// Use an extremely short timeout to force a timeout error
		err = bus.WaitReady(time.Nanosecond)
		require.Error(t, err, "should return error when timeout is too short")
		require.Contains(t, err.Error(), "timed out waiting for bus to be ready")
	})

	t.Run("AfterClose_ReturnsError", func(t *testing.T) {
		t.Parallel()

		// Create a bus with worker pool
		config := blazesub.Config{
			UseGoroutinePool: true,
			WorkerCount:      1000,
		}

		bus, err := blazesub.NewBus(config)
		require.NoError(t, err)

		// Wait for it to be ready
		err = bus.WaitReady(time.Second)
		require.NoError(t, err)

		// Close the bus
		err = bus.Close()
		require.NoError(t, err)

		// After closing, WaitReady should return an error
		err = bus.WaitReady(time.Second)
		require.Error(t, err)
	})

	t.Run("Default_IsReady", func(t *testing.T) {
		t.Parallel()

		// Create a bus with default configuration
		bus, err := blazesub.NewBusWithDefaults()
		require.NoError(t, err)
		defer bus.Close()

		// Default configuration bus should become ready
		err = bus.WaitReady(time.Second)
		require.NoError(t, err, "default bus should be ready within timeout")
	})
}

func TestWaitReadyWithActivity(t *testing.T) {
	t.Parallel()

	// Create a bus with worker pool
	config := blazesub.Config{
		UseGoroutinePool: true,
		WorkerCount:      100,
	}

	bus, err := blazesub.NewBus(config)
	require.NoError(t, err)
	defer bus.Close()

	// Wait for the bus to be ready
	err = bus.WaitReady(time.Second)
	require.NoError(t, err)

	// Use a simple counter to verify message delivery
	counter := atomic.NewInt64(0)

	subscription, err := bus.Subscribe("test/topic")
	require.NoError(t, err)

	subscription.OnMessage(&SharedCounterHandler{
		MessageCount: counter,
	})

	// Publish a small number of messages
	const messageCount = 10

	for range messageCount {
		bus.Publish("test/topic", []byte("test-data"))
	}

	// Should be able to wait ready again without errors
	err = bus.WaitReady(time.Second)
	require.NoError(t, err, "bus should still be ready after activity")

	// Wait for messages to be processed
	time.Sleep(time.Millisecond * 200)

	// Check that at least some messages were processed
	count := counter.Load()
	require.Positive(t, count, "should have processed some messages")
}
