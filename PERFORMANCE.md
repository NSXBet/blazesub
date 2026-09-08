# BlazeSub Performance Guide

This document provides performance metrics and recommendations for BlazeSub users to help you get the most out of the system.

## Message Throughput Benchmarks

All figures below are end-to-end delivery throughput: the clock stops only when every published message has reached every subscriber handler.

**Fan-out to 1000 subscribers** (deliveries per second, Apple M4 Pro, Go 1.26.6, 3 runs each):

| Scenario                | Direct Goroutines | Worker Pool      |
| ----------------------- | ----------------- | ---------------- |
| Direct match            | 9.9–10.7M/s       | 12.4–12.6M/s     |
| Wildcard match          | 9.8–10.5M/s       | 12.7–12.8M/s     |

**vs MochiMQTT** (5000 subscriptions across 1000 topics, 20% wildcard-matching publishes; in-process routing with no network I/O — this measures routing speed, not end-to-end broker throughput):

| Benchmark               | BlazeSub (best)   | MochiMQTT        | BlazeSub advantage            |
| ----------------------- | ----------------- | ---------------- | ----------------------------- |
| Single publish          | 1,396 ns/op       | 1,442 ns/op      | ~1.04x faster, 7x less memory |
| Concurrent publish      | 325 ns/op         | 637 ns/op        | ~1.9x faster, 11x less memory |
| Subscribe/unsubscribe   | 5,195 ns/op       | 1,487 ns/op      | 3.5x slower                   |

Reproduce with:

```bash
go test -run='^$' -bench 'BenchmarkThroughputWith1000Subscribers|BenchmarkBusVsMQTT$|BenchmarkBusVsMQTTConcurrent|BenchmarkBusVsMQTTSubscribeUnsubscribe' -benchtime=3s -benchmem -count=3 .
```

## Delivery Mode Comparison

BlazeSub offers two delivery modes, each with different performance characteristics:

### Direct Goroutines Mode

**Advantages:**

- Up to 10.7 million message deliveries/second to 1000 subscribers
- ~2x faster than the worker pool at small subscriber counts; at 1000 subscribers the worker pool delivers slightly faster

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

- Up to 12.8 million message deliveries/second to 1000 subscribers
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

   - Direct goroutines: ~2x faster at small subscriber counts, slightly slower at 1000 subscribers
   - Worker pool: the fast fan-out path at 1000 subscribers and predictable resource usage

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

We re-measured scaling up to 1000 subscribers; larger counts are not claimed because we have not benchmarked them.

| Subscribers | Direct Goroutines | Worker Pool      |
| ----------- | ----------------- | ---------------- |
| 1,000       | 9.9–10.7M/s       | 12.4–12.6M/s     |

Note the crossover: at small subscriber counts direct goroutines are roughly twice as fast as the worker pool, but by 1000 subscribers the fixed cost of fanning out goroutines dominates and the worker pool delivers slightly faster.

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

## Subscription Performance Optimizations

Recent optimizations to the subscription system have significantly improved performance and memory efficiency:

| Operation                  | Before Optimizations | After Optimizations | Improvement       |
| -------------------------- | -------------------- | ------------------- | ----------------- |
| Subscribe/Unsubscribe Time | ~4,904 ns/op         | ~1,762 ns/op        | ~64% faster       |
| Memory Usage               | ~21,630 B/op         | ~5,104 B/op         | ~76% less memory  |
| Allocations                | 52 allocs/op         | 11 allocs/op        | ~79% fewer allocs |

These optimizations include:

1. **Object Pooling**: Extensive use of sync.Pool for trie nodes, subscriptions, and segment slices
2. **Improved Memory Management**: Reduced allocations through better buffer reuse
3. **Optimized Topic Splitting**: Cache for frequently accessed topics and pooled buffers
4. **Subscription Lifecycle Tracking**: Better tracking of closed subscriptions
5. **Lock-free Patterns**: All operations remain lock-free while being more efficient

### Memory Efficiency

The optimized subscription system now consumes significantly less memory thanks to:

- Reusing node and subscription objects instead of allocating new ones
- Better string handling and reduced unnecessary allocations
- Efficient segment tracking and caching
- Smart buffer management for temporary storage

### Wildcard Matching Performance

Wildcard matching performance has been preserved while improving memory efficiency:

- Exact topic matches: ~213 ns/op with only 143 B/op and 4 allocs/op
- Consistent matching speed regardless of subscription complexity
- Efficient memory usage during wildcard topic matching

### Bulk Subscription Management

For applications managing many subscriptions, the optimizations provide:

- Faster bulk unsubscribe operations: ~950 ns per subscription
- Reduced GC pressure during subscription churn
- Better memory locality through optimized data structures
- 35% performance improvement in direct goroutines mode vs worker pool

### Implementation Differences

Our optimizations revealed performance differences between modes:

| Implementation Mode | Subscribe/Unsubscribe Speed | Memory Usage | Best Use Case                                                  |
| ------------------- | --------------------------- | ------------ | -------------------------------------------------------------- |
| Direct Goroutines   | ~3,689 ns/op                | ~9,206 B/op  | High-throughput systems with moderate subscription changes     |
| Worker Pool         | ~5,735 ns/op                | ~9,208 B/op  | Systems with resource constraints needing predictable behavior |
| Single Operation    | ~1,762 ns/op                | ~5,104 B/op  | Individual subscribe/unsubscribe operations                    |
