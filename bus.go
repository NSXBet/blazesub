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

type EventBus interface {
	WaitReady(timeout time.Duration) error
	Publish(topic string, payload []byte)
	Subscribe(topic string) (*Subscription, error)
	Close() error

	// Stats
	SubscriptionsCount() uint64
	InflightMessagesCount() uint64
}

const (
	MinimumWorkerCount  = 20000
	DefaultMaxCacheSize = 2000

	closeTimeout  = time.Second * 10
	closeInterval = time.Millisecond * 10
)

type MessageHandler interface {
	OnMessage(message *Message) error
}

type Bus struct {
	subID         *atomic.Uint64
	subscriptions *SubscriptionTrie
	pubpool       *ants.Pool // Will be nil if UseGoroutinePool is false
	logger        logr.Logger

	// Cache recently used topics for faster lookup
	topicCache   *xsync.Map[string, []*Subscription]
	maxCacheSize int

	maxConcurrentSubscriptions int
	useGoroutinePool           bool // Whether using worker pool or direct goroutines

	currentSubscriptionsCount *atomic.Uint64
	inflightMessagesCount     *atomic.Uint64
}

var _ EventBus = (*Bus)(nil)

func NewBus(config Config) (*Bus, error) {
	if config.Logger.IsZero() {
		logger, err := zap.NewProduction()
		if err != nil {
			return nil, fmt.Errorf("creating default logger: %w", err)
		}

		config.Logger = zapr.NewLogger(logger)
	}

	maxConcurrentSubscriptions := max(config.MaxConcurrentSubscriptions, DefaultMaxConcurrentSubscriptions)

	// Create a new bus instance
	bus := &Bus{
		subID:                      atomic.NewUint64(0),
		subscriptions:              NewSubscriptionTrie(),
		logger:                     config.Logger,
		topicCache:                 xsync.NewMap[string, []*Subscription](),
		maxCacheSize:               DefaultMaxCacheSize,
		maxConcurrentSubscriptions: maxConcurrentSubscriptions,
		useGoroutinePool:           config.UseGoroutinePool,
		currentSubscriptionsCount:  atomic.NewUint64(0),
		inflightMessagesCount:      atomic.NewUint64(0),
	}

	// Only initialize worker pool if needed
	if config.UseGoroutinePool {
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
		poolSize := max(config.WorkerCount, MinimumWorkerCount)

		pool, err := ants.NewPool(
			poolSize,
			options...,
		)
		if err != nil {
			return nil, fmt.Errorf("creating publish pool: %w", err)
		}

		bus.pubpool = pool
	}

	return bus, nil
}

func NewBusWithDefaults() (*Bus, error) {
	return NewBus(NewConfig())
}

func (b *Bus) Close() error {
	// Only release pool if it exists
	if b.pubpool != nil {
		defer b.pubpool.Release()

		startTime := time.Now()
		for b.pubpool.Waiting() > 0 {
			if time.Since(startTime) > closeTimeout {
				return ErrTimeoutClosingBus
			}

			time.Sleep(closeInterval)
		}
	}

	return nil
}

// Publish publishes a message to subscribers of the specified topic.
//
//nolint:gocognit // reason: code is optimized for performance.
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

	b.inflightMessagesCount.Add(uint64(subscriberCount))

	// Fast path for single subscriber (very common case)
	if subscriberCount == 1 {
		subscription := matchingSubs[0]

		if b.useGoroutinePool {
			// Use worker pool
			_ = b.pubpool.Submit(func() {
				defer b.inflightMessagesCount.Sub(1)

				_ = subscription.receiveMessage(msg)
			})
		} else {
			// Use direct goroutine
			go func() {
				defer b.inflightMessagesCount.Sub(1)

				_ = subscription.receiveMessage(msg)
			}()
		}
		return
	}

	// For a small number of subscribers, submit them individually
	if subscriberCount <= b.maxConcurrentSubscriptions {
		for _, subscription := range matchingSubs {
			sub := subscription // Local copy for closure

			if b.useGoroutinePool {
				// Use worker pool
				_ = b.pubpool.Submit(func() {
					defer b.inflightMessagesCount.Sub(1)

					_ = sub.receiveMessage(msg)
				})
			} else {
				// Use direct goroutine
				go func(s *Subscription) {
					defer b.inflightMessagesCount.Sub(1)

					_ = s.receiveMessage(msg)
				}(sub)
			}
		}
		return
	}

	// For larger subscriber sets, batch them to reduce overhead
	if b.useGoroutinePool {
		// Use worker pool with batching
		_ = b.pubpool.Submit(func() {
			// Process all subscriptions in a single task
			for _, subscription := range matchingSubs {
				_ = subscription.receiveMessage(msg)

				b.inflightMessagesCount.Sub(1)
			}
		})
	} else {
		// Use a single goroutine for batching
		go func() {
			// Process all subscriptions in a single goroutine
			for _, subscription := range matchingSubs {
				_ = subscription.receiveMessage(msg)

				b.inflightMessagesCount.Sub(1)
			}
		}()
	}
}

// removeSubscription removes a subscription from the trie.
func (b *Bus) removeSubscription(topic string, subscriptionID uint64) {
	b.currentSubscriptionsCount.Sub(1)
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

	b.currentSubscriptionsCount.Add(1)

	return subscription, nil
}

func (b *Bus) SubscriptionsCount() uint64 {
	return b.currentSubscriptionsCount.Load()
}

func (b *Bus) InflightMessagesCount() uint64 {
	return b.inflightMessagesCount.Load()
}

// WaitReady waits for the Bus to be ready to handle messages.
//
// This method is useful during application startup to ensure the bus is ready before
// publishing messages. For direct goroutine mode (UseGoroutinePool = false), the bus
// is always ready and this method returns immediately. For worker pool mode, this method
// waits until the worker pool is properly initialized.
//
// The timeout parameter specifies how long to wait for the bus to become ready.
// Returns nil when the bus is ready, or ErrTimeoutWaitReady if the timeout expires.
func (b *Bus) WaitReady(timeout time.Duration) error {
	// If direct goroutines mode is used, the bus is always ready
	if !b.useGoroutinePool {
		return nil
	}

	// When using goroutine pool, wait for the pool to be ready
	if b.pubpool == nil {
		return ErrTimeoutWaitReady
	}

	// Wait for the pool to be ready within the specified timeout
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		// Check if the worker pool is ready
		if !b.pubpool.IsClosed() {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}

	return ErrTimeoutWaitReady
}
