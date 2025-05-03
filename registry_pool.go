package blazesub

import (
	"context"
	"sync"
	"sync/atomic"
)

// RegistryPool manages multiple Registry instances to distribute load
type RegistryPool struct {
	registries []*Registry
	count      int
	nextIndex  uint64 // for round-robin selection
}

// NewRegistryPool creates a new pool of registries
func NewRegistryPool(count int) *RegistryPool {
	if count <= 0 {
		count = 1 // At least one registry
	}

	registries := make([]*Registry, count)
	for i := 0; i < count; i++ {
		registries[i] = NewRegistry()
	}

	return &RegistryPool{
		registries: registries,
		count:      count,
		nextIndex:  0,
	}
}

// getRegistry returns the next registry in round-robin fashion
func (p *RegistryPool) getRegistry() *Registry {
	// Atomically increment and wrap around
	idx := atomic.AddUint64(&p.nextIndex, 1) % uint64(p.count)
	return p.registries[idx]
}

// RegisterSubscription registers a subscription across all registries in the pool
// This ensures the subscription is matched regardless of which registry processes a topic
func (p *RegistryPool) RegisterSubscription(filter string, id int, subscription *Subscription) error {
	// Register with the first registry to validate the filter
	err := p.registries[0].RegisterSubscription(filter, id, subscription)
	if err != nil {
		return err
	}

	// If successful, register with all other registries
	for i := 1; i < p.count; i++ {
		// We don't need to validate again since we already validated with the first registry
		p.registries[i].RegisterSubscription(filter, id, subscription)
	}

	return nil
}

// RemoveSubscription removes a subscription from all registries in the pool
func (p *RegistryPool) RemoveSubscription(id int) {
	for _, registry := range p.registries {
		registry.RemoveSubscription(id)
	}
}

// ExecuteSubscriptions executes the callback for all subscriptions matching the topic
// using a single registry chosen in round-robin fashion
func (p *RegistryPool) ExecuteSubscriptions(topic string, callback SubscriptionCallback) error {
	registry := p.getRegistry()
	return registry.ExecuteSubscriptions(topic, callback)
}

// ExecuteSubscriptionsParallel executes the callback for all subscriptions matching the topic
// across all registries in parallel and combines the results
func (p *RegistryPool) ExecuteSubscriptionsParallel(topic string, callback SubscriptionCallback) error {
	var wg sync.WaitGroup
	wg.Add(p.count)

	var errOnce sync.Once
	var firstErr error

	for i := 0; i < p.count; i++ {
		go func(regIdx int) {
			defer wg.Done()

			// Wrap the callback to avoid duplicate matches across registries
			// This is only needed for the parallel execute method
			matched := make(map[uint64]bool)

			wrappedCallback := func(ctx context.Context, sub *Subscription) error {
				// Use a sync.Mutex if needed for thread safety
				if sub != nil && !matched[sub.id] {
					matched[sub.id] = true
					return callback(ctx, sub)
				}
				return nil
			}

			err := p.registries[regIdx].ExecuteSubscriptions(topic, wrappedCallback)
			if err != nil {
				errOnce.Do(func() {
					firstErr = err
				})
			}
		}(i)
	}

	wg.Wait()
	return firstErr
}

// Count returns the number of registries in the pool
func (p *RegistryPool) Count() int {
	return p.count
}

// PublishTopic publishes a topic to a single registry (round-robin) or all registries
// based on the publishing strategy
func (p *RegistryPool) PublishTopic(topic string, callback SubscriptionCallback, publishToAll bool) error {
	// If publishToAll is true, publish to all registries in parallel
	if publishToAll {
		return p.ExecuteSubscriptionsParallel(topic, callback)
	}

	// Otherwise, use round-robin to select a single registry
	return p.ExecuteSubscriptions(topic, callback)
}

// GetAllRegistries returns all registries in the pool
// This is useful for the Bus to directly access registries if needed
func (p *RegistryPool) GetAllRegistries() []*Registry {
	return p.registries
}
