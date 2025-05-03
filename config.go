package blazesub

import "time"

type Config struct {
	WorkerCount      int
	PreAlloc         bool
	MaxBlockingTasks int
	ExpiryDuration   time.Duration
	RegistryPoolSize int // Number of registries in the pool
}

const (
	DefaultWorkerCount      = 10_000
	DefaultMaxBlockingTasks = 10_000
	DefaultRegistryPoolSize = 4 // Default number of registries
)

func NewConfig() Config {
	return Config{
		WorkerCount:      DefaultWorkerCount,
		PreAlloc:         true,
		MaxBlockingTasks: DefaultMaxBlockingTasks,
		ExpiryDuration:   0,
		RegistryPoolSize: DefaultRegistryPoolSize,
	}
}
