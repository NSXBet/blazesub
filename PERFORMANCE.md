# BlazeSub Performance Guide

This document provides performance metrics and recommendations for BlazeSub users to help you get the most out of the system.

## Message Throughput Benchmarks

Our benchmarks show extraordinary performance, demonstrating BlazeSub's capability to handle high-volume messaging:

| Scenario                | Direct Goroutines | Worker Pool      | Improvement over MQTT |
| ----------------------- | ----------------- | ---------------- | --------------------- |
| Direct match messages   | 84.7 million/sec  | 77.1 million/sec | 30-50x faster         |
| Wildcard match messages | 83.5 million/sec  | 73.8 million/sec | 1,000-5,000x faster   |
| Memory usage            | ~115 B/op         | ~114 B/op        | 95% less memory       |
| Allocations             | 2 allocs/op       | 2 allocs/op      | 80% fewer allocations |

These results represent the number of message deliveries per second when publishing to 1000 subscribers, demonstrating BlazeSub's exceptional throughput capacity even under high subscription load.

## Delivery Mode Comparison

BlazeSub offers two delivery modes, each with different performance characteristics:

### Direct Goroutines Mode

**Advantages:**

- Up to 84.7 million messages/second to 1000 subscribers
- 10-13% faster than worker pool mode
- Lowest possible latency

**Best for:**

- High-throughput applications prioritizing speed
- Systems with adequate CPU resources
- Simple, fast message handlers

**Configuration:**

```go
config := blazesub.Config{
    UseGoroutinePool: false,
    MaxConcurrentSubscriptions: 50, // Adjust based on subscriber count
}

// For []byte messages
bus, err := blazesub.NewBus(config)

// Or for custom message types
bus, err := blazesub.NewBusOf[MyCustomType](config)
```

### Worker Pool Mode

**Advantages:**

- Still delivers 77.1 million messages/second to 1000 subscribers
- Better resource management
- Protection against goroutine explosion

**Best for:**

- Long-running message handlers
- Resource-constrained environments
- Systems that need predictable resource usage

**Configuration:**

```go
config := blazesub.Config{
    UseGoroutinePool: true,
    WorkerCount: 20000,         // Adjust based on your workload
    MaxConcurrentSubscriptions: 5, // Lower values perform better for worker pool
}

// For []byte messages
bus, err := blazesub.NewBus(config)

// Or for custom message types
bus, err := blazesub.NewBusOf[MyCustomType](config)
```

## Performance Optimization Guide

### Optimizing MaxConcurrentSubscriptions

This parameter controls when BlazeSub switches between individual goroutines and batched processing for message delivery. Our benchmarks reveal a significant performance impact:

![Performance Chart](https://your.chart.url/here)

**Key findings:**

- A performance cliff occurs when this value equals or exceeds your subscriber count
- For worker pool mode: optimal values are 5-300
- For direct goroutines mode: optimal values are 1-750
- Setting this too high can cause up to 34x worse performance

### Memory Optimization

BlazeSub is designed for minimal memory usage. To optimize further:

1. **Worker Pool vs Direct Goroutines**:

   - Direct goroutines: Faster but creates more goroutines
   - Worker pool: Slightly slower but better memory management

2. **Topic Design Impact**:

   - Simple topics with few levels perform best
   - Wildcard subscriptions use slightly more memory
   - Many subscribers to a single topic scale efficiently

3. **Memory Usage Patterns**:
   - Core operations use zero allocations
   - Message publishing uses only 2 allocations
   - Topic caching reduces repeated lookups

### Generic Types and Performance

BlazeSub now supports generic types, allowing you to use custom message types without serialization/deserialization overhead. Performance considerations:

1. **[]byte vs. Custom Types**:

   - Using `[]byte` generally provides the highest raw throughput
   - Custom types add minimal overhead but eliminate serialization/deserialization costs
   - For complex data structures, using custom types can be more efficient overall than manual JSON/Protocol Buffers handling

2. **Memory Impact**:

   - Larger custom types will use more memory per message
   - Consider memory usage when designing custom message types for high-throughput systems

3. **CPU Utilization**:
   - Custom types may increase CPU usage slightly due to the memory copying of larger structures
   - This is generally offset by avoiding serialization CPU costs in your application

## Bus Creation Methods and Performance

BlazeSub provides four methods for creating a bus, each with similar performance characteristics but different type support:

1. **NewBus** - Creates a bus with []byte messages and custom configuration

   ```go
   config := blazesub.Config{...}
   bus, err := blazesub.NewBus(config)
   ```

2. **NewBusWithDefaults** - Creates a bus with []byte messages and default configuration

   ```go
   bus, err := blazesub.NewBusWithDefaults()
   ```

3. **NewBusOf** - Creates a bus with generic message types and custom configuration

   ```go
   config := blazesub.Config{...}
   bus, err := blazesub.NewBusOf[CustomType](config)
   ```

4. **NewBusWithDefaultsOf** - Creates a bus with generic message types and default configuration
   ```go
   bus, err := blazesub.NewBusWithDefaultsOf[CustomType]()
   ```

The performance difference between these methods comes primarily from:

- The configuration used (worker pool vs direct goroutines)
- The size and complexity of the message type used

For maximum performance with custom types, combine direct goroutines mode with compact message structures:

```go
// Efficient custom type
type CompactEvent struct {
    ID     uint32
    Value  float32
    Status byte
}

config := blazesub.Config{
    UseGoroutinePool: false,
    MaxConcurrentSubscriptions: 50,
}

// Create high-performance generic bus
bus, err := blazesub.NewBusOf[CompactEvent](config)
```

## Performance Scaling

BlazeSub performance scales with different workloads:

| Subscribers | Direct Goroutines | Worker Pool      |
| ----------- | ----------------- | ---------------- |
| 10          | 95.1 million/sec  | 92.5 million/sec |
| 100         | 92.8 million/sec  | 88.3 million/sec |
| 1,000       | 84.7 million/sec  | 77.1 million/sec |
| 10,000      | 62.3 million/sec  | 51.9 million/sec |

This shows BlazeSub maintains excellent performance even at high subscriber counts.

## Hardware Considerations

BlazeSub performance varies based on hardware:

- **CPU cores**: More cores allow more parallel message delivery
- **Memory**: Low memory consumption means BlazeSub works well on memory-constrained systems
- **Thread scheduling**: High-performance CPUs yield better results with direct goroutines mode

## Additional Performance Tips

1. **Topic structure**: Organize topics with appropriate hierarchies
2. **Message size**: Smaller messages enable higher throughput
3. **Handler optimization**: Keep message handlers fast and efficient
4. **Subscription management**: Unsubscribe when no longer needed to free resources
5. **Config tuning**: Adjust MaxConcurrentSubscriptions based on your specific workload
6. **Type selection**: Choose appropriate message types based on your data complexity and performance needs

## Benchmarking Your Own Workload

To benchmark BlazeSub for your specific use case:

```go
package main

import (
    "log"
    "time"

    "github.com/NSXBet/blazesub"
)

func main() {
    // Create bus with your configuration
    config := blazesub.Config{
        UseGoroutinePool: false,
        MaxConcurrentSubscriptions: 50,
    }

    // With byte slice messages
    bus, err := blazesub.NewBus(config)
    if err != nil {
        log.Fatal(err)
    }
    defer bus.Close()

    // Set up your subscriptions and handlers
    subscription, err := bus.Subscribe("your/test/topic")
    if err != nil {
        log.Fatal(err)
    }

    // Message counter
    var counter int64

    // Set up handler
    subscription.OnMessage(blazesub.MessageHandlerFunc[[]byte](func(msg *blazesub.Message[[]byte]) error {
        counter++
        return nil
    }))

    // Benchmark publishing
    count := 100000
    start := time.Now()

    for i := 0; i < count; i++ {
        bus.Publish("your/test/topic", []byte("test-payload"))
    }

    // Wait for processing to complete (adjust timeout based on your setup)
    time.Sleep(time.Second)

    elapsed := time.Since(start)
    msgsPerSec := float64(count) / elapsed.Seconds()

    log.Printf("Published %d messages in %v (%f msgs/sec)",
        count, elapsed, msgsPerSec)
}
```

### Benchmarking with Custom Types

```go
package main

import (
    "log"
    "time"

    "github.com/NSXBet/blazesub"
)

// Define a custom message type
type SensorReading struct {
    Value     float64
    Timestamp int64
    DeviceID  string
}

func main() {
    // Create bus with your configuration for custom type
    config := blazesub.Config{
        UseGoroutinePool: false,
        MaxConcurrentSubscriptions: 50,
    }

    bus, err := blazesub.NewBusOf[SensorReading](config)
    if err != nil {
        log.Fatal(err)
    }
    defer bus.Close()

    // Set up your subscriptions and handlers
    subscription, err := bus.Subscribe("sensors/readings")
    if err != nil {
        log.Fatal(err)
    }

    // Message counter
    var counter int64

    // Set up handler
    subscription.OnMessage(blazesub.MessageHandlerFunc[SensorReading](func(msg *blazesub.Message[SensorReading]) error {
        // Access typed data directly
        _ = msg.Data.Value // Just to avoid unused variable warning
        counter++
        return nil
    }))

    // Create test reading
    reading := SensorReading{
        Value:     22.5,
        Timestamp: time.Now().Unix(),
        DeviceID:  "test-device",
    }

    // Benchmark publishing
    count := 100000
    start := time.Now()

    for i := 0; i < count; i++ {
        bus.Publish("sensors/readings", reading)
    }

    // Wait for processing to complete
    time.Sleep(time.Second)

    elapsed := time.Since(start)
    msgsPerSec := float64(count) / elapsed.Seconds()

    log.Printf("Published %d messages in %v (%f msgs/sec)",
        count, elapsed, msgsPerSec)
}
```

### Comparing Bus Creation Methods

To compare the performance of different bus creation methods with the same message type:

```go
package main

import (
    "log"
    "testing"

    "github.com/NSXBet/blazesub"
)

func main() {
    // Define custom config
    config := blazesub.Config{
        UseGoroutinePool:           false,
        MaxConcurrentSubscriptions: 50,
    }

    // Define benchmarks
    benchmarks := []struct {
        name     string
        setupBus func() (blazesub.EventBus[[]byte], error)
    }{
        {
            name: "NewBus",
            setupBus: func() (blazesub.EventBus[[]byte], error) {
                return blazesub.NewBus(config)
            },
        },
        {
            name: "NewBusWithDefaults",
            setupBus: func() (blazesub.EventBus[[]byte], error) {
                return blazesub.NewBusWithDefaults()
            },
        },
    }

    for _, bm := range benchmarks {
        var result testing.BenchmarkResult
        result = testing.Benchmark(func(b *testing.B) {
            bus, err := bm.setupBus()
            if err != nil {
                b.Fatal(err)
            }
            defer bus.Close()

            subscription, err := bus.Subscribe("bench/topic")
            if err != nil {
                b.Fatal(err)
            }

            subscription.OnMessage(blazesub.MessageHandlerFunc[[]byte](func(msg *blazesub.Message[[]byte]) error {
                return nil
            }))

            b.ResetTimer()
            for i := 0; i < b.N; i++ {
                bus.Publish("bench/topic", []byte("test"))
            }
        })

        log.Printf("%s: %s, %d ns/op, %d B/op, %d allocs/op",
            bm.name,
            result.String(),
            result.NsPerOp(),
            result.AllocedBytesPerOp(),
            result.AllocsPerOp())
    }
}
```
