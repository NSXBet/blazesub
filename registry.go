package blazesub

import (
	"context"
	"strings"
	"sync"
)

// matchTopic checks if a topic matches a filter with wildcards
func (r *Registry) matchTopic(filter, topic string) bool {
	// Exact match (case insensitive)
	if strings.EqualFold(filter, topic) {
		return true
	}

	if filter == "#" {
		return true
	}

	if filter == "+" && !strings.Contains(topic, "/") {
		return true
	}

	// Simple topic validation
	if strings.Contains(topic, "//") || strings.HasSuffix(topic, "/") || strings.HasPrefix(topic, "/") {
		return false
	}

	filterParts := strings.Split(filter, "/")
	topicParts := strings.Split(topic, "/")

	// Handle # wildcard (matches any level)
	if filterParts[len(filterParts)-1] == "#" {
		// Remove the # part for comparison
		filterParts = filterParts[:len(filterParts)-1]

		// Topic must be at least as long as the filter without the #
		if len(topicParts) < len(filterParts) {
			return false
		}

		// Check all parts before the # for exact match or + wildcard
		for i := range len(filterParts) {
			if filterParts[i] != "+" && !strings.EqualFold(filterParts[i], topicParts[i]) {
				return false
			}
		}

		return true
	}

	// If no # wildcard, the number of parts must match
	if len(filterParts) != len(topicParts) {
		return false
	}

	// Check each part for exact match or + wildcard
	for i := range len(filterParts) {
		if filterParts[i] != "+" && !strings.EqualFold(filterParts[i], topicParts[i]) {
			return false
		}
	}

	return true
}

// SubscriptionCallback is the function type for handling subscription matches
type SubscriptionCallback func(ctx context.Context, subscription *Subscription) error

// Registry manages topic subscriptions with wildcard support
type Registry struct {
	mu            sync.RWMutex
	subscriptions map[int]struct {
		filter       string
		subscription *Subscription
	}
	filterCache      map[string][]int // Cache for fast lookup: topic -> []subscriptionID
	cacheInvalidated bool
	topicFilter      TopicFilter // Filter validator
}

// NewRegistry creates a new subscription registry
func NewRegistry() *Registry {
	return &Registry{
		subscriptions: make(map[int]struct {
			filter       string
			subscription *Subscription
		}),
		filterCache:      make(map[string][]int),
		cacheInvalidated: false,
		topicFilter:      NewTopicFilter(),
	}
}

// RegisterSubscription registers a new subscription with the given filter and ID
func (r *Registry) RegisterSubscription(filter string, id int, subscription *Subscription) error {
	// Validate original filter (with proper case)
	if err := r.topicFilter.Validate(filter); err != nil {
		return err
	}

	// Convert filter to lowercase for case insensitivity in storage and matching
	filter = strings.ToLower(filter)

	r.mu.Lock()
	defer r.mu.Unlock()

	r.subscriptions[id] = struct {
		filter       string
		subscription *Subscription
	}{
		filter:       filter,
		subscription: subscription,
	}

	// Mark cache as invalidated
	r.cacheInvalidated = true

	return nil
}

// RemoveSubscription removes a subscription by ID
func (r *Registry) RemoveSubscription(id int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.subscriptions, id)

	// Mark cache as invalidated
	r.cacheInvalidated = true
}

// rebuildCache rebuilds the filter cache for faster lookups
// Should be called with mutex locked
func (r *Registry) rebuildCache() {
	// Clear the existing cache
	r.filterCache = make(map[string][]int)

	// No need to rebuild if no subscriptions
	if len(r.subscriptions) == 0 {
		r.cacheInvalidated = false
		return
	}

	// Group subscriptions by exact filters for faster lookup
	for id, sub := range r.subscriptions {
		// We don't put wildcard filters in the cache as they need special matching
		if !strings.Contains(sub.filter, "+") && !strings.Contains(sub.filter, "#") {
			r.filterCache[sub.filter] = append(r.filterCache[sub.filter], id)
		}
	}

	r.cacheInvalidated = false
}

// ExecuteSubscriptions executes the callback for all subscriptions matching the topic
func (r *Registry) ExecuteSubscriptions(topic string, callback SubscriptionCallback) error {
	// Validate topic
	if err := r.topicFilter.ValidateTopic(topic); err != nil {
		return nil // No matches for invalid topics
	}

	// Convert topic to lowercase for case insensitivity
	topic = strings.ToLower(topic)

	r.mu.RLock()

	// Rebuild cache if invalidated
	if r.cacheInvalidated {
		// Upgrade to write lock
		r.mu.RUnlock()
		r.mu.Lock()

		// Double-check after acquiring write lock
		if r.cacheInvalidated {
			r.rebuildCache()
		}

		// Downgrade to read lock
		r.mu.Unlock()
		r.mu.RLock()
	}

	// First check exact match in cache for performance
	matchedIDs := make(map[int]bool)

	// Add exact matches from cache
	if ids, ok := r.filterCache[topic]; ok {
		for _, id := range ids {
			matchedIDs[id] = true
		}
	}

	// Check all wildcard subscriptions
	for id, sub := range r.subscriptions {
		// Skip if already matched
		if matchedIDs[id] {
			continue
		}

		// Check if the filter contains wildcards
		if strings.Contains(sub.filter, "+") || strings.Contains(sub.filter, "#") {
			if r.matchTopic(sub.filter, topic) {
				matchedIDs[id] = true
			}
		}
	}

	// Create a snapshot of matches to release the lock sooner
	matchedSubs := make([]*Subscription, 0, len(matchedIDs))
	for id := range matchedIDs {
		matchedSubs = append(matchedSubs, r.subscriptions[id].subscription)
	}

	r.mu.RUnlock()

	// Execute callbacks outside the lock
	ctx := context.Background()
	for _, sub := range matchedSubs {
		if err := callback(ctx, sub); err != nil {
			return err
		}
	}

	return nil
}
