package blazesub

import (
	"strings"

	"github.com/puzpuzpuz/xsync/v4"
	"go.uber.org/atomic"
)

// DefaultExactMatchesCapacity is the default pre-allocation capacity for the exact matches map.
const (
	DefaultExactMatchesCapacity = 1024
	DefaultMaxResultCacheSize   = 10000
)

// TrieNode represents a node in the subscription trie.
type TrieNode[T any] struct {
	segment       string
	children      *xsync.Map[string, *TrieNode[T]]
	subscriptions *xsync.Map[uint64, *Subscription[T]] // Use thread-safe map for subscriptions
}

func (t *TrieNode[T]) Children() map[string]*TrieNode[T] {
	return xsync.ToPlainMap(t.children)
}

func (t *TrieNode[T]) Subscriptions() *xsync.Map[uint64, *Subscription[T]] {
	return t.subscriptions
}

// SubscriptionTrie is a trie-based structure for efficient topic subscriptions.
type SubscriptionTrie[T any] struct {
	root            *TrieNode[T]
	exactMatches    *xsync.Map[string, *xsync.Map[uint64, *Subscription[T]]] // Fast map for exact match topics
	wildcardCount   *atomic.Uint32                                           // Counter for wildcard subscriptions
	exactMatchCount *atomic.Uint32
	totalCount      *atomic.Uint32

	// Caching for frequently used operations
	resultCache        *xsync.Map[string, []*Subscription[T]] // Cache for frequently accessed topic matches
	maxResultCacheSize int32                                  // Maximum size of the result cache
}

// NewSubscriptionTrie creates a new subscription trie.
func NewSubscriptionTrie[T any]() *SubscriptionTrie[T] {
	st := &SubscriptionTrie[T]{
		root: &TrieNode[T]{
			segment:       "",
			children:      xsync.NewMap[string, *TrieNode[T]](),
			subscriptions: xsync.NewMap[uint64, *Subscription[T]](),
		},
		exactMatches:       xsync.NewMap[string, *xsync.Map[uint64, *Subscription[T]]](),
		resultCache:        xsync.NewMap[string, []*Subscription[T]](),
		maxResultCacheSize: DefaultMaxResultCacheSize,

		wildcardCount:   atomic.NewUint32(0),
		exactMatchCount: atomic.NewUint32(0),
		totalCount:      atomic.NewUint32(0),
	}

	return st
}

// hasWildcard checks if a subscription pattern contains wildcard characters.
func hasWildcard(pattern string) bool {
	return strings.ContainsAny(pattern, "+#")
}

func (st *SubscriptionTrie[T]) SubscriptionCount() int {
	return int(st.totalCount.Load())
}

func (st *SubscriptionTrie[T]) WildcardCount() int {
	return int(st.wildcardCount.Load())
}

func (st *SubscriptionTrie[T]) ExactMatchCount() int {
	return int(st.exactMatchCount.Load())
}

// splitTopic returns segments of a topic split by the '/' character.
// This implementation is optimized to reduce allocations.
func (st *SubscriptionTrie[T]) splitTopic(topic string) []string {
	// Special case for empty topics
	if topic == "" {
		return []string{}
	}

	// Fast path for simple topics with no separator
	if !strings.Contains(topic, "/") {
		return []string{topic}
	}

	// Count separators to pre-allocate exact slice size
	separatorCount := 0
	for i := 0; i < len(topic); i++ {
		if topic[i] == '/' {
			separatorCount++
		}
	}

	// Create a slice with exact capacity needed
	segments := make([]string, 0, separatorCount+1)

	// Manual splitting to avoid additional allocations
	start := 0
	for i := 0; i < len(topic); i++ {
		if topic[i] == '/' {
			// Add segment to result
			if i > start {
				segments = append(segments, topic[start:i])
			} else {
				segments = append(segments, "")
			}
			start = i + 1
		}
	}

	// Add the final segment
	if start < len(topic) {
		segments = append(segments, topic[start:])
	} else if start == len(topic) {
		// Handle trailing slash
		segments = append(segments, "")
	}

	return segments
}

// Subscribe adds a subscription for a topic pattern.
func (st *SubscriptionTrie[T]) Subscribe(subID uint64, topic string, handler MessageHandler[T]) *Subscription[T] {
	// Create a new subscription
	subscription := &Subscription[T]{
		id:            subID,
		topic:         topic,
		handler:       handler,
		unsubscribeFn: nil,
	}

	// Set the unsubscribe function that references this trie
	unsubscribeFn := func() error {
		st.Unsubscribe(topic, subID)

		return nil
	}
	subscription.SetUnsubscribeFunc(unsubscribeFn)

	isWildcard := hasWildcard(topic)

	st.totalCount.Add(1)

	if isWildcard {
		st.wildcardCount.Add(1)
	} else {
		st.exactMatchCount.Add(1)
	}

	// Clear result cache if any exists for this topic
	// This ensures we'll compute fresh results after a subscription change
	st.resultCache.Delete(topic)

	// Always add to the trie for node cleanup tests to work properly
	segments := st.splitTopic(topic)

	// Add to the trie
	currentNode := st.root

	var subscriptions *xsync.Map[uint64, *Subscription[T]]

	for _, segment := range segments {
		trieNode, _ := currentNode.children.LoadOrStore(
			segment,
			&TrieNode[T]{
				segment:       segment,
				children:      xsync.NewMap[string, *TrieNode[T]](),
				subscriptions: xsync.NewMap[uint64, *Subscription[T]](),
			},
		)

		currentNode = trieNode

		subscriptions = trieNode.subscriptions

		// If we reach a wildcard segment "#", it matches everything at this level and below
		if segment == "#" {
			break
		}
	}

	// Add the subscription to the final node's map (thread-safe)
	subscriptions.Store(subID, subscription)

	// If it's an exact match, also add to the exactMatches map
	if !isWildcard {
		topicSubs, _ := st.exactMatches.LoadOrStore(topic, xsync.NewMap[uint64, *Subscription[T]]())

		topicSubs.Store(subID, subscription)
	}

	// Also clear any result caches for related topics
	// This is important for wildcard subscriptions
	if isWildcard {
		// For wildcard topics, invalidate all cached results since this subscription might match many topics
		st.resultCache.Clear()
	}

	return subscription
}

// Unsubscribe removes a subscription for a topic pattern.
func (st *SubscriptionTrie[T]) Unsubscribe(topic string, subscriptionID uint64) {
	isWildcard := hasWildcard(topic)

	st.totalCount.Sub(1)

	if isWildcard {
		st.wildcardCount.Sub(1)
	} else {
		st.exactMatchCount.Sub(1)
	}

	// Clear result cache if any exists for this topic
	st.resultCache.Delete(topic)

	// Fast path for exact matches
	if !isWildcard {
		// Try the direct map lookup first for exact matches
		topicSubs, found := st.exactMatches.Load(topic)
		if found {
			// Delete the specific subscription
			topicSubs.Delete(subscriptionID)

			// If the map is empty, remove the topic entry - use a cheaper isEmpty check
			isEmpty := topicSubs.Size() == 0
			if isEmpty {
				st.exactMatches.Delete(topic)
			}
		}
	} else {
		// For wildcard topics, invalidate all cached results since this unsubscription might affect many topics
		st.resultCache.Clear()
	}

	// Always clean up the trie for all subscriptions
	segments := st.splitTopic(topic)
	if len(segments) == 0 {
		return
	}

	currentNode := st.root

	// Pre-allocate nodePath with the exact capacity
	nodePath := make([]*TrieNode[T], 0, len(segments))

	var subscriptions *xsync.Map[uint64, *Subscription[T]]

	// Navigate to the target node - fast path for short segments
	for _, segment := range segments {
		newNode, exists := currentNode.children.Load(segment)
		if !exists {
			// Topic path doesn't exist in the trie
			return
		}

		nodePath = append(nodePath, currentNode)
		currentNode = newNode

		subscriptions = currentNode.subscriptions

		// If we reach a wildcard segment "#", it matches everything
		if segment == "#" {
			break
		}
	}

	// Delete the subscription from the final node's map
	// Check if the subscription exists before decrementing the counter
	subscriptions.Delete(subscriptionID)

	// Check if the node is now empty - use Size() instead of Range
	isEmpty := subscriptions.Size() == 0

	// Clean up empty nodes only if needed
	if isEmpty && currentNode.children.Size() == 0 {
		// Use optimized cleanup that doesn't use Range
		cleanupEmptyNodesOptimized(nodePath, segments)
	}
}

// cleanupEmptyNodesOptimized is an optimized version that avoids Range calls.
func cleanupEmptyNodesOptimized[T any](nodePath []*TrieNode[T], segments []string) {
	for i := len(nodePath) - 1; i >= 0; i-- {
		parentNode := nodePath[i]
		childSegment := segments[i]

		childNode, exists := parentNode.children.Load(childSegment)
		if !exists {
			continue
		}

		// Check if the node is empty using Size() rather than Range
		isEmpty := childNode.subscriptions.Size() == 0
		if isEmpty && childNode.children.Size() == 0 {
			parentNode.children.Delete(childSegment)
		} else {
			// If we find a non-empty node, stop cleanup
			break
		}
	}
}

// findMatches recursively finds all matching subscriptions for a topic.
func findMatches[T any](node *TrieNode[T], segments []string, index int, result map[uint64]*Subscription[T]) {
	// Fast check for wildcard "#" - this matches everything at this level and below
	if wildcard, exists := node.children.Load("#"); exists {
		// Add all wildcard subscriptions
		wildcard.subscriptions.Range(func(id uint64, sub *Subscription[T]) bool {
			result[id] = sub
			return true
		})
	}

	// If we've processed all segments, collect subscriptions at current node
	if index >= len(segments) {
		// No need to continue if we're past the segments or the node has no subscriptions
		if node.subscriptions.Size() == 0 {
			return
		}

		// Add all subscriptions from the current node
		node.subscriptions.Range(func(id uint64, sub *Subscription[T]) bool {
			result[id] = sub
			return true
		})
		return
	}

	segment := segments[index]
	nextIndex := index + 1 // Pre-compute the next index

	// Most common case: exact match for the segment
	if child, exists := node.children.Load(segment); exists {
		findMatches(child, segments, nextIndex, result)
	}

	// Handle "+" wildcard (matches exactly one segment)
	if plus, exists := node.children.Load("+"); exists {
		findMatches(plus, segments, nextIndex, result)
	}
}

// FindMatchingSubscriptions returns all subscriptions that match a given topic.
func (st *SubscriptionTrie[T]) FindMatchingSubscriptions(topic string) []*Subscription[T] {
	// Fast path for empty topic
	if topic == "" {
		return nil
	}

	// Try cache first - most topics are frequently reused
	if cachedResult, ok := st.resultCache.Load(topic); ok {
		return cachedResult
	}

	// Optimization: If no wildcards exist in the system, we only need exact matches
	if st.wildcardCount.Load() == 0 {
		// Fast exact match lookup
		topicSubs, found := st.exactMatches.Load(topic)
		if !found {
			return nil
		}

		// Optimize for the common case of a single subscriber
		size := topicSubs.Size()
		if size == 1 {
			var singleSub *Subscription[T]
			topicSubs.Range(func(_ uint64, sub *Subscription[T]) bool {
				singleSub = sub
				return false // stop after first one
			})

			// Return a pre-allocated single-element slice
			result := []*Subscription[T]{singleSub}

			// Cache for future lookups
			st.resultCache.Store(topic, result)
			return result
		}

		// Pre-allocate exact size for multiple subscribers
		result := make([]*Subscription[T], 0, size)
		topicSubs.Range(func(_ uint64, sub *Subscription[T]) bool {
			result = append(result, sub)
			return true
		})

		// Cache result for future lookups
		st.resultCache.Store(topic, result)
		return result
	}

	// We have wildcards, so need to combine exact and wildcard matches
	// Estimate an initial capacity for the result map based on wildcard count
	resultMap := make(map[uint64]*Subscription[T], 8) // Most topics have few matches

	// First add any exact matches
	if topicSubs, found := st.exactMatches.Load(topic); found {
		topicSubs.Range(func(id uint64, sub *Subscription[T]) bool {
			resultMap[id] = sub
			return true
		})
	}

	// Then find wildcard matches
	segments := st.splitTopic(topic)
	findMatches(st.root, segments, 0, resultMap)

	// Create result slice with exact capacity
	result := make([]*Subscription[T], 0, len(resultMap))
	for _, sub := range resultMap {
		result = append(result, sub)
	}

	// Cache result for future lookups (if not too many cached topics already)
	if st.resultCache.Size() < int(st.maxResultCacheSize) {
		st.resultCache.Store(topic, result)
	}

	return result
}

func (st *SubscriptionTrie[T]) Root() *TrieNode[T] {
	return st.root
}
