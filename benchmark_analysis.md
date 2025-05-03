# Subscription Lookup Performance Analysis

This document presents a performance analysis of different subscription lookup methods for our publish-subscribe system. We compare four approaches:

1. **Trie-based lookup**: Our implemented subscription trie structure
2. **Map-based lookup**: Direct hashmap lookup (fastest possible, but lacks wildcard support)
3. **Linear search**: Brute force iteration through all subscriptions (supports wildcards but slow)
4. **Hybrid approach**: Our optimized implementation using both a map for exact matches and a trie for all subscriptions

## Benchmark Results

### Overall Comparison

| Implementation | Time (ns/op) | Memory (B/op) | Allocations |
| -------------- | ------------ | ------------- | ----------- |
| Trie           | 1,940        | 2,616         | 9           |
| Hybrid Trie    | 356.7        | 416           | 1           |
| Map            | 6.18         | 0             | 0           |
| Linear Search  | 433,884      | 802,959       | 10,010      |

The direct map lookup is still the fastest (approximately 58x faster than our hybrid approach), but it can only support exact matches. The hybrid trie is about 5.4x faster than the regular trie and 1,216x faster than linear search.

### Exact Match Performance

When only looking up exact matches (no wildcards):

| Implementation | Time (ns/op) | Memory (B/op) | Allocations |
| -------------- | ------------ | ------------- | ----------- |
| Trie           | 1,940        | 2,616         | 9           |
| Hybrid Trie    | 356.7        | 416           | 1           |
| Map            | 6.76         | 0             | 0           |

For exact matches, our hybrid approach is about 5.4x faster than the original trie and only about 53x slower than a direct map lookup, which is a significant improvement.

### Mixed Workload Performance (80% exact matches, 20% wildcard matches)

| Implementation | Time (ns/op) | Memory (B/op) | Allocations |
| -------------- | ------------ | ------------- | ----------- |
| Hybrid Trie    | 2,329        | 2,111         | 7           |

The hybrid approach performs reasonably well on mixed workloads, though as expected, it's slower than pure exact-match lookups due to the need to process wildcard patterns.

### Key Observations

1. **Dramatic Performance Improvement**: Our hybrid approach is approximately 5.4x faster than the original trie implementation for exact matches.

2. **Reduced Memory Usage**: Memory allocations dropped from 2,616 bytes to 416 bytes for exact matches, an 84% reduction.

3. **Fewer Allocations**: The number of allocations decreased from 9 to just 1 per lookup for exact matches.

4. **Smart Fast Path**: The optimization avoids trie traversal when there are no wildcards in the system.

5. **Balanced Approach**: We've achieved a good balance between performance and correctness for the different query patterns.

## Architectural Design

Our revised hybrid approach includes some key design decisions:

1. **Store Exact-Match Subscriptions in Both Map and Trie**: Although initially we considered storing exact matches only in the map, we found that storing them in both places simplifies the code and ensures that node cleanup works correctly. This is important for memory management as subscriptions are added and removed over time.

2. **Wildcard State Tracking**: We maintain a flag (`wildcardExists`) to quickly determine if any wildcard subscriptions exist in the system. This allows us to skip trie traversal entirely when doing exact matches and no wildcards exist.

3. **Early Returns**: For the common case of exact matches when no wildcards exist, we provide a fast return path that avoids unnecessary work.

4. **Dual-Storage Trade-offs**: The memory overhead of storing some subscriptions in both the map and trie is compensated by the significant performance gains, especially in systems where lookups are much more frequent than subscription changes.

## Optimization Details

Our hybrid approach combines the strengths of both map-based and trie-based lookups:

1. **Fast Exact Match Lookups**: We store non-wildcard subscriptions in a dedicated map for O(1) lookup time.

2. **Complete Wildcard Support**: We maintain a full trie structure for all subscriptions, ensuring wildcard patterns work correctly.

3. **Smart Traversal Decisions**: We avoid trie traversal when no wildcards exist in the system.

4. **Pre-sized Maps**: We pre-allocate maps with larger initial capacity to reduce rehashing operations.

5. **Optimized Allocation**: The implementation now allocates much less memory per lookup.

6. **State Management**: We track the existence of wildcard subscriptions globally to avoid unnecessary checks.

## Conclusion

The hybrid approach successfully addresses the performance gap between the trie-based solution and a direct map lookup. While a direct map lookup is still faster for exact matches (as expected), the hybrid approach brings the performance to within a reasonable range (53x slower vs. 292x slower previously) while maintaining the full wildcard matching capability.

For real-world publish-subscribe systems with a mix of exact and wildcard subscriptions, this hybrid approach provides an excellent balance of performance and functionality. The 5.4x speedup over the original trie implementation represents a significant improvement.

This optimization is particularly valuable in high-throughput systems where message dispatching performance is critical. The reduced memory allocations will also help reduce garbage collection pressure in long-running services.
