package blazesub

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/panjf2000/ants/v2"
	"go.uber.org/atomic"
)

type MessageHandler interface {
	OnMessage(message *Message) error
}

type Bus struct {
	subID         *atomic.Uint64
	subscriptions *SubscriptionTrie
	subLock       sync.RWMutex
	pubpool       *ants.Pool
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

func (b *Bus) Close() error {
	defer b.pubpool.Release()

	for b.pubpool.Waiting() > 0 {
		time.Sleep(time.Millisecond * 10)
	}

	// TODO: wait for all messages to be processed

	return nil
}

// Publish publishes a message to subscribers of the specified topic
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
				fmt.Printf("error receiving message: %s", err)
			}
		}); err != nil {
			fmt.Printf("error submitting message to pool: %s", err)
		}
	}
}

// removeSubscription removes a subscription from the trie
func (b *Bus) removeSubscription(topic string, subscriptionID uint64) {
	b.subLock.Lock()
	defer b.subLock.Unlock()

	b.subscriptions.Unsubscribe(topic, subscriptionID)
}

// Subscribe creates a new subscription for the specified topic
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

// addSubscriptionToTrie adds a subscription to the trie
func (b *Bus) addSubscriptionToTrie(subscription *Subscription) {
	topic := subscription.Topic()
	segments := strings.Split(topic, "/")
	currentNode := b.subscriptions.root

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
	currentNode.subscriptions[subscription.ID()] = subscription
}
