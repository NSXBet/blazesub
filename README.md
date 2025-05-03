# BlazeSub

BlazeSub is a high-performance, lock-free publish/subscribe system designed to outperform traditional MQTT brokers. It provides efficient message routing with support for wildcard subscriptions while maintaining thread safety through lock-free data structures.

## Features

- **Ultra-fast performance**: Up to 170 million operations per second for subscription matching
- **Zero memory allocations**: Core operations don't allocate memory, reducing GC pressure
- **Thread-safe by design**: Uses lock-free data structures from the xsync library
- **MQTT-compatible topic matching**: Supports single-level (+) and multi-level (#) wildcards
- **Efficient topic caching**: Optimizes repeat accesses to common topics
- **Flexible message delivery**: Choose between worker pool or direct goroutines for optimal performance
- **Low-latency message delivery**: Direct goroutines up to 52% faster than worker pool and 34% faster than MQTT

## Performance

BlazeSub significantly outperforms traditional publish/subscribe systems:

- **Direct goroutines mode**: 34% faster than MochiMQTT for message publishing
- **Concurrent performance**: 31.7% faster than MochiMQTT under high concurrent load
- **Memory efficiency**: Uses up to 95% less memory than MochiMQTT
- **Zero allocations** for core subscription matching operations
- **Minimal GC impact**: Fewer allocations mean less garbage collection overhead

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
