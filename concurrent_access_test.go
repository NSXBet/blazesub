package blazesub_test

import (
	"sync"
	"testing"

	"github.com/NSXBet/blazesub"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

// TestConcurrentAccess tests that the subscription trie can handle concurrent operations
// correctly, maintaining exact counts and proper state.
func TestConcurrentAccess(t *testing.T) {
	t.Parallel()

	trie := blazesub.NewSubscriptionTrie()
	handler := newMockHandler(t)

	numOperations := 100

	setSubscriptions := func(num int) {
		wg := sync.WaitGroup{}

		wg.Add(numOperations)

		for i := range num {
			go func(index int) {
				defer wg.Done()

				sub := trie.Subscribe(uint64(index), "concurrent/topic", handler)

				sub.OnMessage(handler)
			}(i)
		}

		wg.Wait()

		require.Equal(t, trie.SubscriptionCount(), num)
		require.Equal(t, trie.ExactMatchCount(), num)
		require.Equal(t, 0, trie.WildcardCount())

		require.Len(t, trie.FindMatchingSubscriptions("concurrent/topic"), num)
	}

	setSubscriptions(numOperations)

	wg := sync.WaitGroup{}

	wg.Add(numOperations)

	for i := range numOperations {
		go func(index int) {
			defer wg.Done()

			trie.Unsubscribe("concurrent/topic", uint64(index))
		}(i)
	}

	wg.Wait()

	require.Equal(t, 0, trie.SubscriptionCount())
	require.Equal(t, 0, trie.ExactMatchCount())
	require.Equal(t, 0, trie.WildcardCount())
	require.Empty(t, trie.FindMatchingSubscriptions("concurrent/topic"))

	setSubscriptions(numOperations)

	wg.Add(numOperations)

	found := atomic.NewUint32(0)

	go func() {
		for range numOperations {
			go func() {
				defer wg.Done()

				matches := trie.FindMatchingSubscriptions("concurrent/topic")

				found.Add(uint32(len(matches)))
			}()
		}
	}()

	wg.Wait()

	// Since we have numOperations subscriptions, and numOperations FindMatchingSubscriptions
	// calls, we should have numOperations * numOperations calls to the handler.
	require.Equal(t, found.Load(), uint32(numOperations)*uint32(numOperations))
}
