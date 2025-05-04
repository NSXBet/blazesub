# 🔥 BlazeSub

BlazeSub is a high-performance, lock-free publish/subscribe system designed to outperform traditional MQTT brokers. It provides efficient message routing with support for wildcard subscriptions while maintaining thread safety through lock-free data structures.

## ✨ Features

- **⚡ Ultra-fast performance**: Up to [84.7 million messages per second delivered to 1000 subscribers](PERFORMANCE.md)
- **🧠 Zero memory allocations**: Core operations don't allocate memory, reducing GC pressure
- **🔒 Thread-safe by design**: Uses lock-free data structures for maximum concurrency
- **🌳 MQTT-compatible topic matching**: Supports single-level (+) and multi-level (#) wildcards
- **🚀 Efficient topic caching**: Optimizes repeat accesses to common topics
- **🔄 Flexible message delivery**: Choose between worker pool or direct goroutines for optimal performance
- **⏱️ Low-latency message delivery**: Direct goroutines up to 52% faster than worker pool and 34% faster than MQTT

## 📊 Performance Highlights

- **💯 Direct match throughput**: 84.7 million messages per second to 1000 subscribers
- **🔍 Wildcard match throughput**: 83.5 million messages per second to 1000 subscribers
- **📉 Memory efficiency**: Uses up to 95% less memory than MochiMQTT
- **0️⃣ Zero allocations** for core subscription matching operations
- **🗑️ Minimal GC impact**: Only 2 allocations per publish operation

## 📘 Documentation

- [**User Guide**](USER_GUIDE.md) - Comprehensive guide for using BlazeSub
- [**Performance Analysis**](PERFORMANCE.md) - Detailed performance metrics and comparisons
- [**MaxConcurrentSubscriptions Guide**](max_concurrent_subscriptions.md) - Optimizing message delivery to multiple subscribers

## 📝 Quick Start

```go
// Create a new bus with defaults
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
    fmt.Printf("Received: %s\n", string(msg.Data))
    return nil
})

// Publish to a topic
bus.Publish("sensors/temperature", []byte("25.5"))

// When done
subscription.Unsubscribe()
```

For optimal performance, consider using direct goroutines:

```go
config := blazesub.Config{
    UseGoroutinePool: false,  // Use direct goroutines for max performance
    MaxConcurrentSubscriptions: 50,  // Optimal for most workloads
}
bus, err := blazesub.NewBus(config)
```

## 🔧 Key Configuration Options

| Option                     | Purpose           | Recommendation                                 |
| -------------------------- | ----------------- | ---------------------------------------------- |
| UseGoroutinePool           | Delivery mode     | `false` for speed, `true` for resource control |
| MaxConcurrentSubscriptions | Delivery batching | Keep below your subscriber count               |

See the [User Guide](USER_GUIDE.md) for detailed configuration information.

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
