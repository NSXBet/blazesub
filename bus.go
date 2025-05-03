package blazesub

import (
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/panjf2000/ants/v2"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

type MessageHandler interface {
	OnMessage(message *Message) error
}

type Bus struct {
	subID         *atomic.Uint64
	subscriptions *SubscriptionTrie
	subLock       sync.RWMutex
	pubpool       *ants.Pool
	logger        logr.Logger
}

func NewBus(config Config) (*Bus, error) {
	if config.Logger.IsZero() {
		logger, err := zap.NewProduction()
		if err != nil {
			return nil, fmt.Errorf("creating default logger: %w", err)
		}

		config.Logger = zapr.NewLogger(logger)
	}

	options := []ants.Option{
		ants.WithPanicHandler(func(err any) {
			typedErr, ok := err.(error)
			if !ok {
				return
			}

			config.Logger.Error(typedErr, "panic in publish pool")
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

	return &Bus{
		subID:         atomic.NewUint64(0),
		subscriptions: NewSubscriptionTrie(),
		subLock:       sync.RWMutex{},
		pubpool:       pool,
	}, nil
}

func NewBusWithDefaults() (*Bus, error) {
	return NewBus(NewConfig())
}

const (
	closeTimeout  = time.Second * 10
	closeInterval = time.Millisecond * 10
)

func (b *Bus) Close() error {
	defer b.pubpool.Release()

	startTime := time.Now()
	for b.pubpool.Waiting() > 0 {
		if time.Since(startTime) > closeTimeout {
			return ErrTimeoutClosingBus
		}

		time.Sleep(closeInterval)
	}

	return nil
}

// Publish publishes a message to subscribers of the specified topic.
func (b *Bus) Publish(topic string, payload []byte) {
	msg := &Message{
		Topic:        topic,
		Data:         payload,
		UTCTimestamp: time.Now().UTC(),
	}

	// Find matching subscriptions using the trie
	b.subLock.RLock()
	matchingSubs := b.subscriptions.FindMatchingSubscriptions(topic)
	b.subLock.RUnlock()

	// Submit a task to the pool for each subscription
	for _, sub := range matchingSubs {
		subscription := sub // Create a local copy for the closure

		if err := b.pubpool.Submit(func() {
			if err := subscription.receiveMessage(msg); err != nil {
				b.logger.Error(err, "error receiving message", "topic", topic)
			}
		}); err != nil {
			b.logger.Error(err, "error submitting message to pool", "topic", topic)
		}
	}
}

// removeSubscription removes a subscription from the trie.
func (b *Bus) removeSubscription(topic string, subscriptionID uint64) {
	b.subLock.Lock()
	defer b.subLock.Unlock()

	b.subscriptions.Unsubscribe(topic, subscriptionID)
}

// Subscribe creates a new subscription for the specified topic.
func (b *Bus) Subscribe(topic string) (*Subscription, error) {
	subID := b.subID.Add(1)

	b.subLock.Lock()
	defer b.subLock.Unlock()

	// Use the trie's Subscribe method to create and register the subscription
	subscription := b.subscriptions.Subscribe(subID, topic, nil)

	// Set up unsubscribe function
	unsubscribeFn := func() error {
		b.removeSubscription(topic, subID)

		return nil
	}
	subscription.SetUnsubscribeFunc(unsubscribeFn)

	return subscription, nil
}
