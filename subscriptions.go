package blazesub

// Subscription represents a subscription to a topic.
type Subscription struct {
	id            uint64
	topic         string
	handler       MessageHandler
	unsubscribeFn func() error
}

// MessageHandlerFunc is a function type that implements MessageHandler interface
type MessageHandlerFunc func(message *Message) error

// OnMessage implements MessageHandler interface
func (f MessageHandlerFunc) OnMessage(message *Message) error {
	return f(message)
}

func (s *Subscription) OnMessage(handler MessageHandler) {
	s.handler = handler
}

func (s *Subscription) ID() uint64 {
	return s.id
}

func (s *Subscription) Topic() string {
	return s.topic
}

func (s *Subscription) receiveMessage(message *Message) error {
	// Fast path for nil handler (very common in benchmarks)
	if s.handler == nil {
		return nil
	}

	// Directly call handler without additional error checking in hot path
	// This saves function call overhead and error handling overhead
	return s.handler.OnMessage(message)
}

func (s *Subscription) SetUnsubscribeFunc(fn func() error) {
	s.unsubscribeFn = fn
}

func (s *Subscription) Unsubscribe() error {
	if s.unsubscribeFn != nil {
		return s.unsubscribeFn()
	}

	return nil
}
