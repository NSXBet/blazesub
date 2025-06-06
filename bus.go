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

type EventBus[T any] interface {
	WaitReady(timeout time.Duration) error
	Publish(topic string, payload T, metadata ...map[string]any)
	Subscribe(topic string) (*Subscription[T], error)
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

type Bus[T any] struct {
	subID         *atomic.Uint64
	subscriptions *SubscriptionTrie[T]
	pubpool       *ants.Pool // Will be nil if UseGoroutinePool is false
	logger        logr.Logger

	// Cache recently used topics for faster lookup
	topicCache   *xsync.Map[string, []*Subscription[T]]
	maxCacheSize int

	maxConcurrentSubscriptions int
	useGoroutinePool           bool // Whether using worker pool or direct goroutines

	currentSubscriptionsCount *atomic.Uint64
	inflightMessagesCount     *atomic.Uint64
}

var _ EventBus[any] = (*Bus[any])(nil)

func NewBusOf[T any](config Config) (*Bus[T], error) {
	if config.Logger.IsZero() {
		logger, err := zap.NewProduction()
		if err != nil {
			return nil, fmt.Errorf("creating default logger: %w", err)
		}

		config.Logger = zapr.NewLogger(logger)
	}

	maxConcurrentSubscriptions := max(config.MaxConcurrentSubscriptions, DefaultMaxConcurrentSubscriptions)

	// Create a new bus instance
	bus := &Bus[T]{
		subID:                      atomic.NewUint64(0),
		subscriptions:              NewSubscriptionTrie[T](),
		logger:                     config.Logger,
		topicCache:                 xsync.NewMap[string, []*Subscription[T]](),
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

func NewBusWithDefaultsOf[T any]() (*Bus[T], error) {
	return NewBusOf[T](NewConfig())
}

type ByteBus = Bus[[]byte]

func NewBus(config Config) (*ByteBus, error) {
	return NewBusOf[[]byte](config)
}

func NewBusWithDefaults() (*ByteBus, error) {
	return NewBusWithDefaultsOf[[]byte]()
}

func (b *Bus[T]) Close() error {
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
func (b *Bus[T]) Publish(topic string, payload T, metadata ...map[string]any) {
	// Fast path for common empty topic case
	if topic == "" {
		return
	}

	var metadataMap map[string]any

	if len(metadata) > 0 {
		metadataMap = metadata[0]
	}

	// Create the message only once
	msg := &Message[T]{
		Topic:        topic,
		Data:         payload,
		UTCTimestamp: time.Now().UTC(),
		Metadata:     metadataMap,
	}

	// Try to get subscribers from cache first for common topics
	var matchingSubs []*Subscription[T]

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
				go func(s *Subscription[T]) {
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
func (b *Bus[T]) removeSubscription(topic string, subscriptionID uint64) error {
	// Remove the subscription from the trie
	err := b.subscriptions.Unsubscribe(topic, subscriptionID)
	if err != nil {
		return err
	}

	// Track the current number of subscriptions
	b.currentSubscriptionsCount.Sub(1)

	// Clear the topic from the cache to ensure fresh results
	// If this is a wildcard subscription, clear the entire topic cache
	// as it could affect any number of topics
	if strings.ContainsAny(topic, "+#") {
		// For wildcard topics, clear the entire cache as it might affect many topics
		b.topicCache.Clear()
	} else {
		// For exact topic matches, just remove that topic from the cache
		b.topicCache.Delete(topic)
	}

	return nil
}

// Subscribe creates a new subscription for the specified topic.
func (b *Bus[T]) Subscribe(topic string) (*Subscription[T], error) {
	// Generate a unique subscription ID
	subID := b.subID.Add(1)

	// Track the current number of subscriptions
	b.currentSubscriptionsCount.Add(1)

	// Use the trie's Subscribe method to create and register the subscription
	subscription := b.subscriptions.Subscribe(subID, topic, nil)

	// Set up unsubscribe function
	subscription.SetUnsubscribeFunc(b.removeSubscription)

	// Clear topic from cache to ensure it's refreshed on next publish
	if strings.ContainsAny(topic, "+#") {
		// For wildcard topics, clear the entire cache as it might affect many topics
		b.topicCache.Clear()
	} else {
		// For exact topic matches, just remove that topic from the cache
		b.topicCache.Delete(topic)
	}

	return subscription, nil
}

func (b *Bus[T]) SubscriptionsCount() uint64 {
	return b.currentSubscriptionsCount.Load()
}

func (b *Bus[T]) InflightMessagesCount() uint64 {
	return b.inflightMessagesCount.Load()
}

const waitReadyInterval = time.Millisecond * 10

// WaitReady waits for the Bus to be ready to handle messages.
//
// This method is useful during application startup to ensure the bus is ready before
// publishing messages. For direct goroutine mode (UseGoroutinePool = false), the bus
// is always ready and this method returns immediately. For worker pool mode, this method
// waits until the worker pool is properly initialized.
//
// The timeout parameter specifies how long to wait for the bus to become ready.
// Returns nil when the bus is ready, or ErrTimeoutWaitReady if the timeout expires.
func (b *Bus[T]) WaitReady(timeout time.Duration) error {
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

		time.Sleep(waitReadyInterval)
	}

	return ErrTimeoutWaitReady
}
