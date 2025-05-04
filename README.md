# 🔥 BlazeSub

BlazeSub is a high-performance, lock-free publish/subscribe system designed to outperform traditional MQTT brokers. It provides efficient message routing with support for wildcard subscriptions while maintaining thread safety through lock-free data structures.

## ✨ Features

- **⚡ Ultra-fast performance**: Up to [84.7 million messages per second delivered to 1000 subscribers](PERFORMANCE.md)
- **🧠 Zero memory allocations**: Core operations don't allocate memory, reducing GC pressure
- **🔒 Thread-safe by design**: Uses lock-free data structures from the xsync library
- **🌳 MQTT-compatible topic matching**: Supports single-level (+) and multi-level (#) wildcards
- **🚀 Efficient topic caching**: Optimizes repeat accesses to common topics
- **🔄 Flexible message delivery**: Choose between worker pool or direct goroutines for optimal performance
- **⏱️ Low-latency message delivery**: Direct goroutines up to 52% faster than worker pool and 34% faster than MQTT

## 📊 Performance

BlazeSub significantly outperforms traditional publish/subscribe systems:

- **💯 Direct match throughput**: 84.7 million messages per second to 1000 subscribers
- **🔍 Wildcard match throughput**: 83.5 million messages per second to 1000 subscribers
- **📉 Memory efficiency**: Uses up to 95% less memory than MochiMQTT
- **0️⃣ Zero allocations** for core subscription matching operations
- **🗑️ Minimal GC impact**: Only 2 allocations per publish operation

See the [detailed benchmark report](BENCHMARK.md) for comprehensive performance metrics, [performance comparison](PERFORMANCE.md) for worker pool vs direct goroutines analysis, and [MaxConcurrentSubscriptions guide](max_concurrent_subscriptions.md) for optimizing message delivery to multiple subscribers.

## 📝 Usage

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
    // Tune this for optimal performance with many subscribers per topic
    MaxConcurrentSubscriptions: 50,
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

## 🔧 Implementation Details

BlazeSub uses a hybrid approach combining:

1. **🌲 Trie-based wildcard matching**: Efficiently handles wildcard patterns
2. **🗝️ Lock-free hash maps**: Fast exact match lookups with thread safety
3. **🧵 Flexible message delivery**: Choose between worker pool or direct goroutines
4. **💾 Result caching**: Optimizes repeated topic lookups

The system uses nested xsync maps to provide thread safety without mutex locks, leading to dramatic performance improvements in high-concurrency scenarios.

### 🚚 Delivery Modes

BlazeSub offers two modes for message delivery:

- **👷 Worker Pool Mode**: Uses the `ants` library to manage a pool of reusable goroutines, which helps prevent goroutine explosion under extreme load.
- **🏎️ Direct Goroutines Mode**: Creates new goroutines for each message delivery, providing maximum performance for typical use cases with fast message handlers.

See [PERFORMANCE.md](PERFORMANCE.md) for detailed analysis and recommendations on delivery modes and [max_concurrent_subscriptions.md](max_concurrent_subscriptions.md) for optimizing message delivery with many subscribers.

## 📄 License

MIT License

Copyright (c) 2023 BlazeSub Contributors

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
