# BlazeSub vs MochiMQTT Performance Comparison

This document summarizes the performance comparison between BlazeSub (a custom lock-free publish/subscribe system) and an embedded MQTT server using MochiMQTT.

The reason for this is that at NSX we were previously using MochiMQTT for a very concurrency heavy and low latency service and we were getting problems related with memory pressure.

Thus in order to consider blazesub a win over our previous solution it merits a thorough comparison with it. You can run the benchmark in this repository by running the tests in the `mqtt_comparison_test.go` file yourself.

These are results running in one machine and your mileage may vary. This is here more to give a full account of what we found while developing blaze and not to bash MochiMQTT in any way. It is an amazing piece of software and does MUCH MUCH more than what blazesub does.

## Benchmark Methodology

Three benchmark scenarios were implemented to compare performance:

1. **Basic Publish/Subscribe Performance**: Tests how efficiently messages are published and delivered to subscribers.
2. **Concurrent Publishing Performance**: Tests how well the systems handle concurrent publishing operations.
3. **Subscribe/Unsubscribe Operations**: Tests the efficiency of adding and removing subscriptions.

The benchmarks were run on an Intel Core i9-14900KF CPU with a benchmark time of 5 seconds for more accurate results. All handlers use equivalent lock-free implementations to ensure a fair comparison.

## Benchmark Results

### 1. Basic Publish/Subscribe Performance

| Implementation               | Operations/sec | Time/op     | Memory/op  | Allocations/op |
| ---------------------------- | -------------- | ----------- | ---------- | -------------- |
| BlazeSub (Worker Pool)       | 3,506,482      | 1,830 ns/op | 187 B/op   | 6 allocs/op    |
| BlazeSub (Direct Goroutines) | 6,627,648      | 900.9 ns/op | 261 B/op   | 10 allocs/op   |
| MochiMQTT                    | 4,390,024      | 1,372 ns/op | 2,471 B/op | 13 allocs/op   |

**Analysis**:

- BlazeSub with direct goroutines is 51% faster than MochiMQTT
- BlazeSub with direct goroutines is 89% faster than BlazeSub with worker pool
- MochiMQTT is 25.0% faster than BlazeSub with worker pool
- Both BlazeSub implementations use significantly less memory per operation than MochiMQTT (89.4% less for direct goroutines and 93% for worker pool)

### 2. Concurrent Publishing Performance

| Implementation               | Operations/sec | Time/op     | Memory/op  | Allocations/op |
| ---------------------------- | -------------- | ----------- | ---------- | -------------- |
| BlazeSub (Worker Pool)       | 11,083,462     | 534.7 ns/op | 88 B/op    | 2 allocs/op    |
| BlazeSub (Direct Goroutines) | 34,126,406     | 255.0 ns/op | 89 B/op    | 2 allocs/op    |
| MochiMQTT                    | 16,698,988     | 373.6 ns/op | 1,776 B/op | 10 allocs/op   |

**Analysis**:

- Under concurrent load, BlazeSub with direct goroutines is 31.7% faster than MochiMQTT
- BlazeSub with direct goroutines is 52.3% faster than BlazeSub with worker pool
- MochiMQTT is 30.1% faster than BlazeSub with worker pool
- Both BlazeSub implementations use approximately 95% less memory than MochiMQTT and make 80% fewer allocations

### 3. Subscribe/Unsubscribe Operations

| Implementation               | Operations/sec | Time/op     | Memory/op   | Allocations/op |
| ---------------------------- | -------------- | ----------- | ----------- | -------------- |
| BlazeSub (Worker Pool)       | 1,206,195      | 4,904 ns/op | 21,630 B/op | 52 allocs/op   |
| BlazeSub (Direct Goroutines) | 1,206,478      | 5,027 ns/op | 21,630 B/op | 52 allocs/op   |
| MochiMQTT                    | 4,298,108      | 1,349 ns/op | 2,481 B/op  | 32 allocs/op   |

**Analysis**:

- MochiMQTT is significantly faster for subscribe/unsubscribe operations (72.5% faster than BlazeSub with worker pool)
- MochiMQTT uses 88.5% less memory for these operations
- There's little performance difference between worker pool and direct goroutines for these operations in BlazeSub

### 3.1 Subscribe/Unsubscribe Operations After Optimization

| Implementation               | Operations/sec | Time/op     | Memory/op  | Allocations/op |
| ---------------------------- | -------------- | ----------- | ---------- | -------------- |
| BlazeSub (Worker Pool)       | ~175,000       | 5,735 ns/op | 9,208 B/op | 25 allocs/op   |
| BlazeSub (Direct Goroutines) | ~270,000       | 3,689 ns/op | 9,206 B/op | 25 allocs/op   |
| BlazeSub (Single Op)         | ~643,000       | 1,762 ns/op | 5,104 B/op | 11 allocs/op   |
| MochiMQTT                    | ~949,000       | 1,583 ns/op | 2,480 B/op | 32 allocs/op   |

**Analysis After Optimization**:

- BlazeSub single operations are now within 10-15% of MochiMQTT's performance
- Memory usage has been reduced by ~76%, now only about 2-3x MochiMQTT's usage
- Allocations reduced by 65-79%, now fewer than MochiMQTT (11-25 vs 32)
- Direct goroutines mode is now 35% faster than worker pool mode for subscribe/unsubscribe operations
- The performance gap with MochiMQTT has been significantly narrowed while maintaining lock-free design

### 4. Core Matching Performance

Looking at the detailed benchmarks for BlazeSub's core components:

- **HybridTrieExactMatch**: 6.106 ns/op with 0 B/op and 0 allocs/op
- **FindMatchingSubscriptions**: 4.978 ns/op with 0 B/op and 0 allocs/op
- **WildcardMatching**: 5.870 ns/op with 0 B/op and 0 allocs/op

**Analysis**: The core subscription matching in BlazeSub (the most critical operation for messaging systems) is extremely fast and allocates **zero memory**. This is a significant advantage for high-throughput scenarios.

## Overall Comparison

| Aspect                   | BlazeSub (Worker Pool) | BlazeSub (Direct Goroutines) | MochiMQTT            | Winner                 |
| ------------------------ | ---------------------- | ---------------------------- | -------------------- | ---------------------- |
| Publish/Subscribe Speed  | Slowest                | Fastest                      | Middle               | BlazeSub (Direct)      |
| Concurrent Publishing    | Slowest                | Fastest                      | Middle               | BlazeSub (Direct)      |
| Subscribe/Unsubscribe    | Slowest                | Slowest                      | Fastest              | MochiMQTT              |
| Memory Usage             | Very Efficient         | Very Efficient               | Less Efficient       | BlazeSub (Both)        |
| Memory Allocations       | Fewest                 | Few                          | Most                 | BlazeSub (Worker Pool) |
| Core Matching Operations | Allocation-Free        | Allocation-Free              | Requires Allocations | BlazeSub (Both)        |

## Key Observations

1. **Direct Goroutines vs Worker Pool**: BlazeSub with direct goroutines consistently outperforms the worker pool implementation for message publishing by 50-52%, highlighting the overhead introduced by the worker pool.

2. **Memory Efficiency**: Both BlazeSub implementations are dramatically more memory-efficient than MochiMQTT, using 89-95% less memory for publishing operations. In high-throughput systems, this means:

   - Less garbage collection pressure
   - Better cache locality
   - Lower memory usage overall

3. **Allocation-Free Core Operations**: BlazeSub's core matching operations (exact and wildcard) use zero memory allocations, which is ideal for high-performance systems where GC pauses can be problematic.

4. **Trade-offs in Subscribe/Unsubscribe**: MochiMQTT significantly outperforms BlazeSub for subscription management operations, though these are typically less frequent than message publishing.

## Conclusions

1. **For Raw Publishing Performance**:

   - BlazeSub with direct goroutines offers the best raw performance, being up to 34% faster than MochiMQTT
   - The worker pool implementation of BlazeSub introduces significant overhead and is slower than MochiMQTT

2. **For Resource Efficiency**:

   - Both BlazeSub implementations use significantly less memory (89-95% less) and make fewer allocations
   - This makes them more suitable for resource-constrained environments or long-running services

3. **For High-Scale Production**:

   - BlazeSub's lock-free design and zero-allocation core operations suggest better performance stability under varying loads
   - Using direct goroutines instead of the worker pool provides substantial performance benefits

4. **For Subscribe/Unsubscribe Heavy Workloads**:
   - MochiMQTT is significantly faster and more memory efficient for subscription management

## Recommendations

1. **For High-Throughput Applications**:

   - For maximum throughput, use BlazeSub with direct goroutines
   - The zero-allocation design of BlazeSub makes it superior for sustained high-throughput where GC pauses would be problematic

2. **For Resource-Constrained Environments**:

   - Both BlazeSub implementations have significantly lower memory footprints, making them good choices for memory-constrained environments
   - The choice between worker pool and direct goroutines should be based on whether controlling goroutine creation is a concern

3. **For Predictable Latency Requirements**:

   - BlazeSub with direct goroutines provides the most consistent performance with the lowest latency
   - The allocation-free design helps avoid GC-related latency spikes

4. **For Connection-Heavy Workloads**:
   - If your application involves frequent connection/disconnection of clients rather than sustained messaging, MochiMQTT may be more suitable

## Future Optimizations

While BlazeSub already demonstrates excellent performance, potential optimizations include:

1. **Subscription Management**: The subscribe/unsubscribe operations in BlazeSub could be optimized to be more competitive with MochiMQTT

2. **Memory Usage in Direct Goroutines**: For large loads, direct goroutines use slightly more memory than the worker pool implementation, which could potentially be optimized

3. **Real-World Workload Testing**: Testing with realistic message patterns, payload sizes, and subscription topologies would provide additional insights

These benchmarks confirm that BlazeSub with direct goroutines provides superior performance for high-throughput message publishing scenarios, while maintaining exceptional memory efficiency.
