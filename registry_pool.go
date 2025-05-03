package blazesub

import (
	"context"
	"sync"
)

// RegistryPool manages multiple Registry instances to distribute load
type RegistryPool struct {
	registries []Registry
	count      int
}

var _ Registry = (*RegistryPool)(nil)

// NewRegistryPool creates a new pool of registries
func NewRegistryPool(count int) *RegistryPool {
	if count <= 0 {
		count = 1 // At least one registry
	}

	registries := make([]Registry, count)
	for i := range count {
		registries[i] = NewRegistry()
	}

	return &RegistryPool{
		registries: registries,
		count:      count,
	}
}

// RegisterSubscription registers a subscription with a single registry
// selected by hashing the subscription ID
func (p *RegistryPool) RegisterSubscription(filter string, id int, subscription *Subscription) error {
	// Choose registry based on hash of ID
	regIdx := id % p.count

	// Register with the chosen registry
	return p.registries[regIdx].RegisterSubscription(filter, id, subscription)
}

// RemoveSubscription removes a subscription from all registries in the pool
func (p *RegistryPool) RemoveSubscription(id int) {
	// Choose registry based on hash of ID
	regIdx := id % p.count

	// Remove from the chosen registry
	p.registries[regIdx].RemoveSubscription(id)
}

// ExecuteSubscriptions executes the callback for all subscriptions matching the topic
// across all registries sequentially
func (p *RegistryPool) ExecuteSubscriptions(topic string, callback SubscriptionCallback) error {
	// Process each registry one by one
	for _, registry := range p.registries {
		err := registry.ExecuteSubscriptions(topic, callback)
		if err != nil {
			return err
		}
	}
	return nil
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

// GetAllRegistries returns all registries in the pool
// This is useful for the Bus to directly access registries if needed
func (p *RegistryPool) GetAllRegistries() []Registry {
	return p.registries
}
