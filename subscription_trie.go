package blazesub

import (
	"strings"
	"sync"
	"sync/atomic"
)

// DefaultExactMatchesCapacity is the default pre-allocation capacity for the exact matches map
const DefaultExactMatchesCapacity = 1024

// TrieNode represents a node in the subscription trie
type TrieNode struct {
	segment       string
	children      map[string]*TrieNode
	subscriptions map[uint64]*Subscription // Map of subscriptions by ID at this node
	mutex         sync.RWMutex             // For concurrency safety
}

// SubscriptionTrie is a trie-based structure for efficient topic subscriptions
type SubscriptionTrie struct {
	root            *TrieNode
	exactMatches    map[string]map[uint64]*Subscription // Fast map for exact match topics
	exactMatchMutex sync.RWMutex                        // Separate mutex for the exact matches map
	wildcardCount   atomic.Uint32                       // Counter for wildcard subscriptions
}

// NewSubscriptionTrie creates a new subscription trie
func NewSubscriptionTrie() *SubscriptionTrie {
	st := &SubscriptionTrie{
		root: &TrieNode{
			segment:       "",
			children:      make(map[string]*TrieNode),
			subscriptions: make(map[uint64]*Subscription),
			mutex:         sync.RWMutex{},
		},
		exactMatches:    make(map[string]map[uint64]*Subscription, DefaultExactMatchesCapacity),
		exactMatchMutex: sync.RWMutex{},
	}

	// Initialize atomic counter to 0
	st.wildcardCount.Store(0)

	return st
}

// hasWildcard checks if a subscription pattern contains wildcard characters
func hasWildcard(pattern string) bool {
	return strings.ContainsAny(pattern, "+#")
}

// Subscribe adds a subscription for a topic pattern
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

	// Always add to the trie for node cleanup tests to work properly
	segments := strings.Split(topic, "/")

	st.root.mutex.Lock()
	currentNode := st.root

	for _, segment := range segments {
		if _, exists := currentNode.children[segment]; !exists {
			currentNode.children[segment] = &TrieNode{
				segment:       segment,
				children:      make(map[string]*TrieNode),
				subscriptions: make(map[uint64]*Subscription),
				mutex:         sync.RWMutex{},
			}
		}

		currentNode = currentNode.children[segment]

		// If we reach a wildcard segment "#", it matches everything at this level and below
		if segment == "#" {
			break
		}
	}

	// Add the subscription to the final node's map
	currentNode.subscriptions[subID] = subscription
	st.root.mutex.Unlock()

	// Update the wildcard counter if this is a wildcard subscription
	if isWildcard {
		st.wildcardCount.Add(1)
	}

	// If it's an exact match, also add to the exactMatches map
	if !isWildcard {
		st.exactMatchMutex.Lock()
		if _, exists := st.exactMatches[topic]; !exists {
			st.exactMatches[topic] = make(map[uint64]*Subscription)
		}

		st.exactMatches[topic][subID] = subscription
		st.exactMatchMutex.Unlock()
	}

	return subscription
}

// Unsubscribe removes a subscription for a topic pattern
func (st *SubscriptionTrie) Unsubscribe(topic string, subscriptionID uint64) {
	isWildcard := hasWildcard(topic)

	// If not a wildcard, remove from exactMatches
	if !isWildcard {
		st.exactMatchMutex.Lock()
		if topicSubs, exists := st.exactMatches[topic]; exists {
			delete(topicSubs, subscriptionID)
			// If the map is empty, remove the topic entry
			if len(topicSubs) == 0 {
				delete(st.exactMatches, topic)
			}
		}
		st.exactMatchMutex.Unlock()
	}

	// Always clean up the trie for all subscriptions
	segments := strings.Split(topic, "/")

	st.root.mutex.Lock()
	defer st.root.mutex.Unlock()

	currentNode := st.root
	// Pre-allocate nodePath with the expected capacity
	nodePath := make([]*TrieNode, 0, len(segments))

	// Navigate to the target node
	for _, segment := range segments {
		if _, exists := currentNode.children[segment]; !exists {
			// Topic path doesn't exist in the trie
			return
		}

		nodePath = append(nodePath, currentNode)
		currentNode = currentNode.children[segment]

		// If we reach a wildcard segment "#", it matches everything
		if segment == "#" {
			break
		}
	}

	// Delete the subscription from the final node's map
	// Check if the subscription exists before decrementing the counter
	_, subExists := currentNode.subscriptions[subscriptionID]
	delete(currentNode.subscriptions, subscriptionID)

	// Decrement the wildcard counter if this was a wildcard subscription
	if isWildcard && subExists {
		st.wildcardCount.Add(^uint32(0)) // Equivalent to -1 for atomic operations
	}

	// Clean up empty nodes
	if len(currentNode.subscriptions) == 0 && len(currentNode.children) == 0 {
		cleanupEmptyNodes(nodePath, segments)
	}
}

// cleanupEmptyNodes removes nodes that have no subscriptions and no children
func cleanupEmptyNodes(nodePath []*TrieNode, segments []string) {
	for i := len(nodePath) - 1; i >= 0; i-- {
		parentNode := nodePath[i]
		childSegment := segments[i]

		childNode := parentNode.children[childSegment]
		if len(childNode.subscriptions) == 0 && len(childNode.children) == 0 {
			delete(parentNode.children, childSegment)
		} else {
			// If we find a non-empty node, stop cleanup
			break
		}
	}
}

// findMatches recursively finds all matching subscriptions for a topic
func findMatches(node *TrieNode, segments []string, index int, result map[uint64]*Subscription) {
	// If we've reached a "#" node, it matches everything at this level and below
	if wildcard, exists := node.children["#"]; exists {
		// Manually copy map entries to avoid using maps.Copy which isn't concurrency-safe
		for id, sub := range wildcard.subscriptions {
			result[id] = sub
		}
	}

	// If we've processed all segments, add subscriptions at current node
	if index >= len(segments) {
		// Manually copy map entries to avoid using maps.Copy which isn't concurrency-safe
		for id, sub := range node.subscriptions {
			result[id] = sub
		}

		return
	}

	segment := segments[index]

	// Check for exact match
	if child, exists := node.children[segment]; exists {
		findMatches(child, segments, index+1, result)
	}

	// Check for "+" wildcard match (matches any single segment)
	if plus, exists := node.children["+"]; exists {
		findMatches(plus, segments, index+1, result)
	}
}

// FindMatchingSubscriptions returns all subscriptions that match a given topic
func (st *SubscriptionTrie) FindMatchingSubscriptions(topic string) []*Subscription {
	// Create a map to hold results (to avoid duplicates)
	resultMap := make(map[uint64]*Subscription)

	// First check exactMatches for a direct hit (fastest path)
	st.exactMatchMutex.RLock()
	if exactSubs, exists := st.exactMatches[topic]; exists {
		// Fast path: If there are no wildcard subscriptions, we can skip trie traversal
		if st.wildcardCount.Load() == 0 {
			// No wildcards in the system, just return exact matches
			result := make([]*Subscription, 0, len(exactSubs))
			for _, sub := range exactSubs {
				result = append(result, sub)
			}

			st.exactMatchMutex.RUnlock()

			return result
		}

		// Manually copy exact matches to result map rather than using maps.Copy
		for id, sub := range exactSubs {
			resultMap[id] = sub
		}
	}
	st.exactMatchMutex.RUnlock()

	// Check if we need to search for wildcard matches
	if st.wildcardCount.Load() > 0 {
		// Search the trie for wildcard matches
		segments := strings.Split(topic, "/")

		st.root.mutex.RLock()
		findMatches(st.root, segments, 0, resultMap)
		st.root.mutex.RUnlock()
	}

	// Convert result map to slice
	result := make([]*Subscription, 0, len(resultMap))
	for _, sub := range resultMap {
		result = append(result, sub)
	}

	return result
}
