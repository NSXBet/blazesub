# BlazeSub vs MQTT Performance Comparison

This document summarizes the performance comparison between BlazeSub (a custom trie-based publish/subscribe system) and an embedded MQTT server using mochimqtt.

## Benchmark Methodology

Three benchmark scenarios were implemented to compare performance:

1. **Basic Publish/Subscribe Performance**: Tests how efficiently messages are published and delivered to subscribers.
2. **Concurrent Publishing Performance**: Tests how well the systems handle concurrent publishing operations.
3. **Subscribe/Unsubscribe Operations**: Tests the efficiency of adding and removing subscriptions.

The benchmarks were run on an Intel Core i9-14900KF CPU.

## Benchmark Results

### 1. Basic Publish/Subscribe Performance

| Implementation | Operations/sec | Time/op   | Memory/op | Allocations/op |
| -------------- | -------------- | --------- | --------- | -------------- |
| BlazeSub       | ~550,000       | ~2,120 ns | 410 B     | 8              |
| MQTT Server    | ~940,000       | ~1,270 ns | 2,471 B   | 13             |

**Analysis**:

- The MQTT server is about **40% faster** in raw publish/subscribe operations
- However, BlazeSub uses **83% less memory** per operation
- BlazeSub makes **38% fewer memory allocations** per operation

### 2. Concurrent Publishing Performance

| Implementation | Operations/sec | Time/op | Memory/op | Allocations/op |
| -------------- | -------------- | ------- | --------- | -------------- |
| BlazeSub       | ~2,250,000     | ~550 ns | 120 B     | 3              |
| MQTT Server    | ~4,200,000     | ~280 ns | 1,776 B   | 10             |

**Analysis**:

- The MQTT server handles **~2x more operations per second** under concurrent load
- However, BlazeSub uses **93% less memory** per operation
- BlazeSub makes **70% fewer memory allocations** per operation

### 3. Subscribe/Unsubscribe Operations

| Implementation | Operations/sec | Time/op   | Memory/op | Allocations/op |
| -------------- | -------------- | --------- | --------- | -------------- |
| BlazeSub       | ~1,500,000     | ~790 ns   | 1,536 B   | 23             |
| MQTT Server    | ~900,000       | ~1,280 ns | 2,480 B   | 32             |

**Analysis**:

- BlazeSub is **~38% faster** at subscribe/unsubscribe operations
- BlazeSub uses **38% less memory** per operation
- BlazeSub makes **28% fewer memory allocations** per operation

## Overall Comparison

| Aspect                  | BlazeSub       | MQTT Server    | Winner   |
| ----------------------- | -------------- | -------------- | -------- |
| Publish/Subscribe Speed | Slower         | Faster         | MQTT     |
| Concurrent Publishing   | Slower         | Faster         | MQTT     |
| Subscribe/Unsubscribe   | Faster         | Slower         | BlazeSub |
| Memory Usage            | Very Efficient | Less Efficient | BlazeSub |
| Memory Allocations      | Fewer          | More           | BlazeSub |

## Conclusions

1. **For Raw Performance**:

   - The MQTT server offers better raw publishing performance, particularly in concurrent scenarios
   - This might make it more suitable for high-throughput applications where memory usage is less critical

2. **For Resource Efficiency**:

   - BlazeSub consistently uses significantly less memory and makes fewer allocations
   - This could make it more suitable for resource-constrained environments or long-running services where memory efficiency is crucial

3. **For Subscription Management**:

   - BlazeSub performs better at subscribe/unsubscribe operations
   - This could make it more suitable for applications with frequently changing subscription patterns

4. **Optimization Opportunities**:
   - BlazeSub's publish operation could potentially be optimized further to match MQTT's performance
   - The significant memory efficiency of BlazeSub suggests its core data structure (subscription trie) is well-optimized

## Recommendations

1. For high-throughput applications with stable subscription patterns and sufficient memory resources, the MQTT server may be preferable.

2. For applications with more dynamic subscription patterns, memory constraints, or where resource efficiency is crucial, BlazeSub may be the better choice.

3. Consider the specific memory and CPU constraints of your deployment environment when choosing between these implementations.

4. The performance gap in basic publishing operations suggests potential further optimization opportunities in BlazeSub's publish mechanism.

## Performance Profiling Insights

CPU profiling of BlazeSub reveals several potential optimization areas:

1. **Lock Contention**: A significant amount of time (~41.6%) is spent in lock operations, particularly `runtime.lock2` and related functions. This suggests potential lock contention in the subscription trie, especially during concurrent operations.

2. **Worker Pool Overhead**: The `ants` worker pool used for publishing messages (`github.com/panjf2000/ants/v2`) has some overhead, with functions like `(*Pool).Submit` and pool management taking significant CPU time.

3. **Topic Matching**: The `FindMatchingSubscriptions` method (~2.9% of CPU time) could be optimized further, as it's a critical path for message publishing.

4. **Memory Allocation**: While BlazeSub uses less memory than MQTT, there's still some time spent in `mallocgc` and related functions, suggesting potential for further allocation optimizations.

### Optimization Opportunities

1. **Lock Granularity**: Consider using more fine-grained locking in the subscription trie to reduce contention, especially for read operations like topic matching.

2. **Worker Pool Tuning**: Experiment with different worker pool configurations (worker count, queue size) to find the optimal balance for your specific workload.

3. **Subscription Data Structure**: The trie implementation is memory-efficient but could potentially be optimized for faster lookups at the cost of some additional memory.

4. **Pre-allocation**: Further reduce allocations by pre-allocating common data structures, particularly for message handling paths.

5. **Lock-Free Alternatives**: Consider using atomic operations or lock-free data structures in critical paths where appropriate.

## Future Work

1. Profiling BlazeSub's publish path to identify bottlenecks
2. Testing with larger topic spaces and more complex wildcard patterns
3. Measuring latency distribution instead of just averages
4. Testing under realistic message payload sizes and patterns
5. Examining performance under backpressure scenarios
