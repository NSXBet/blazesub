# Worker Pool vs Direct Goroutines Performance Analysis

BlazeSub supports two different modes for handling message delivery:

1. **Worker Pool Mode** - Uses the `ants` worker pool for managing and reusing goroutines
2. **Direct Goroutines Mode** - Creates new goroutines for each message delivery

This document summarizes the performance characteristics of each approach based on benchmark results.

## Basic Performance (Single-Threaded)

| Scenario        | Worker Pool | Direct Goroutines | MochiMQTT    | Direct vs Pool   | Direct vs MQTT   |
| --------------- | ----------- | ----------------- | ------------ | ---------------- | ---------------- |
| Base Publishing | 1,830 ns/op | 900.9 ns/op       | 1,372 ns/op  | **50.8% faster** | **34.4% faster** |
| Memory Usage    | 187 B/op    | 261 B/op          | 2,471 B/op   | 39.6% more       | **89.4% less**   |
| Allocations     | 6 allocs/op | 10 allocs/op      | 13 allocs/op | 66.7% more       | 23.1% less       |

For basic publishing operations, direct goroutines are significantly faster than both the worker pool implementation and MochiMQTT. While direct goroutines use slightly more memory than the worker pool, both BlazeSub implementations are dramatically more memory-efficient than MochiMQTT.

## Concurrent Performance (Multi-Threaded)

| Scenario              | Worker Pool | Direct Goroutines | MochiMQTT    | Direct vs Pool   | Direct vs MQTT   |
| --------------------- | ----------- | ----------------- | ------------ | ---------------- | ---------------- |
| Concurrent Publishing | 534.7 ns/op | 255.0 ns/op       | 373.6 ns/op  | **52.3% faster** | **31.7% faster** |
| Memory Usage          | 88 B/op     | 89 B/op           | 1,776 B/op   | Nearly identical | **95.0% less**   |
| Allocations           | 2 allocs/op | 2 allocs/op       | 10 allocs/op | Identical        | **80% less**     |

Under concurrent load, the performance advantage of direct goroutines becomes even more pronounced. Direct goroutines are over 52% faster than the worker pool and nearly 32% faster than MochiMQTT, while maintaining exceptional memory efficiency.

## Subscription Operations

| Mode                  | Worker Pool  | Direct Goroutines | MochiMQTT    |
| --------------------- | ------------ | ----------------- | ------------ |
| Subscribe/Unsubscribe | 4,904 ns/op  | 5,027 ns/op       | 1,349 ns/op  |
| Memory Usage          | 21,630 B/op  | 21,630 B/op       | 2,481 B/op   |
| Allocations           | 52 allocs/op | 52 allocs/op      | 32 allocs/op |

For subscription management operations, MochiMQTT significantly outperforms both BlazeSub implementations, being approximately 72.5% faster and using 88.5% less memory. There's little difference between the worker pool and direct goroutines implementations for these operations in BlazeSub.

## Analysis and Recommendations

### Why Direct Goroutines Outperform Worker Pools and MochiMQTT

1. **Reduced Overhead**: Worker pools introduce additional synchronization and management overhead, which direct goroutines avoid entirely.
2. **Simpler Scheduling**: Direct goroutines leverage Go's built-in scheduler without additional abstraction layers.
3. **Zero Allocations in Core Operations**: BlazeSub's core subscription matching operations allocate no memory, giving it an advantage over MochiMQTT.
4. **Allocation Efficiency**: Both BlazeSub modes use significantly less memory than MochiMQTT for publishing operations.

### When to Use Each Mode

#### Use Direct Goroutines When:

- You need maximum throughput and minimum latency
- Your message handlers execute quickly (most common case)
- Memory pressure is not a critical concern compared to performance
- Your system has sufficient resources to handle temporary goroutine creation peaks

#### Use Worker Pools When:

- Your message handlers perform heavy or long-running operations
- You need to precisely control concurrency limits
- You're operating in a resource-constrained environment where goroutine creation needs to be limited
- You need protection against "goroutine explosion" under extreme load

#### Consider MochiMQTT When:

- Your workload involves frequent subscription changes rather than sustained publishing
- Memory efficiency is less important than subscription management performance
- You require full MQTT protocol compatibility

### Configuration Example

To use direct goroutines:

```go
config := blazesub.Config{
    // Other configuration...
    UseGoroutinePool: false, // Use direct goroutines instead of worker pool
}
bus, err := blazesub.NewBus(config)
```

To use the worker pool (default):

```go
config := blazesub.Config{
    // Other configuration...
    UseGoroutinePool: true, // Use worker pool (default)
}
bus, err := blazesub.NewBus(config)
```

Or simply use the defaults, which include the worker pool:

```go
bus, err := blazesub.NewBusWithDefaults()
```

## Conclusion

For most typical pub/sub use cases where message handlers execute quickly, direct goroutines provide superior performance, being:

- 50.8% faster than the worker pool implementation
- 34.4% faster than MochiMQTT for basic publishing
- 52.3% faster than the worker pool and 31.7% faster than MochiMQTT under concurrent load

While MochiMQTT excels at subscription management operations, BlazeSub with direct goroutines offers the best overall performance for high-throughput message publishing scenarios while maintaining excellent memory efficiency.

Both delivery options are available in BlazeSub, giving users the flexibility to choose the approach that best suits their specific requirements.
