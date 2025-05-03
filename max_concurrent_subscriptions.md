# MaxConcurrentSubscriptions Parameter Guide

## Overview

The `MaxConcurrentSubscriptions` parameter in BlazeSub is a critical performance tuning option that controls how messages are delivered to multiple subscribers. This document explains how this parameter works and provides evidence-based recommendations for optimal settings.

## How It Works

When a message is published to a topic with multiple subscribers, BlazeSub uses two different strategies to deliver the message:

1. **Individual Delivery**: If the number of subscribers is ≤ `MaxConcurrentSubscriptions`, BlazeSub creates one goroutine (or worker pool task) per subscriber
2. **Batched Delivery**: If the number of subscribers is > `MaxConcurrentSubscriptions`, BlazeSub processes all subscribers in a single goroutine/task

The default value is 10, which means topics with 10 or fewer subscribers will use individual delivery, while topics with more than 10 subscribers will use batched delivery.

## Benchmark Results

We conducted detailed benchmarks with 1000 subscribers per topic and varying `MaxConcurrentSubscriptions` values to understand the impact:

### Worker Pool Mode

| MaxConcurrent | Time (ns/op)     | Memory (B/op) | Allocations | Performance |
| ------------- | ---------------- | ------------- | ----------- | ----------- |
| 1-750         | ~10,000-11,000   | ~115          | 2           | Good        |
| 1000+         | ~345,000-347,000 | ~24,000       | 1,002       | 34x slower  |

### Direct Goroutines Mode

| MaxConcurrent | Time (ns/op)  | Memory (B/op) | Allocations | Performance |
| ------------- | ------------- | ------------- | ----------- | ----------- |
| 1-750         | ~9,600-11,200 | ~112-256      | 2           | Good        |
| 1000+         | ~164,000      | ~40,000       | 2,001       | 17x slower  |

## Performance Visualization

The following chart illustrates the dramatic performance cliff that occurs when MaxConcurrentSubscriptions reaches or exceeds the number of subscribers (1000 in our benchmark):

```
Performance (ns/op) - lower is better
│
│                                        Worker Pool          Direct Goroutines
350,000 ┼                                    ╭───────────────────┐
        │                                    │                   │
300,000 ┼                                    │                   │
        │                                    │                   │
250,000 ┼                                    │                   │
        │                                    │                   │
200,000 ┼                                    │                   │
        │                                    │                   │
150,000 ┼                                    │                   │     ╭───────────────┐
        │                                    │                   │     │                │
100,000 ┼                                    │                   │     │                │
        │                                    │                   │     │                │
 50,000 ┼                                    │                   │     │                │
        │                                    │                   │     │                │
 10,000 ┼────────────────────────────────────╯                   │     │                │
        │                                                        │     │                │
      0 ┼─────┬─────┬─────┬─────┬─────┬─────┬─────┬─────┬─────┬──┴─────┴────────────────┴─────
        │     │     │     │     │     │     │     │     │     │
        1     5     10    25    50   100   300   500   750  1000   2000
                                   MaxConcurrentSubscriptions
```

Notice how both modes maintain good performance until MaxConcurrentSubscriptions reaches 1000, at which point performance dramatically worsens.

## Key Findings

1. **Performance Cliff**: Both delivery modes experience a dramatic performance drop when `MaxConcurrentSubscriptions` reaches or exceeds the actual subscriber count

   - Worker Pool: 34x slower, 200x more memory
   - Direct Goroutines: 17x slower, 350x more memory

2. **Optimal Values**:

   - Worker Pool: Best performance at MaxConcurrent=5 (10,069 ns/op)
   - Direct Goroutines: Best performance at MaxConcurrent=750 (9,666 ns/op)

3. **Memory Usage**:
   - Below threshold: 2 allocations, ~112-256 bytes
   - At/above threshold: 1000+ allocations, 24,000-40,000 bytes

## Recommendations

1. **Never set MaxConcurrentSubscriptions equal to or higher than your maximum expected subscriber count per topic**

   - This is the most important rule - doing so creates a severe performance cliff
   - Example: If you expect topics to have up to 1000 subscribers, set MaxConcurrent to 750 or lower

2. **For typical pub/sub patterns**:

   - The default value of 10 is good for most use cases
   - Optimization is only needed for specialized workloads

3. **For high-throughput systems with many subscribers per topic**:
   - Worker Pool Mode: Set between 5-300
   - Direct Goroutines Mode: Set between 1-750

## Implementation Example

```go
config := blazesub.Config{
    // Other configuration...
    MaxConcurrentSubscriptions: 50,  // Good value for most use cases
    UseGoroutinePool: false,  // Direct goroutines mode for maximum throughput
}
bus, err := blazesub.NewBus(config)
```

## Advanced Considerations

1. **Hardware Scaling**: Systems with more CPU cores may benefit from higher values, but never exceed your max subscriber count
2. **Message Handler Complexity**: For lightweight handlers, lower values often perform better
3. **Mixed Workloads**: If some topics have many subscribers and others have few, err on the side of lower values

## Testing Your Workload

For optimal performance, benchmark with your specific workload characteristics:

1. Run tests with your actual message sizes
2. Test with your expected subscriber counts
3. Try different values and observe both throughput and memory usage
