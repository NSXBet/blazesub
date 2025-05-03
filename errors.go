package blazesub

import "errors"

var (
	ErrInvalidFilter     = errors.New("invalid filter")
	ErrInvalidTopic      = errors.New("invalid topic")
	ErrTimeoutClosingBus = errors.New("timed out closing bus")
)
