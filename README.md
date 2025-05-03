# BlazeSub

BlazeSub is a high-performance, lock-free publish/subscribe system designed to outperform traditional MQTT brokers. It provides efficient message routing with support for wildcard subscriptions while maintaining thread safety through lock-free data structures.

## Features

- **Ultra-fast performance**: Up to 170 million operations per second for subscription matching
- **Zero memory allocations**: Core operations don't allocate memory, reducing GC pressure
- **Thread-safe by design**: Uses lock-free data structures from the xsync library
- **MQTT-compatible topic matching**: Supports single-level (+) and multi-level (#) wildcards
- **Efficient topic caching**: Optimizes repeat accesses to common topics
- **Low-latency message delivery**: Worker pool for parallel message processing

## Performance

BlazeSub significantly outperforms traditional MQTT brokers:

- **Exact match lookups**: 1,700-3,400x faster than MQTT
- **Wildcard match lookups**: 5,200-15,600x faster than MQTT
- **Zero allocations** for subscription matching operations

See the [detailed benchmark report](BENCHMARK.md) for comprehensive performance metrics.

## Usage

```go
// Create a new bus
bus, err := blazesub.NewBusWithDefaults()
if err != nil {
    log.Fatal(err)
}
defer bus.Close()

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
3. **Worker pool**: Parallelizes message delivery to subscribers
4. **Result caching**: Optimizes repeated topic lookups

The system uses nested xsync maps to provide thread safety without mutex locks, leading to dramatic performance improvements in high-concurrency scenarios.

## License

[Insert your license information here]
