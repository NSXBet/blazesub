<h1 align="center"><img src="logo.png" width="96" /><br />🔥 BlazeSub</h1>

<p align="center">
  <a href="https://codecov.io/gh/NSXBet/blazesub" >
    <img src="https://codecov.io/gh/NSXBet/blazesub/graph/badge.svg?token=WVMB77T2ZG"/>
  </a>
  <a href="https://pkg.go.dev/github.com/NSXBet/blazesub"><img src="https://pkg.go.dev/badge/github.com/NSXBet/blazesub.svg" alt="Go Reference"></a>
  <a href="https://github.com/NSXBet/blazesub/blob/main/LICENSE"><img src="https://img.shields.io/github/license/NSXBet/blazesub" alt="License"></a>
  <a href="https://github.com/NSXBet/blazesub/blob/main/go.mod"><img src="https://img.shields.io/github/go-mod/go-version/NSXBet/blazesub" alt="Go Version"></a>
</p>

BlazeSub is a high-performance, lock-free publish/subscribe system designed to outperform traditional MQTT brokers. It provides efficient message routing with support for wildcard subscriptions while maintaining thread safety through lock-free data structures.

## ✨ Features

- **⚡ High-throughput delivery**: Up to 12.8 million message deliveries per second to 1000 subscribers
- **🧠 Zero memory allocations**: Core operations don't allocate memory, reducing GC pressure
- **🔒 Thread-safe by design**: Uses lock-free data structures for maximum concurrency
- **🌳 MQTT-compatible topic matching**: Supports single-level (+) and multi-level (#) wildcards
- **🚀 Efficient topic caching**: Optimizes repeat accesses to common topics
- **🔄 Flexible message delivery**: Choose between worker pool or direct goroutines for optimal performance
- **📉 Memory efficiency**: Uses 7–14x less memory per publish than MochiMQTT
- **0️⃣ Zero allocations** for core subscription matching operations
- **🗑️ Minimal GC impact**: Only 2 allocations per publish operation

## 📊 Performance Highlights

All numbers measured on an Apple M4 Pro (Go 1.26.6); the fan-out figures are end-to-end delivery throughput — the clock stops only when every message has reached every subscriber handler. See [PERFORMANCE.md](PERFORMANCE.md) for methodology and how to reproduce.

**Fan-out to 1000 subscribers** (deliveries per second, 3 runs each):

| Scenario                | Direct Goroutines | Worker Pool      |
| ----------------------- | ----------------- | ---------------- |
| Direct match            | 9.9–10.7M/s       | 12.4–12.6M/s     |
| Wildcard match          | 9.8–10.5M/s       | 12.7–12.8M/s     |

**vs MochiMQTT** (5000 subscriptions across 1000 topics, 20% wildcard-matching publishes, in-process routing with no network I/O):

| Benchmark               | BlazeSub (best)   | MochiMQTT        | BlazeSub advantage            |
| ----------------------- | ----------------- | ---------------- | ----------------------------- |
| Single publish          | 1,396 ns/op       | 1,442 ns/op      | ~1.04x faster, 7x less memory |
| Concurrent publish      | 325 ns/op         | 637 ns/op        | ~1.9x faster, 11x less memory |
| Subscribe/unsubscribe   | 5,195 ns/op       | 1,487 ns/op      | 3.5x slower                   |

## ⚖️ Tradeoffs

- **Subscription churn is slow**: subscribing/unsubscribing is ~3.5x slower than MochiMQTT (copy-on-write subscription trie). BlazeSub favors publish fan-out over frequent subscription changes.
- **Worker pool vs MQTT on single publishes**: with a large worker pool, a single publish can be slower than MochiMQTT's in-process routing (3.3μs vs 1.4μs) — use direct goroutines for the fast path.
- **Delivery mode**: direct goroutines are ~2x faster than the worker pool at small subscriber counts; at 1000 subscribers the fixed overhead of goroutine fan-out dominates and the worker pool delivers slightly faster.
- **The old headline was wrong**: previous versions of these docs claimed 84.7M msg/s and "30–50x faster than MQTT"; re-measurement showed those figures came from a timing flaw in the benchmark (a fixed sleep inside the measured window) and network-free comparisons presented as broker speed.

## 📘 Documentation

- [**API Reference**](https://pkg.go.dev/github.com/NSXBet/blazesub) - Complete API documentation on pkg.go.dev
- [**User Guide**](USER_GUIDE.md) - Comprehensive guide for using BlazeSub
- [**Performance Analysis**](PERFORMANCE.md) - Detailed performance metrics and comparisons
- [**MaxConcurrentSubscriptions Guide**](USER_GUIDE_MAX_CONCURRENT_SUBS.md) - Optimizing message delivery to multiple subscribers

## 🔍 Subscription Types

BlazeSub supports both exact and wildcard subscriptions, following the MQTT topic matching rules:

### Exact Match Subscriptions
- Match topics exactly as specified
- Example: `sensors/temperature` only matches messages published to `sensors/temperature`

### Wildcard Subscriptions
BlazeSub supports two types of wildcards:

#### Single-Level Wildcard (+)
- Matches exactly one level in the topic hierarchy
- Can be used multiple times in a topic
- Examples:
  - `sensors/+/temperature` matches:
    - `sensors/room1/temperature`
    - `sensors/room2/temperature`
    - But not `sensors/temperature` or `sensors/room1/floor1/temperature`
  - `+/+/temperature` matches any topic with exactly three levels ending in `temperature`

#### Multi-Level Wildcard (#)
- Matches any number of levels in the topic hierarchy
- Must be the last character in the topic
- Can only be used once
- Examples:
  - `sensors/#` matches:
    - `sensors/temperature`
    - `sensors/humidity`
    - `sensors/room1/temperature`
    - Any topic starting with `sensors/`
  - `#` matches all topics

### Subscription Examples
```go
// Exact match
bus.Subscribe("sensors/temperature")

// Single-level wildcard
bus.Subscribe("sensors/+/temperature")  // Matches any single level
bus.Subscribe("+/+/temperature")        // Matches exactly three levels

// Multi-level wildcard
bus.Subscribe("sensors/#")              // Matches all sensors topics
bus.Subscribe("#")                      // Matches all topics
```

## 📝 Quick Start

### With basic []byte messages

```go
// Create a new bus with defaults ([]byte for the message)
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
subscription.OnMessage(blazesub.MessageHandlerFunc[[]byte](func(msg *blazesub.Message[[]byte]) error {
    fmt.Printf("Received: %s\n", string(msg.Data))

    // Access metadata (if any)
    if msg.Metadata != nil {
        if unit, ok := msg.Metadata["unit"].(string); ok {
            fmt.Printf("Unit: %s\n", unit)
        }
    }

    return nil
}))

// Publish to a topic (with metadata)
metadata := map[string]any{
    "unit": "celsius",
    "device_id": "thermostat-living-room",
}
bus.Publish("sensors/temperature", []byte("25.5"), metadata)

// When done
subscription.Unsubscribe()
```

### With custom message types

```go
// Define your custom message type
type SensorReading struct {
    Value       float64   `json:"value"`
    Unit        string    `json:"unit"`
    DeviceID    string    `json:"device_id"`
    Timestamp   time.Time `json:"timestamp"`
}

// Create a bus with your custom type
bus, err := blazesub.NewBusWithDefaultsOf[SensorReading]()
if err != nil {
    log.Fatal(err)
}
defer bus.Close()

// Subscribe and handle typed messages directly
subscription, err := bus.Subscribe("sensors/temperature")
if err != nil {
    log.Fatal(err)
}

subscription.OnMessage(blazesub.MessageHandlerFunc[SensorReading](func(msg *blazesub.Message[SensorReading]) error {
    // Direct access to structured data!
    reading := msg.Data
    fmt.Printf("Device %s reports %.1f %s at %s\n",
        reading.DeviceID, reading.Value, reading.Unit, reading.Timestamp.Format(time.RFC3339))
    return nil
}))

// Publish structured data directly
bus.Publish("sensors/temperature", SensorReading{
    Value:     22.5,
    Unit:      "celsius",
    DeviceID:  "thermostat-living-room",
    Timestamp: time.Now(),
})
```

For optimal performance, consider using direct goroutines:

```go
config := blazesub.Config{
    UseGoroutinePool: false,  // Use direct goroutines for max performance
    MaxConcurrentSubscriptions: 50,  // Optimal for most workloads
}
bus, err := blazesub.NewBus(config)
```

For more examples demonstrating different configurations and usage patterns, see the [Examples section on pkg.go.dev](https://pkg.go.dev/github.com/NSXBet/blazesub#pkg-examples).

## 🔧 Key Configuration Options

| Option                     | Purpose           | Recommendation                                 |
| -------------------------- | ----------------- | ---------------------------------------------- |
| UseGoroutinePool           | Delivery mode     | `false` for speed, `true` for resource control |
| MaxConcurrentSubscriptions | Delivery batching | Keep below your subscriber count               |

See the [User Guide](USER_GUIDE.md) for detailed configuration information.

## 🚀 Releasing New Versions

BlazeSub uses [GoReleaser](https://goreleaser.com/) for streamlined releases. To create a new release:

1. **Update Version**: Update documentation and code with the new version
2. **Create Tag**:
   ```bash
   git tag -a v0.x.y -m "Release v0.x.y"
   git push origin v0.x.y
   ```
3. **Automatic Release**: GitHub Actions will automatically:
   - Build and validate the release
   - Generate documentation archive
   - Create GitHub release with release notes
   - Update pkg.go.dev

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
