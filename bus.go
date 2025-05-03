package blazesub

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/panjf2000/ants/v2"
	"github.com/puzpuzpuz/xsync/v4"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

type MessageHandler interface {
	OnMessage(message *Message) error
}

type Bus struct {
	subID         *atomic.Uint64
	subscriptions *SubscriptionTrie
	pubpool       *ants.Pool
	logger        logr.Logger
	// Cache recently used topics for faster lookup
	topicCache   *xsync.Map[string, []*Subscription]
	maxCacheSize int
}

func NewBus(config Config) (*Bus, error) {
	if config.Logger.IsZero() {
		logger, err := zap.NewProduction()
		if err != nil {
			return nil, fmt.Errorf("creating default logger: %w", err)
		}

		config.Logger = zapr.NewLogger(logger)
	}

	// Optimize pool options for maximum performance
	options := []ants.Option{
		ants.WithPanicHandler(func(err any) {
			typedErr, ok := err.(error)
			if !ok {
				return
			}

			config.Logger.Error(typedErr, "panic in publish pool")
		}),
		ants.WithPreAlloc(true), // Always preallocate for better performance
		ants.WithMaxBlockingTasks(config.MaxBlockingTasks),
		ants.WithNonblocking(true), // Use non-blocking mode for high performance
	}

	if config.ExpiryDuration > 0 {
		options = append(options, ants.WithExpiryDuration(config.ExpiryDuration))
	}

	// Set a minimum pool size for better performance
	poolSize := config.WorkerCount
	if poolSize < 20000 {
		poolSize = 20000 // Ensure we have enough workers for high throughput
	}

	pool, err := ants.NewPool(
		poolSize,
		options...,
	)
	if err != nil {
		return nil, fmt.Errorf("creating publish pool: %w", err)
	}

	return &Bus{
		subID:         atomic.NewUint64(0),
		subscriptions: NewSubscriptionTrie(),
		pubpool:       pool,
		logger:        config.Logger,
		topicCache:    xsync.NewMap[string, []*Subscription](),
		maxCacheSize:  2000, // Increase cache size for better hit rate
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
	// Fast path for common empty topic case
	if topic == "" {
		return
	}

	// Create the message only once
	msg := &Message{
		Topic:        topic,
		Data:         payload,
		UTCTimestamp: time.Now().UTC(),
	}

	// Try to get subscribers from cache first for common topics
	var matchingSubs []*Subscription

	if cachedSubs, exists := b.topicCache.Load(topic); exists {
		matchingSubs = cachedSubs
	} else {
		// Find matching subscriptions using the trie
		matchingSubs = b.subscriptions.FindMatchingSubscriptions(topic)

		// Cache the result if there aren't too many cached topics already
		if b.topicCache.Size() < b.maxCacheSize {
			b.topicCache.Store(topic, matchingSubs)
		}
	}

	// Fast path: if there are no subscribers, return immediately
	subscriberCount := len(matchingSubs)
	if subscriberCount == 0 {
		return
	}

	// Fast path for single subscriber (very common case)
	if subscriberCount == 1 {
		subscription := matchingSubs[0]
		// Submit directly without wrapping in a closure when possible
		_ = b.pubpool.Submit(func() {
			_ = subscription.receiveMessage(msg)
		})
		return
	}

	// For a small number of subscribers, submit them directly
	if subscriberCount <= 8 {
		for _, subscription := range matchingSubs {
			sub := subscription // Local copy for closure
			_ = b.pubpool.Submit(func() {
				_ = sub.receiveMessage(msg)
			})
		}
		return
	}

	// For larger subscriber sets, batch them to reduce pool overhead
	// but still use the pool to avoid blocking
	_ = b.pubpool.Submit(func() {
		// Process all subscriptions without further overhead
		for _, subscription := range matchingSubs {
			// Ignore errors in this fast path
			_ = subscription.receiveMessage(msg)
		}
	})
}

// removeSubscription removes a subscription from the trie.
func (b *Bus) removeSubscription(topic string, subscriptionID uint64) {
	b.subscriptions.Unsubscribe(topic, subscriptionID)

	// If this is a wildcard subscription, we need to clear the entire topic cache
	// as it could affect any number of topics
	if strings.ContainsAny(topic, "+#") {
		// For wildcard topics, clear the entire cache as it might affect many topics
		b.topicCache = xsync.NewMap[string, []*Subscription]()
	} else {
		// For exact topic matches, just remove that topic from the cache
		b.topicCache.Delete(topic)
	}
}

// Subscribe creates a new subscription for the specified topic.
func (b *Bus) Subscribe(topic string) (*Subscription, error) {
	subID := b.subID.Add(1)

	// Use the trie's Subscribe method to create and register the subscription
	subscription := b.subscriptions.Subscribe(subID, topic, nil)

	// Remove from topic cache to ensure it's refreshed on next publish
	b.topicCache.Delete(topic)

	// Set up unsubscribe function
	unsubscribeFn := func() error {
		b.removeSubscription(topic, subID)
		return nil
	}
	subscription.SetUnsubscribeFunc(unsubscribeFn)

	return subscription, nil
}
