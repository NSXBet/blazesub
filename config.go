package blazesub

import (
	"time"

	"github.com/go-logr/logr"
)

type Config struct {
	WorkerCount                int
	PreAlloc                   bool
	MaxBlockingTasks           int
	MaxConcurrentSubscriptions int
	ExpiryDuration             time.Duration
	Logger                     logr.Logger
}

const (
	DefaultWorkerCount                = 10_000
	DefaultMaxBlockingTasks           = 10_000
	DefaultMaxConcurrentSubscriptions = 10
)

func NewConfig() Config {
	return Config{
		WorkerCount:                DefaultWorkerCount,
		PreAlloc:                   true,
		MaxBlockingTasks:           DefaultMaxBlockingTasks,
		ExpiryDuration:             0,
		MaxConcurrentSubscriptions: DefaultMaxConcurrentSubscriptions,
	}
}
