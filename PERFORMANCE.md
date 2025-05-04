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
    bus, err := blazesub.NewBus(config)
    if err != nil {
        log.Fatal(err)
    }
    defer bus.Close()

    // Set up your subscriptions and handlers
    // ...

    // Benchmark publishing
    count := 100000
    start := time.Now()

    for i := 0; i < count; i++ {
        bus.Publish("your/test/topic", []byte("test-payload"))
    }

    elapsed := time.Since(start)
    msgsPerSec := float64(count) / elapsed.Seconds()

    log.Printf("Published %d messages in %v (%f msgs/sec)",
        count, elapsed, msgsPerSec)
}
```
