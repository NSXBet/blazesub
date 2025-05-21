package blazesub

import "time"

// Message represents a topic message with generic data.
type Message[T any] struct {
	Topic        string
	Data         T
	Metadata     map[string]any
	UTCTimestamp time.Time
}

// ByteMessage is a type alias for Message[[]byte].
// It's a convenience type for working with byte slices as it is the most common use case.
type ByteMessage = Message[[]byte]

// NewMessage creates a new Message with the given topic and data.
func NewMessage[T any](topic string, data T, metadata ...map[string]any) *Message[T] {
	var metadataMap map[string]any

	if len(metadata) > 0 {
		metadataMap = metadata[0]
	}

	return &Message[T]{
		Topic:        topic,
		Data:         data,
		Metadata:     metadataMap,
		UTCTimestamp: time.Now().UTC(),
	}
}

// Reset clears the message fields for reuse, preparing it to be put back into a sync.Pool.
func (m *Message[T]) Reset() {
	var zeroVal T // Ensures proper zeroing for generic type T
	m.Topic = ""
	m.Data = zeroVal
	m.Metadata = nil
	m.UTCTimestamp = time.Time{}
}
