package blazesub

import (
	"context"
	"fmt"
	"time"

	"github.com/panjf2000/ants/v2"
	"go.uber.org/atomic"
)

type MessageHandler interface {
	OnMessage(message *Message) error
}

type Bus struct {
	subID    *atomic.Uint64
	registry Registry
	pubpool  *ants.Pool
}

func NewBus(config Config) (*Bus, error) {
	options := []ants.Option{
		ants.WithPanicHandler(func(err any) {
			// TODO: log error
			fmt.Printf("panic in publish pool: %s", err)
		}),
		ants.WithPreAlloc(config.PreAlloc),
		ants.WithMaxBlockingTasks(config.MaxBlockingTasks),
	}

	if config.ExpiryDuration > 0 {
		options = append(options, ants.WithExpiryDuration(config.ExpiryDuration))
	}

	pool, err := ants.NewPool(
		config.WorkerCount,
		options...,
	)
	if err != nil {
		return nil, fmt.Errorf("creating publish pool: %w", err)
	}

	// Create registry pool with specified size
	var registry Registry

	if config.RegistryPoolSize > 1 {
		registry = NewRegistryPool(config.RegistryPoolSize)
	} else {
		registry = NewRegistry()
	}

	return &Bus{
		subID:    atomic.NewUint64(0),
		registry: registry,
		pubpool:  pool,
	}, nil
}

func NewBusWithDefaults() (*Bus, error) {
	return NewBus(NewConfig())
}

func (b *Bus) Close() error {
	defer b.pubpool.Release()

	// TODO: wait for all messages to be processed

	return nil
}

// Publish publishes a message to the specified topic
func (b *Bus) Publish(topic string, payload []byte) {
	msg := &Message{
		Topic:        topic,
		Data:         payload,
		UTCTimestamp: time.Now().UTC(),
	}

	// Use the registry pool to find and execute matching subscriptions
	err := b.registry.ExecuteSubscriptions(topic, func(ctx context.Context, sub *Subscription) error {
		// Submit the message processing to the publish pool
		return b.pubpool.Submit(func() {
			if err := sub.receiveMessage(msg); err != nil {
				fmt.Printf("error receiving message: %s", err)
			}
		})
	})
	if err != nil {
		fmt.Printf("error publishing message: %s", err)
	}
}

// Subscribe creates a new subscription for the specified topic
func (b *Bus) Subscribe(topic string) (*Subscription, error) {
	// Create a new subscription
	subscription := &Subscription{
		id:    b.subID.Add(1),
		topic: topic,
		bus:   b,
	}

	// Register the subscription with the registry pool
	err := b.registry.RegisterSubscription(topic, int(subscription.id), subscription)
	if err != nil {
		return nil, fmt.Errorf("registering subscription: %w", err)
	}

	return subscription, nil
}

// removeSubscription removes a subscription by ID and topic
func (b *Bus) removeSubscription(subscriptionID uint64) {
	b.registry.RemoveSubscription(int(subscriptionID))
}
