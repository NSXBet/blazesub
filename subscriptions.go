package blazesub

// Subscription represents a subscription to a topic with generic message type.
type Subscription[T any] struct {
	id            uint64
	topic         string
	handler       MessageHandler[T]
	unsubscribeFn func(string, uint64) error
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
	// Fast path for nil handler (very common in benchmarks)
	if s.handler == nil {
		return nil
	}

	// Directly call handler without additional error checking in hot path
	// This saves function call overhead and error handling overhead
	return s.handler.OnMessage(message)
}

// SetUnsubscribeFunc sets the function to call when unsubscribing.
// The function takes the topic and subscription ID as parameters.
func (s *Subscription[T]) SetUnsubscribeFunc(fn func(string, uint64) error) {
	s.unsubscribeFn = fn
}

// Unsubscribe unsubscribes from the topic.
func (s *Subscription[T]) Unsubscribe() error {
	if s.unsubscribeFn != nil {
		return s.unsubscribeFn(s.topic, s.id)
	}

	return nil
}
