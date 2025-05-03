package blazesub

import "time"

type Config struct {
	WorkerCount      int
	PreAlloc         bool
	MaxBlockingTasks int
	ExpiryDuration   time.Duration
}

const (
	DefaultWorkerCount      = 10_000
	DefaultMaxBlockingTasks = 10_000
)

func NewConfig() Config {
	return Config{
		WorkerCount:      DefaultWorkerCount,
		PreAlloc:         true,
		MaxBlockingTasks: DefaultMaxBlockingTasks,
		ExpiryDuration:   0,
	}
}
