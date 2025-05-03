# BlazeSub

BlazeSub is a high-performance, lock-free publish/subscribe system designed to outperform traditional MQTT brokers. It provides efficient message routing with support for wildcard subscriptions while maintaining thread safety through lock-free data structures.

## Features

- **Ultra-fast performance**: Up to 170 million operations per second for subscription matching
- **Zero memory allocations**: Core operations don't allocate memory, reducing GC pressure
- **Thread-safe by design**: Uses lock-free data structures from the xsync library
- **MQTT-compatible topic matching**: Supports single-level (+) and multi-level (#) wildcards
- **Efficient topic caching**: Optimizes repeat accesses to common topics
- **Flexible message delivery**: Choose between worker pool or direct goroutines for optimal performance
- **Low-latency message delivery**: Up to 56% faster with direct goroutines mode

## Performance

BlazeSub significantly outperforms traditional MQTT brokers:

- **Exact match lookups**: 1,700-3,400x faster than MQTT
- **Wildcard match lookups**: 5,200-15,600x faster than MQTT
- **Zero allocations** for subscription matching operations
- **Direct goroutines mode**: 45-56% faster than worker pool mode for most use cases

See the [detailed benchmark report](BENCHMARK.md) for comprehensive performance metrics and [performance comparison](PERFORMANCE.md) for worker pool vs direct goroutines analysis.

## Usage

```go
// Create a new bus with defaults (uses worker pool)
bus, err := blazesub.NewBusWithDefaults()
if err != nil {
    log.Fatal(err)
}
defer bus.Close()

// Or create with custom configuration
config := blazesub.Config{
    // Set to false for maximum performance with direct goroutines
    UseGoroutinePool: false,
    // Other configuration options...
}
fastBus, err := blazesub.NewBus(config)
if err != nil {
    log.Fatal(err)
}
defer fastBus.Close()

// Subscribe to a topic
subscription, err := bus.Subscribe("sensors/temperature")
if err != nil {
    log.Fatal(err)
}

// Handle messages
subscription.OnMessage(func(msg *blazesub.Message) error {
    fmt.Printf("Received message on %s: %s\n", msg.Topic, string(msg.Data))
    return nil
})

// Publish to a topic
bus.Publish("sensors/temperature", []byte("25.5"))

// Subscribe with wildcards
wildcard, err := bus.Subscribe("sensors/+/status")
if err != nil {
    log.Fatal(err)
}

// Unsubscribe when done
subscription.Unsubscribe()
```

## Implementation Details

BlazeSub uses a hybrid approach combining:

1. **Trie-based wildcard matching**: Efficiently handles wildcard patterns
2. **Lock-free hash maps**: Fast exact match lookups with thread safety
3. **Flexible message delivery**: Choose between worker pool or direct goroutines
4. **Result caching**: Optimizes repeated topic lookups

The system uses nested xsync maps to provide thread safety without mutex locks, leading to dramatic performance improvements in high-concurrency scenarios.

### Delivery Modes

BlazeSub offers two modes for message delivery:

- **Worker Pool Mode**: Uses the `ants` library to manage a pool of reusable goroutines, which helps prevent goroutine explosion under extreme load.
- **Direct Goroutines Mode**: Creates new goroutines for each message delivery, providing maximum performance for typical use cases with fast message handlers.

See [PERFORMANCE.md](PERFORMANCE.md) for detailed analysis and recommendations.

## License

[Insert your license information here]
