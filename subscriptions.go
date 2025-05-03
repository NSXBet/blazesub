package blazesub

import "fmt"

type Subscription struct {
	id      uint64
	topic   string
	handler MessageHandler
	bus     *Bus
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
	if s.handler == nil {
		return nil
	}

	if err := s.handler.OnMessage(message); err != nil {
		return fmt.Errorf("receiving published message: %w", err)
	}

	return nil
}

func (s *Subscription) Unsubscribe() error {
	s.bus.removeSubscription(s.topic, s.id)

	return nil
}
