package blazesub

// MessageHandler is an interface for handling messages of a specific type.
type MessageHandler[T any] interface {
	OnMessage(message *Message[T]) error
}

type MessageHandlerFunc[T any] func(message *Message[T]) error

func (f MessageHandlerFunc[T]) OnMessage(message *Message[T]) error {
	return f(message)
}
