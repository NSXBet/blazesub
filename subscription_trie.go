package blazesub

import (
	"strings"
	"sync"

	"github.com/puzpuzpuz/xsync/v4"
	"go.uber.org/atomic"
)

// DefaultExactMatchesCapacity is the default pre-allocation capacity for the exact matches map.
const (
	DefaultExactMatchesCapacity = 1024
	DefaultMaxResultCacheSize   = 10000

	reasonableMaxTopicDepth = 8
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

	subPool     sync.Pool // Pool for subscription objects
	segmentPool sync.Pool // Pool for segment slices

	// Used for splitTopic to avoid allocations
	segmentCache *xsync.Map[string, []string]
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
		segmentCache:       xsync.NewMap[string, []string](),
		maxResultCacheSize: DefaultMaxResultCacheSize,

		wildcardCount:   atomic.NewUint32(0),
		exactMatchCount: atomic.NewUint32(0),
		totalCount:      atomic.NewUint32(0),
		subPool: sync.Pool{
			New: func() any {
				return &Subscription[T]{
					status: atomic.NewUint32(0),
				}
			},
		},
		segmentPool: sync.Pool{
			New: func() any {
				s := make([]string, 0, reasonableMaxTopicDepth) // Most topics have fewer than 8 segments
				return &s
			},
		},
	}

	return st
}

// hasWildcard checks if a subscription pattern contains wildcard characters.
func hasWildcard(pattern string) bool {
	return strings.ContainsAny(pattern, "+#")
}

// Use in Subscribe method.
func (st *SubscriptionTrie[T]) getNode(segment string) *TrieNode[T] {
	// Create a new node instead of getting it from a pool
	return &TrieNode[T]{
		segment:       segment,
		children:      xsync.NewMap[string, *TrieNode[T]](),
		subscriptions: xsync.NewMap[uint64, *Subscription[T]](),
	}
}

// Get a subscription from the pool.
func (st *SubscriptionTrie[T]) getSubscription(id uint64, topic string, handler MessageHandler[T]) *Subscription[T] {
	sub, _ := st.subPool.Get().(*Subscription[T])
	sub.id = id
	sub.topic = topic
	sub.handler = handler
	sub.unsubscribeFn = nil

	return sub
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

// Get a segments buffer from the pool.
func (st *SubscriptionTrie[T]) getSegmentsBuffer() []string {
	segmentsPtr, _ := st.segmentPool.Get().(*[]string)
	segments := (*segmentsPtr)[:0] // Reset length but keep capacity

	return segments
}

// Return segments buffer to the pool.
func (st *SubscriptionTrie[T]) recycleSegmentsBuffer(segments []string) {
	// Create a pointer to the segments slice to avoid allocation
	segmentsPtr := &segments
	*segmentsPtr = (*segmentsPtr)[:0] // Clear the slice before returning to pool
	st.segmentPool.Put(segmentsPtr)
}

// splitTopic returns cached segments if available, otherwise splits and caches.
func (st *SubscriptionTrie[T]) splitTopic(topic string) []string {
	// Try to get from cache first
	if segments, ok := st.segmentCache.Load(topic); ok {
		return segments
	}

	// Get a buffer from the pool
	segments := st.getSegmentsBuffer()

	// Split the topic
	start := 0

	for i := range len(topic) {
		if topic[i] == '/' {
			segments = append(segments, topic[start:i])
			start = i + 1
		}
	}
	// Add the final segment
	if start < len(topic) {
		segments = append(segments, topic[start:])
	}

	// Only cache if the map isn't too big (avoid memory leak)
	if st.segmentCache.Size() < int(st.maxResultCacheSize) {
		// Create a copy for caching (since we'll return the buffer to the pool)
		segmentsCopy := make([]string, len(segments))
		copy(segmentsCopy, segments)
		st.segmentCache.Store(topic, segmentsCopy)
	}

	return segments
}

// Subscribe adds a subscription for a topic pattern.
func (st *SubscriptionTrie[T]) Subscribe(subID uint64, topic string, handler MessageHandler[T]) *Subscription[T] {
	// Create a new subscription from the pool
	subscription := st.getSubscription(subID, topic, handler)

	// Set the unsubscribe function that references this trie
	subscription.SetUnsubscribeFunc(st.Unsubscribe)

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

	// Fast path for exact matches - store directly in exactMatches map
	if !isWildcard {
		topicSubs, _ := st.exactMatches.LoadOrStore(topic, xsync.NewMap[uint64, *Subscription[T]]())
		topicSubs.Store(subID, subscription)
	}

	// Always add to the trie for node cleanup tests to work properly
	segments := st.splitTopic(topic)

	// Add to the trie
	currentNode := st.root

	var subscriptions *xsync.Map[uint64, *Subscription[T]]

	for _, segment := range segments {
		trieNode, exists := currentNode.children.Load(segment)
		if !exists {
			newNode := st.getNode(segment)

			trieNode, _ = currentNode.children.LoadOrStore(segment, newNode)
		}

		currentNode = trieNode
		subscriptions = trieNode.subscriptions

		// If we reach a wildcard segment "#", it matches everything at this level and below
		if segment == "#" {
			break
		}
	}

	// Add the subscription to the final node's map (thread-safe)
	subscriptions.Store(subID, subscription)

	// Also clear any result caches for related topics
	// This is important for wildcard subscriptions
	if isWildcard {
		// For wildcard topics, invalidate all cached results since this subscription might match many topics
		st.resultCache.Clear()
	}

	// Return segments buffer to pool if from pool
	if _, ok := st.segmentCache.Load(topic); !ok {
		st.recycleSegmentsBuffer(segments)
	}

	return subscription
}

// Recycle subscription back to the pool.
func (st *SubscriptionTrie[T]) recycleSubscription(sub *Subscription[T]) {
	// Clear references
	sub.id = 0
	sub.topic = ""
	sub.handler = nil
	sub.status.Store(0)
	sub.unsubscribeFn = nil

	st.subPool.Put(sub)
}

// Unsubscribe removes a subscription for a topic pattern.
// TODO: Refactor as this is a complex method.
//
//nolint:gocognit // reason: will be refactored in future PRs
func (st *SubscriptionTrie[T]) Unsubscribe(topic string, subscriptionID uint64) error {
	isWildcard := hasWildcard(topic)

	st.totalCount.Sub(1)

	if isWildcard {
		st.wildcardCount.Sub(1)
	} else {
		st.exactMatchCount.Sub(1)
	}

	// Clear result cache if any exists for this topic
	st.resultCache.Delete(topic)

	var foundSub *Subscription[T]

	// Fast path for exact matches
	//nolint:nestif // reason: clear enough
	if !isWildcard {
		// Try the direct map lookup first for exact matches
		topicSubs, found := st.exactMatches.Load(topic)
		if found {
			// Get subscription before deleting it for recycling
			if sub, ok := topicSubs.LoadAndDelete(subscriptionID); ok {
				foundSub = sub
			}

			// If the map is empty, remove the topic entry
			if topicSubs.Size() == 0 {
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
		if foundSub != nil {
			st.recycleSubscription(foundSub)
		}

		return nil
	}

	currentNode := st.root
	// Pre-allocate nodePath with the exact capacity
	nodePath := make([]*TrieNode[T], 0, len(segments))

	// Navigate to the target node - fast path for short segments
	for _, segment := range segments {
		newNode, exists := currentNode.children.Load(segment)
		if !exists {
			// Topic path doesn't exist in the trie
			if foundSub != nil {
				st.recycleSubscription(foundSub)
			}

			// Return segments buffer to pool if from pool
			if _, ok := st.segmentCache.Load(topic); !ok {
				st.recycleSegmentsBuffer(segments)
			}

			return nil
		}

		nodePath = append(nodePath, currentNode)
		currentNode = newNode

		// If we reach a wildcard segment "#", it matches everything
		if segment == "#" {
			break
		}
	}

	// Get subscription before deleting it for recycling
	if foundSub == nil {
		if sub, ok := currentNode.subscriptions.LoadAndDelete(subscriptionID); ok {
			foundSub = sub
		}
	} else {
		// We already found the subscription in exactMatches, just delete it here
		currentNode.subscriptions.Delete(subscriptionID)
	}

	// Check if the node is now empty - use Size() instead of Range
	isEmpty := currentNode.subscriptions.Size() == 0

	// Clean up empty nodes only if needed
	if isEmpty && currentNode.children.Size() == 0 {
		// Use optimized cleanup that doesn't use Range
		st.cleanupEmptyNodesOptimized(nodePath, segments)
	}

	// Recycle the subscription if found
	if foundSub != nil {
		st.recycleSubscription(foundSub)
	}

	// Return segments buffer to pool if from pool
	if _, ok := st.segmentCache.Load(topic); !ok {
		st.recycleSegmentsBuffer(segments)
	}

	return nil
}

// cleanupEmptyNodesOptimized is an optimized version that avoids Range calls.
func (st *SubscriptionTrie[T]) cleanupEmptyNodesOptimized(nodePath []*TrieNode[T], segments []string) {
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
	// If we've reached a "#" node, it matches everything at this level and below
	if wildcard, exists := node.children.Load("#"); exists {
		// Safely iterate through the concurrent map
		wildcard.subscriptions.Range(func(id uint64, sub *Subscription[T]) bool {
			result[id] = sub
			return true
		})
	}

	// If we've processed all segments, add subscriptions at current node
	if index >= len(segments) {
		// Safely iterate through the concurrent map
		node.subscriptions.Range(func(id uint64, sub *Subscription[T]) bool {
			result[id] = sub
			return true
		})

		return
	}

	segment := segments[index]

	// Check for exact match
	if child, exists := node.children.Load(segment); exists {
		findMatches(child, segments, index+1, result)
	}

	// Check for "+" wildcard match (matches any single segment)
	if plus, exists := node.children.Load("+"); exists {
		findMatches(plus, segments, index+1, result)
	}
}

// FindMatchingSubscriptions returns all subscriptions that match a given topic.
func (st *SubscriptionTrie[T]) FindMatchingSubscriptions(topic string) []*Subscription[T] {
	// Special case for empty topic
	if topic == "" {
		return nil
	}

	// Try to get from cache first
	if cachedResult, ok := st.resultCache.Load(topic); ok {
		return cachedResult
	}

	// Fast path: If there are no wildcard subscriptions, we can use exactMatches directly
	wildcardCount := st.wildcardCount.Load()
	if wildcardCount == 0 {
		// No wildcards in the system, just return exact matches (or nil)
		topicSubs, found := st.exactMatches.Load(topic)
		if !found {
			return nil
		}

		// Fast path: If there's only a single subscriber (common case), optimize
		if topicSubs.Size() == 1 {
			var singleSub *Subscription[T]

			topicSubs.Range(func(_ uint64, sub *Subscription[T]) bool {
				singleSub = sub
				return false // stop at first
			})

			// Create a single-element slice without resizing
			result := []*Subscription[T]{singleSub}
			// Cache the result before returning
			st.resultCache.Store(topic, result)

			return result
		}

		// Optimize the allocation by preallocating the exact size
		size := topicSubs.Size()
		result := make([]*Subscription[T], 0, size)

		topicSubs.Range(func(_ uint64, sub *Subscription[T]) bool {
			// Skip closed subscriptions
			if !sub.IsClosed() {
				result = append(result, sub)
			}

			return true
		})

		// Cache the result before returning
		st.resultCache.Store(topic, result)

		return result
	}

	// Pre-allocate map with reasonable size to avoid resizing
	resultMap := make(map[uint64]*Subscription[T], reasonableMaxTopicDepth)

	// If we have exact matches, add them first
	topicSubs, found := st.exactMatches.Load(topic)
	if found {
		// Fill the map with exact matches
		topicSubs.Range(func(id uint64, sub *Subscription[T]) bool {
			// Skip closed subscriptions
			if !sub.IsClosed() {
				resultMap[id] = sub
			}

			return true
		})
	}

	// Now search for wildcard matches only if we have wildcards
	// Get the segments only when needed
	segments := st.splitTopic(topic)

	// Pre-allocate a result slice with reasonable capacity
	estimatedSize := min(int(wildcardCount)+len(resultMap), int(st.maxResultCacheSize))

	// Search the trie for wildcard matches
	findMatches(st.root, segments, 0, resultMap)

	// Convert result map to slice with pre-allocated capacity
	result := make([]*Subscription[T], 0, estimatedSize)

	for _, sub := range resultMap {
		// Double check for closed subscriptions just to be safe
		if !sub.IsClosed() {
			result = append(result, sub)
		}
	}

	// Cache the result
	if len(result) > 0 {
		st.resultCache.Store(topic, result)
	}

	// Return segments buffer to pool if from pool
	if _, ok := st.segmentCache.Load(topic); !ok {
		st.recycleSegmentsBuffer(segments)
	}

	return result
}

func (st *SubscriptionTrie[T]) Root() *TrieNode[T] {
	return st.root
}
