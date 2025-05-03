package blazesub

import (
	"fmt"
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
	subscriptions map[string]map[uint64][]*Subscription
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
		subscriptions: map[string]map[uint64][]*Subscription{},
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

// TODO: turn data into strong type
// TODO: error handling
func (b *Bus) Publish(topic string, payload []byte) {
	msg := &Message{
		Topic:        topic,
		Data:         payload,
		UTCTimestamp: time.Now().UTC(),
	}

	for _, subs := range b.subscriptions[topic] {
		for _, sub := range subs {
			if err := b.pubpool.Submit(func() {
				if err := sub.receiveMessage(msg); err != nil {
					fmt.Printf("error receiving message: %s", err)
				}
			}); err != nil {
				fmt.Printf("error submitting message to pool: %s", err)
			}
		}
	}
}

func (b *Bus) addSubscription(subscription *Subscription) {
	b.subLock.Lock()
	defer b.subLock.Unlock()

	subs, ok := b.subscriptions[subscription.Topic()]
	if !ok {
		subs = map[uint64][]*Subscription{}
		b.subscriptions[subscription.Topic()] = subs
	}

	subs[subscription.ID()] = append(subs[subscription.ID()], subscription)
}

func (b *Bus) removeSubscription(topic string, subscriptionID uint64) {
	b.subLock.Lock()
	defer b.subLock.Unlock()

	subs, ok := b.subscriptions[topic]
	if !ok {
		return
	}

	delete(subs, subscriptionID)
}

func (b *Bus) Subscribe(topic string) (*Subscription, error) {
	subscription := &Subscription{
		id:    b.subID.Add(1),
		topic: topic,
		bus:   b,
	}

	b.addSubscription(subscription)

	return subscription, nil
}
