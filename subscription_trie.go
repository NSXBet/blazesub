package blazesub

import (
	"strings"
	"sync"
)

// TrieSubscription is a subscription used in the trie implementation
// This is separate from the main Subscription type in subscriptions.go
type TrieSubscription struct {
	id            uint64
	topic         string
	handler       MessageHandler
	unsubscribeFn func() error
}

// ID returns the subscription ID
func (s *TrieSubscription) ID() uint64 {
	return s.id
}

// Topic returns the subscription topic
func (s *TrieSubscription) Topic() string {
	return s.topic
}

// SetUnsubscribeFunc sets the unsubscribe function
func (s *TrieSubscription) SetUnsubscribeFunc(fn func() error) {
	s.unsubscribeFn = fn
}

// Unsubscribe calls the unsubscribe function
func (s *TrieSubscription) Unsubscribe() error {
	if s.unsubscribeFn != nil {
		return s.unsubscribeFn()
	}
	return nil
}

// TrieNode represents a node in the subscription trie
type TrieNode struct {
	segment       string
	children      map[string]*TrieNode
	subscriptions map[uint64]*TrieSubscription // Map of subscriptions by ID at this node
	mutex         sync.RWMutex                 // For concurrency safety
}

// SubscriptionTrie is a trie-based structure for efficient topic subscriptions
type SubscriptionTrie struct {
	root *TrieNode
}

// NewSubscriptionTrie creates a new subscription trie
func NewSubscriptionTrie() *SubscriptionTrie {
	return &SubscriptionTrie{
		root: &TrieNode{
			segment:       "",
			children:      make(map[string]*TrieNode),
			subscriptions: make(map[uint64]*TrieSubscription),
		},
	}
}

// Subscribe adds a subscription for a topic pattern with internal subscription creation
func (st *SubscriptionTrie) Subscribe(id uint64, topic string, handler MessageHandler) *TrieSubscription {
	// Create the subscription with unsubscribe function that references this trie
	subscription := &TrieSubscription{
		id:      id,
		topic:   topic,
		handler: handler,
	}

	// Set the unsubscribe function to reference this trie
	unsubscribeFn := func() error {
		st.Unsubscribe(topic, id)
		return nil
	}
	subscription.SetUnsubscribeFunc(unsubscribeFn)

	segments := strings.Split(topic, "/")
	st.root.mutex.Lock()
	defer st.root.mutex.Unlock()

	currentNode := st.root

	for _, segment := range segments {
		if _, exists := currentNode.children[segment]; !exists {
			currentNode.children[segment] = &TrieNode{
				segment:       segment,
				children:      make(map[string]*TrieNode),
				subscriptions: make(map[uint64]*TrieSubscription),
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

	return subscription
}

// Unsubscribe removes a subscription for a topic pattern
func (st *SubscriptionTrie) Unsubscribe(topic string, subscriptionID uint64) {
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

	// Clean up empty nodes (optional, can be removed if unnecessary)
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

// FindMatchingSubscriptions returns all subscriptions that match a given topic
func (st *SubscriptionTrie) FindMatchingSubscriptions(topic string) []*TrieSubscription {
	segments := strings.Split(topic, "/")
	st.root.mutex.RLock()
	defer st.root.mutex.RUnlock()

	// Use a map to deduplicate subscriptions based on ID
	resultMap := make(map[uint64]*TrieSubscription)
	findMatches(st.root, segments, 0, resultMap)

	// Convert result map to slice
	result := make([]*TrieSubscription, 0, len(resultMap))
	for _, sub := range resultMap {
		result = append(result, sub)
	}
	return result
}

// findMatches recursively finds all matching subscriptions for a topic
func findMatches(node *TrieNode, segments []string, index int, result map[uint64]*TrieSubscription) {
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
