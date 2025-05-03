package blazesub

import (
	"strings"
	"sync"
)

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
	wildcardExists  bool                                // Flag to track if any wildcard subscriptions exist
	wildcardMutex   sync.RWMutex                        // Mutex for the wildcard exists flag
}

// NewSubscriptionTrie creates a new subscription trie
func NewSubscriptionTrie() *SubscriptionTrie {
	return &SubscriptionTrie{
		root: &TrieNode{
			segment:       "",
			children:      make(map[string]*TrieNode),
			subscriptions: make(map[uint64]*Subscription),
		},
		exactMatches: make(
			map[string]map[uint64]*Subscription,
			1024,
		), // Pre-allocate with larger capacity for better performance
		wildcardExists: false,
	}
}

// hasWildcard checks if a subscription pattern contains wildcard characters
func hasWildcard(pattern string) bool {
	return strings.ContainsAny(pattern, "+#")
}

// Subscribe adds a subscription for a topic pattern
func (st *SubscriptionTrie) Subscribe(id uint64, topic string, handler MessageHandler) *Subscription {
	// Create a new subscription
	subscription := &Subscription{
		id:      id,
		topic:   topic,
		handler: handler,
	}

	// Set the unsubscribe function that references this trie
	unsubscribeFn := func() error {
		st.Unsubscribe(topic, id)
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
			}
		}

		currentNode = currentNode.children[segment]

		// If we reach a wildcard segment "#", it matches everything at this level and below
		if segment == "#" {
			break
		}
	}

	// Add the subscription to the final node's map
	currentNode.subscriptions[id] = subscription
	st.root.mutex.Unlock()

	// Set the wildcard flag to true if this is a wildcard subscription
	if isWildcard {
		st.wildcardMutex.Lock()
		st.wildcardExists = true
		st.wildcardMutex.Unlock()
	}

	// If it's an exact match, also add to the exactMatches map
	if !isWildcard {
		st.exactMatchMutex.Lock()
		if _, exists := st.exactMatches[topic]; !exists {
			st.exactMatches[topic] = make(map[uint64]*Subscription)
		}
		st.exactMatches[topic][id] = subscription
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
	var nodePath []*TrieNode

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
	delete(currentNode.subscriptions, subscriptionID)

	// Clean up empty nodes
	if len(currentNode.subscriptions) == 0 && len(currentNode.children) == 0 {
		cleanupEmptyNodes(nodePath, segments)
	}

	// If this was a wildcard, check if we still have wildcards
	if isWildcard {
		st.checkForRemainingWildcards()
	}
}

// checkForRemainingWildcards checks if there are any wildcard subscriptions left
// Must be called with the root mutex locked
func (st *SubscriptionTrie) checkForRemainingWildcards() {
	// This is a simplistic check - we could make this more efficient by keeping a count
	// of wildcard subscriptions, but this is good enough for now

	// Do a quick check of the common wildcard nodes first
	if hashNode, exists := st.root.children["#"]; exists && len(hashNode.subscriptions) > 0 {
		return // We still have wildcards
	}

	if plusNode, exists := st.root.children["+"]; exists && len(plusNode.subscriptions) > 0 {
		return // We still have wildcards
	}

	// Do a breadth-first search to look for any nodes with + or # children
	queue := make([]*TrieNode, 0)
	queue = append(queue, st.root)

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		// Check if this node has any wildcard children
		if hashNode, hasHash := node.children["#"]; hasHash && len(hashNode.subscriptions) > 0 {
			return // Found a wildcard
		}

		if plusNode, hasPlus := node.children["+"]; hasPlus && len(plusNode.subscriptions) > 0 {
			return // Found a wildcard
		}

		// Add all non-wildcard children to the queue
		for segment, child := range node.children {
			if segment != "#" && segment != "+" {
				queue = append(queue, child)
			}
		}
	}

	// If we get here, there are no more wildcards
	st.wildcardMutex.Lock()
	st.wildcardExists = false
	st.wildcardMutex.Unlock()
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

// FindMatchingSubscriptions returns all subscriptions that match a given topic
func (st *SubscriptionTrie) FindMatchingSubscriptions(topic string) []*Subscription {
	// Create a map to hold results (to avoid duplicates)
	resultMap := make(map[uint64]*Subscription)

	// First check exactMatches for a direct hit (fastest path)
	st.exactMatchMutex.RLock()
	if exactSubs, exists := st.exactMatches[topic]; exists {
		// Fast path: If there are no wildcard subscriptions, we can skip trie traversal
		st.wildcardMutex.RLock()
		if !st.wildcardExists {
			// No wildcards in the system, just return exact matches
			st.wildcardMutex.RUnlock()
			st.exactMatchMutex.RUnlock()

			result := make([]*Subscription, 0, len(exactSubs))
			for _, sub := range exactSubs {
				result = append(result, sub)
			}
			return result
		}
		st.wildcardMutex.RUnlock()

		// Add exact matches to result
		for id, sub := range exactSubs {
			resultMap[id] = sub
		}
	}
	st.exactMatchMutex.RUnlock()

	// Check if we need to search for wildcard matches
	st.wildcardMutex.RLock()
	wildcardExists := st.wildcardExists
	st.wildcardMutex.RUnlock()

	if wildcardExists {
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

// findMatches recursively finds all matching subscriptions for a topic
func findMatches(node *TrieNode, segments []string, index int, result map[uint64]*Subscription) {
	// If we've reached a "#" node, it matches everything at this level and below
	if wildcard, exists := node.children["#"]; exists {
		for id, sub := range wildcard.subscriptions {
			result[id] = sub
		}
	}

	// If we've processed all segments, add subscriptions at current node
	if index >= len(segments) {
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
