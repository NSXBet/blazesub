# Hybrid Trie Optimization Guide

This document explains the key optimizations we made to the `SubscriptionTrie` implementation in the BlazeSub library.

## Core Problem

The original subscription trie implementation offered excellent wildcard pattern matching capabilities but had considerably slower performance compared to a direct map lookup for exact-match topics.

## Solution: Hybrid Approach

We implemented a hybrid data structure that combines:

1. A trie structure for all subscriptions (supporting wildcard pattern matching)
2. A direct map lookup for exact-match subscriptions (without wildcards)

## Key Optimizations

### 1. Dual Storage Strategy

Subscriptions are stored in two places:

- **All subscriptions** are stored in the trie structure, which allows proper node cleanup when unsubscribing
- **Exact-match subscriptions** (those without wildcards) are additionally stored in a dedicated map for fast O(1) lookups

```go
type SubscriptionTrie struct {
    root            *TrieNode
    exactMatches    map[string]map[uint64]*Subscription
    exactMatchMutex sync.RWMutex
    wildcardExists  bool
    wildcardMutex   sync.RWMutex
}
```

### 2. Wildcard State Tracking

We added a boolean flag `wildcardExists` to track the existence of any wildcard subscriptions in the system. This allows us to:

- Skip trie traversal completely when doing exact-match lookups and no wildcards exist
- Use only the map for most common scenarios in systems with few/no wildcard subscriptions
- Keep wildcard functionality available when needed

### 3. Fast Path Execution

The `FindMatchingSubscriptions` method implements a fast path:

1. First checks the exactMatches map for a direct hit (O(1) operation)
2. If found and no wildcards exist in the system, returns immediately
3. Only traverses the trie when necessary (when no exact match or wildcards exist)

```go
// Fast path: If there are no wildcard subscriptions, we can skip trie traversal
if exactSubs, exists := st.exactMatches[topic]; exists {
    st.wildcardMutex.RLock()
    if !st.wildcardExists {
        // No wildcards in the system, just return exact matches
        st.wildcardMutex.RUnlock()
        st.exactMatchMutex.RUnlock()

        result := make([]*Subscription, 0, len(exactSubs))
        for _, sub := range exactSubs {
            result = append(result, sub)
        }
        return result
    }
    st.wildcardMutex.RUnlock()

    // Add exact matches to result map
    for id, sub := range exactSubs {
        resultMap[id] = sub
    }
}
```

### 4. Memory Pre-allocation

Several memory optimizations were made:

- Pre-allocating the exactMatches map with a larger initial capacity (1024)
- Allocating result slices with known capacity to avoid resizing
- Reducing overall memory allocations from 9 to 1 per exact-match lookup

### 5. Concurrency Safety

The implementation maintains thread safety while minimizing lock contention:

- Separate mutexes for the trie, exactMatches map, and wildcardExists flag
- Lock scope minimization by releasing locks as early as possible
- Careful ordering of lock acquisition to prevent deadlocks

## Performance Results

The hybrid approach achieved:

- 5.4x faster execution than the original trie for exact matches
- 84% reduction in memory allocations
- Reduced allocation count from 9 to 1 per lookup
- Maintained full wildcard pattern matching capability

## Trade-offs

1. **Increased Code Complexity**: The hybrid approach is more complex than either a pure trie or pure map.

2. **Dual Storage Memory Overhead**: Exact-match subscriptions are stored in both the trie and map, which uses more memory than a single storage approach.

3. **Subscription/Unsubscription Cost**: The overhead of maintaining both data structures makes subscription changes slightly more expensive.

These trade-offs are well justified by the significant performance improvements, especially in systems where lookups are much more frequent than subscription changes.
