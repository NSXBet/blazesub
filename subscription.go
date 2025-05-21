package blazesub

import "go.uber.org/atomic"

// Subscription represents a subscription to a topic with generic message type.
type Subscription[T any] struct {
	id            uint64
	topic         string
	handler       MessageHandler[T]
	unsubscribeFn func(topic string, subID uint64) error
	status        *atomic.Uint32
}

// Reset clears the Subscription fields for reuse.
func (s *Subscription[T]) Reset() {
	s.id = 0 // Or some other indicator of a reset/pooled state if needed
	s.topic = ""
	s.handler = nil
	s.unsubscribeFn = nil
	s.status.Store(0) // Reset status to 0 (active/ready for reuse)
}

func NewSubscription[T any](id uint64, topic string) *Subscription[T] {
	return &Subscription[T]{
		id:     id,
		topic:  topic,
		status: atomic.NewUint32(0),
	}
}

// OnMessage sets the message handler for this subscription.
func (s *Subscription[T]) OnMessage(handler MessageHandler[T]) {
	s.handler = handler
}

// ID returns the subscription ID.
func (s *Subscription[T]) ID() uint64 {
	return s.id
}

// Topic returns the subscription topic pattern.
func (s *Subscription[T]) Topic() string {
	return s.topic
}

// receiveMessage handles incoming messages for this subscription.
func (s *Subscription[T]) receiveMessage(message *Message[T]) error {
	// Fast path for nil handler (very common in benchmarks) or closed subscription
	if s.handler == nil || s.IsClosed() {
		return nil
	}

	// Directly call handler without additional error checking in hot path
	// This saves function call overhead and error handling overhead
	return s.handler.OnMessage(message)
}

// SetUnsubscribeFunc sets the function to call when unsubscribing.
func (s *Subscription[T]) SetUnsubscribeFunc(fn func(topic string, subID uint64) error) {
	s.unsubscribeFn = fn
}

// Unsubscribe unsubscribes from the topic.
func (s *Subscription[T]) Unsubscribe() error {
	isOpen := s.status.CompareAndSwap(0, 1)
	if !isOpen {
		return nil
	}

	if s.unsubscribeFn != nil {
		return s.unsubscribeFn(s.topic, s.id)
	}

	return nil
}

func (s *Subscription[T]) IsClosed() bool {
	return s.status.Load() >= 1
}
