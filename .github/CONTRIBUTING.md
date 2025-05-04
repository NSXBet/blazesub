# Contributing to BlazeSub

Thank you for your interest in contributing to BlazeSub! This document provides guidelines and information about our development process.

## 🧩 Development Process

1. **Fork the repository**
2. **Create a branch** with a descriptive name
3. **Make your changes**
4. **Run tests and benchmarks locally**
5. **Submit a pull request**

## 🧪 Testing

Always run tests before submitting a PR:

```bash
go test ./...
```

To run tests with race detection:

```bash
go test -race ./...
```

## 📊 Performance Benchmarking

Performance is a critical aspect of BlazeSub. Always run relevant benchmarks before and after your changes to ensure there are no performance regressions.

### Running Key Benchmarks

```bash
# Run throughput benchmarks (most important for overall performance)
go test -bench=BenchmarkThroughputWith1000Subscribers -benchmem

# Run delivery mode comparison benchmarks
go test -bench=BenchmarkPoolVsGoroutines/LargeLoad_ -benchmem

# Run MaxConcurrentSubscriptions benchmarks (important for tuning)
go test -bench=BenchmarkMaxConcurrentSubscriptionsDetailed -benchmem
```

### Comparing Before/After Benchmarks

We recommend using [benchstat](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat) to compare benchmark results:

```bash
# Install benchstat
go install golang.org/x/perf/cmd/benchstat@latest

# Run benchmarks before your changes
go test -bench=BenchmarkThroughputWith1000Subscribers -benchmem -count=5 > before.txt

# Make your changes, then run benchmarks again
go test -bench=BenchmarkThroughputWith1000Subscribers -benchmem -count=5 > after.txt

# Compare results
benchstat before.txt after.txt
```

## ⚙️ Continuous Integration

Our CI system automatically runs tests, linters, and performance benchmarks on your PR:

1. **Lint Check**: Ensures code style consistency
2. **Unit Tests**: Verifies functional correctness
3. **Performance Benchmarks**: Compares benchmark results with the main branch
4. **Performance Analysis**: Provides CPU and memory profiles

### Understanding Benchmark Results

The CI will automatically add comments to your PR with benchmark comparisons. Pay attention to any performance degradations highlighted in the results.

A degradation of more than 10% in any key benchmark will be flagged for your attention. While this won't automatically block your PR, you should investigate and address significant performance regressions.

### Using Performance Profiles

For deeper performance analysis, download the performance profiles artifact from the workflow run:

1. Go to the "Actions" tab in the repository
2. Click on your workflow run
3. Download the "performance-profiles" artifact
4. Analyze with `go tool pprof`

## 📝 Pull Request Guidelines

1. **Keep PRs focused** on a single change or feature
2. **Include tests** for new functionality
3. **Add benchmarks** for performance-critical code
4. **Update documentation** as needed
5. **Address CI feedback** before requesting review
6. **Check for performance regressions** in benchmark results

## 🚀 Performance Considerations

When working on BlazeSub, always keep these performance considerations in mind:

1. **Minimize allocations** - Heap allocations trigger GC and slow down the system
2. **Avoid locks** when possible - Use lock-free data structures where appropriate
3. **Optimize for common cases** - Fast paths for frequently executed code
4. **Profile before optimizing** - Use the provided profiling tools to find actual bottlenecks
5. **Benchmark thoroughly** - Test various scenarios including high concurrency

## 📚 Documentation

Update documentation when changing public APIs or behavior:

- Update comments in code
- Update `README.md` for high-level changes
- Update `USER_GUIDE.md` for user-facing changes
- Update `PERFORMANCE.md` for performance-related changes

## 🔄 Review Process

Pull requests are reviewed by maintainers focusing on:

1. Correctness
2. Performance
3. Code quality
4. Test coverage
5. Documentation

## 🙏 Thank You

Your contributions help make BlazeSub better! We appreciate your efforts to maintain the high-performance, high-quality standards of the project.
