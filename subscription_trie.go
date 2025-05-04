package blazesub

import (
	"strings"
	"sync/atomic"

	"github.com/puzpuzpuz/xsync/v4"
)

// DefaultExactMatchesCapacity is the default pre-allocation capacity for the exact matches map.
const (
	DefaultExactMatchesCapacity = 1024
	DefaultMaxResultCacheSize   = 10000
)

// TrieNode represents a node in the subscription trie.
type TrieNode struct {
	segment       string
	children      *xsync.Map[string, *TrieNode]
	subscriptions *xsync.Map[uint64, *Subscription] // Use thread-safe map for subscriptions
}

func (t *TrieNode) Children() map[string]*TrieNode {
	return xsync.ToPlainMap(t.children)
}

func (t *TrieNode) Subscriptions() *xsync.Map[uint64, *Subscription] {
	return t.subscriptions
}

// SubscriptionTrie is a trie-based structure for efficient topic subscriptions.
type SubscriptionTrie struct {
	root          *TrieNode
	exactMatches  *xsync.Map[string, *xsync.Map[uint64, *Subscription]] // Fast map for exact match topics
	wildcardCount atomic.Uint32                                         // Counter for wildcard subscriptions

	// Caching for frequently used operations
	segmentCache       *xsync.Map[string, []string]        // Cache for topic segment splitting
	resultCache        *xsync.Map[string, []*Subscription] // Cache for frequently accessed topic matches
	maxResultCacheSize int32                               // Maximum size of the result cache
}

// NewSubscriptionTrie creates a new subscription trie.
func NewSubscriptionTrie() *SubscriptionTrie {
	st := &SubscriptionTrie{
		root: &TrieNode{
			segment:       "",
			children:      xsync.NewMap[string, *TrieNode](),
			subscriptions: xsync.NewMap[uint64, *Subscription](),
		},
		exactMatches:       xsync.NewMap[string, *xsync.Map[uint64, *Subscription]](),
		segmentCache:       xsync.NewMap[string, []string](),
		resultCache:        xsync.NewMap[string, []*Subscription](),
		maxResultCacheSize: DefaultMaxResultCacheSize,
	}

	// Initialize atomic counter to 0
	st.wildcardCount.Store(0)

	return st
}

// hasWildcard checks if a subscription pattern contains wildcard characters.
func hasWildcard(pattern string) bool {
	return strings.ContainsAny(pattern, "+#")
}

// getSplitTopicCached returns cached segments if available, otherwise splits and caches.
func (st *SubscriptionTrie) getSplitTopicCached(topic string) []string {
	if cachedSegments, ok := st.segmentCache.Load(topic); ok {
		return cachedSegments
	}

	segments := strings.Split(topic, "/")
	st.segmentCache.Store(topic, segments)
	return segments
}

// Subscribe adds a subscription for a topic pattern.
func (st *SubscriptionTrie) Subscribe(subID uint64, topic string, handler MessageHandler) *Subscription {
	// Create a new subscription
	subscription := &Subscription{
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

	// Clear result cache if any exists for this topic
	// This ensures we'll compute fresh results after a subscription change
	st.resultCache.Delete(topic)

	// Always add to the trie for node cleanup tests to work properly
	segments := st.getSplitTopicCached(topic)

	// Add to the trie
	currentNode := st.root

	for _, segment := range segments {
		trieNode, exists := currentNode.children.Load(segment)
		if !exists {
			trieNode = &TrieNode{
				segment:       segment,
				children:      xsync.NewMap[string, *TrieNode](),
				subscriptions: xsync.NewMap[uint64, *Subscription](),
			}
			currentNode.children.Store(segment, trieNode)
		}

		currentNode = trieNode

		// If we reach a wildcard segment "#", it matches everything at this level and below
		if segment == "#" {
			break
		}
	}

	// Add the subscription to the final node's map (thread-safe)
	currentNode.subscriptions.Store(subID, subscription)

	// Update the wildcard counter if this is a wildcard subscription
	if isWildcard {
		st.wildcardCount.Add(1)
	}

	// If it's an exact match, also add to the exactMatches map
	if !isWildcard {
		topicSubs, found := st.exactMatches.Load(topic)
		if !found {
			topicSubs = xsync.NewMap[uint64, *Subscription]()
			st.exactMatches.Store(topic, topicSubs)
		}

		// Store directly in the thread-safe map
		topicSubs.Store(subID, subscription)
	}

	// Also clear any result caches for related topics
	// This is important for wildcard subscriptions
	if isWildcard {
		// For wildcard topics, invalidate all cached results since this subscription might match many topics
		st.resultCache = xsync.NewMap[string, []*Subscription]()
	}

	return subscription
}

// Unsubscribe removes a subscription for a topic pattern.
func (st *SubscriptionTrie) Unsubscribe(topic string, subscriptionID uint64) {
	isWildcard := hasWildcard(topic)

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
		st.resultCache = xsync.NewMap[string, []*Subscription]()
	}

	// Always clean up the trie for all subscriptions
	segments := st.getSplitTopicCached(topic)
	if len(segments) == 0 {
		return
	}

	currentNode := st.root

	// Pre-allocate nodePath with the exact capacity
	nodePath := make([]*TrieNode, 0, len(segments))

	// Navigate to the target node - fast path for short segments
	for _, segment := range segments {
		newNode, exists := currentNode.children.Load(segment)
		if !exists {
			// Topic path doesn't exist in the trie
			return
		}

		nodePath = append(nodePath, currentNode)
		currentNode = newNode

		// If we reach a wildcard segment "#", it matches everything
		if segment == "#" {
			break
		}
	}

	// Delete the subscription from the final node's map
	// Check if the subscription exists before decrementing the counter
	_, subExists := currentNode.subscriptions.LoadAndDelete(subscriptionID)

	// Decrement the wildcard counter if this was a wildcard subscription
	if isWildcard && subExists {
		st.wildcardCount.Add(^uint32(0)) // Equivalent to -1 for atomic operations
	}

	// Check if the node is now empty - use Size() instead of Range
	isEmpty := currentNode.subscriptions.Size() == 0

	// Clean up empty nodes only if needed
	if isEmpty && currentNode.children.Size() == 0 {
		// Use optimized cleanup that doesn't use Range
		cleanupEmptyNodesOptimized(nodePath, segments)
	}
}

// cleanupEmptyNodesOptimized is an optimized version that avoids Range calls.
func cleanupEmptyNodesOptimized(nodePath []*TrieNode, segments []string) {
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
func findMatches(node *TrieNode, segments []string, index int, result map[uint64]*Subscription) {
	// If we've reached a "#" node, it matches everything at this level and below
	if wildcard, exists := node.children.Load("#"); exists {
		// Safely iterate through the concurrent map
		wildcard.subscriptions.Range(func(id uint64, sub *Subscription) bool {
			result[id] = sub
			return true
		})
	}

	// If we've processed all segments, add subscriptions at current node
	if index >= len(segments) {
		// Safely iterate through the concurrent map
		node.subscriptions.Range(func(id uint64, sub *Subscription) bool {
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
func (st *SubscriptionTrie) FindMatchingSubscriptions(topic string) []*Subscription {
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
			var singleSub *Subscription
			topicSubs.Range(func(_ uint64, sub *Subscription) bool {
				singleSub = sub
				return false // stop at first
			})

			// Create a single-element slice without resizing
			result := []*Subscription{singleSub}
			// Cache the result before returning
			st.resultCache.Store(topic, result)
			return result
		}

		// We know approximately how many subscriptions to expect
		size := topicSubs.Size()
		result := make([]*Subscription, 0, size)

		topicSubs.Range(func(_ uint64, sub *Subscription) bool {
			result = append(result, sub)
			return true
		})

		// Cache the result before returning
		st.resultCache.Store(topic, result)
		return result
	}

	// If we have exact matches and wildcards, start with exactMatches
	resultMap := make(map[uint64]*Subscription)
	topicSubs, found := st.exactMatches.Load(topic)
	if found {
		// Fill the map with exact matches
		topicSubs.Range(func(id uint64, sub *Subscription) bool {
			resultMap[id] = sub
			return true
		})
	}

	// Now search for wildcard matches only if we have wildcards
	// Get the segments only when needed
	segments := st.getSplitTopicCached(topic)

	// Pre-allocate a result slice to avoid resizing
	estimatedSize := min(int(wildcardCount)+len(resultMap), int(st.maxResultCacheSize))

	// Search the trie for wildcard matches
	findMatches(st.root, segments, 0, resultMap)

	// Convert result map to slice with pre-allocated capacity
	result := make([]*Subscription, 0, estimatedSize)
	for _, sub := range resultMap {
		result = append(result, sub)
	}

	// Cache the result
	st.resultCache.Store(topic, result)

	return result
}

func (st *SubscriptionTrie) Root() *TrieNode {
	return st.root
}
