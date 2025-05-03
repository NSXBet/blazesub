package blazesub

import (
	"strings"
	"sync/atomic"

	"github.com/puzpuzpuz/xsync/v4"
)

// DefaultExactMatchesCapacity is the default pre-allocation capacity for the exact matches map.
const DefaultExactMatchesCapacity = 1024

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
		maxResultCacheSize: 10000, // Cache up to 10000 results
	}

	// Initialize atomic counter to 0
	st.wildcardCount.Store(0)

	return st
}

// hasWildcard checks if a subscription pattern contains wildcard characters.
func hasWildcard(pattern string) bool {
	return strings.ContainsAny(pattern, "+#")
}

// getSplitTopicCached returns cached segments if available, otherwise splits and caches
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
	// This ensures we'll compute fresh results after a subscription change
	st.resultCache.Delete(topic)

	// If not a wildcard, remove from exactMatches
	if !isWildcard {
		topicSubs, found := st.exactMatches.Load(topic)
		if found {
			// Delete the specific subscription
			topicSubs.Delete(subscriptionID)

			// If the map is empty, remove the topic entry
			// We need to check if the map is empty
			isEmpty := true
			topicSubs.Range(func(key uint64, value *Subscription) bool {
				isEmpty = false
				return false // stop iteration at first item
			})

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

	currentNode := st.root

	// Pre-allocate nodePath with the expected capacity
	nodePath := make([]*TrieNode, 0, len(segments))

	// Navigate to the target node
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

	// Check if the node is now empty
	isEmpty := true
	currentNode.subscriptions.Range(func(key uint64, value *Subscription) bool {
		isEmpty = false
		return false // stop at first entry
	})

	// Clean up empty nodes
	if isEmpty && currentNode.children.Size() == 0 {
		cleanupEmptyNodes(nodePath, segments)
	}
}

// cleanupEmptyNodes removes nodes that have no subscriptions and no children.
func cleanupEmptyNodes(nodePath []*TrieNode, segments []string) {
	for i := len(nodePath) - 1; i >= 0; i-- {
		parentNode := nodePath[i]
		childSegment := segments[i]

		childNode, exists := parentNode.children.Load(childSegment)
		if !exists {
			continue
		}

		// Check if the node is empty
		isEmpty := true
		childNode.subscriptions.Range(func(key uint64, value *Subscription) bool {
			isEmpty = false
			return false // stop at first entry
		})

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
	// Try to get from cache first
	if cachedResult, ok := st.resultCache.Load(topic); ok {
		return cachedResult
	}

	// Create a map to hold results (to avoid duplicates)
	resultMap := make(map[uint64]*Subscription)

	// First check exactMatches for a direct hit (fastest path)
	topicSubs, found := st.exactMatches.Load(topic)
	if found {
		// Safely iterate over the xsync map
		topicSubs.Range(func(id uint64, sub *Subscription) bool {
			resultMap[id] = sub

			return true
		})

		// Fast path: If there are no wildcard subscriptions, we can skip trie traversal
		if st.wildcardCount.Load() == 0 {
			// No wildcards in the system, just return exact matches
			result := make([]*Subscription, 0, len(resultMap))
			for _, sub := range resultMap {
				result = append(result, sub)
			}

			// Cache the result before returning
			st.resultCache.Store(topic, result)
			return result
		}
	}

	// Check if we need to search for wildcard matches
	if st.wildcardCount.Load() > 0 {
		// Search the trie for wildcard matches
		segments := st.getSplitTopicCached(topic)

		findMatches(st.root, segments, 0, resultMap)
	}

	// Convert result map to slice
	result := make([]*Subscription, 0, len(resultMap))
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
