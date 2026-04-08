# APICerebrus Benchmark Suite

Comprehensive benchmark tests for APICerebrus API Gateway components.

## Overview

This benchmark suite provides performance measurements for:

- **Gateway Components**: Router, proxy, load balancers, WebSocket handling
- **Plugin System**: JWT validation, rate limiting, CORS, caching
- **Store Layer**: SQLite operations, transactions, batch inserts

## Quick Start

```bash
# Run all benchmarks
go test -bench=. ./test/benchmark/...

# Run with the benchmark runner script
./scripts/run-benchmarks.sh

# Run with profiling
./scripts/run-benchmarks.sh --all-profiles -t 30s
```

## Running Benchmarks

### Basic Usage

```bash
# Run all benchmarks
go test -bench=. ./test/benchmark/...

# Run specific benchmark categories
go test -bench=BenchmarkRouter ./test/benchmark/...     # Routing only
go test -bench=BenchmarkJWT ./test/benchmark/...        # JWT only
go test -bench=BenchmarkUser ./test/benchmark/...       # Store only

# Run with profiling
go test -bench=. -cpuprofile=cpu.prof -memprofile=mem.prof ./test/benchmark/...
```

### Using the Benchmark Runner Script

```bash
# Run all benchmarks with defaults
./scripts/run-benchmarks.sh

# Run with extended duration and multiple iterations
./scripts/run-benchmarks.sh -t 5s -c 3

# Run with full profiling
./scripts/run-benchmarks.sh --all-profiles -t 30s

# Run only specific benchmark categories
./scripts/run-benchmarks.sh --gateway-only --cpu
./scripts/run-benchmarks.sh --plugin-only --mem
./scripts/run-benchmarks.sh --store-only

# View help
./scripts/run-benchmarks.sh --help
```

## Performance Baselines

### Gateway Benchmarks

| Benchmark | Baseline | Description |
|-----------|----------|-------------|
| `BenchmarkRouterExactMatch` | ~500 ns/op | Exact path matching |
| `BenchmarkRouterParameterizedMatch` | ~800 ns/op | Path parameter extraction |
| `BenchmarkRouterWildcardMatch` | ~1000 ns/op | Wildcard path matching |
| `BenchmarkRouterLargeRouteSet` | ~1500 ns/op | 100 routes lookup |
| `BenchmarkRouterHostBasedRouting` | ~600 ns/op | Virtual host routing |
| `BenchmarkRouterParallel` | ~2000 ns/op | Concurrent routing |
| `BenchmarkProxyThroughput` | ~50,000 req/sec | Single-threaded proxy |
| `BenchmarkProxyParallelThroughput` | ~100,000 req/sec | Concurrent proxy |
| `BenchmarkLoadBalancerRoundRobin` | ~50 ns/op | Round-robin selection |
| `BenchmarkLoadBalancerLeastConn` | ~80 ns/op | Least connections |
| `BenchmarkLoadBalancerIPHash` | ~200 ns/op | IP-based hashing |
| `BenchmarkGatewayEndToEnd` | ~10,000 req/sec | Full request flow |

### Plugin Benchmarks

| Benchmark | Baseline | Description |
|-----------|----------|-------------|
| `BenchmarkJWTValidationHS256` | ~5,000 val/sec | HMAC JWT validation |
| `BenchmarkJWTValidationRS256` | ~1,000 val/sec | RSA JWT validation |
| `BenchmarkRateLimitTokenBucket` | ~500,000 checks/sec | Token bucket algorithm |
| `BenchmarkRateLimitFixedWindow` | ~600,000 checks/sec | Fixed window algorithm |
| `BenchmarkRateLimitSlidingWindow` | ~400,000 checks/sec | Sliding window algorithm |
| `BenchmarkCORSSimpleRequest` | ~200 ns/op | Simple CORS handling |
| `BenchmarkCORSPreflightRequest` | ~300 ns/op | Preflight handling |
| `BenchmarkCacheGetHit` | ~100 ns/op | Cache hit retrieval |
| `BenchmarkCacheGetMiss` | ~50 ns/op | Cache miss check |
| `BenchmarkCacheSet` | ~500 ns/op | Cache write |
| `BenchmarkPipelineExecution` | ~1,000 exec/sec | 3-plugin pipeline |

### Store Benchmarks

| Benchmark | Baseline | Description |
|-----------|----------|-------------|
| `BenchmarkUserCreate` | ~2,000 inserts/sec | User creation |
| `BenchmarkUserFindByID` | ~50,000 lookups/sec | Primary key lookup |
| `BenchmarkUserFindByEmail` | ~40,000 lookups/sec | Indexed lookup |
| `BenchmarkUserList` | ~5,000 lists/sec | Paginated listing |
| `BenchmarkUserUpdateCreditBalance` | ~3,000 updates/sec | Atomic credit update |
| `BenchmarkAPIKeyCreate` | ~1,500 keys/sec | Key generation |
| `BenchmarkAPIKeyResolveUser` | ~30,000 res/sec | JOIN query |
| `BenchmarkTransactionThroughput` | ~2,000 tx/sec | Transaction rate |
| `BenchmarkConcurrentUserReads` | ~100,000 reads/sec | Parallel reads |
| `BenchmarkRawQuery` | ~100,000 queries/sec | Simple SELECT |

## Interpreting Results

### Understanding Benchmark Output

```
BenchmarkRouterExactMatch-16     2345678    512 ns/op    0 B/op    0 allocs/op
```

- `BenchmarkRouterExactMatch-16`: Benchmark name with GOMAXPROCS (16)
- `2345678`: Number of iterations executed
- `512 ns/op`: Nanoseconds per operation (lower is better)
- `0 B/op`: Bytes allocated per operation (lower is better)
- `0 allocs/op`: Heap allocations per operation (lower is better)

### Performance Regression Detection

Use `benchcmp` or `benchstat` to compare results:

```bash
# Install benchstat
go install golang.org/x/perf/cmd/benchstat@latest

# Run benchmarks twice and compare
go test -bench=. ./test/benchmark/... > old.txt
go test -bench=. ./test/benchmark/... > new.txt
benchstat old.txt new.txt
```

## Profiling

### CPU Profiling

```bash
# Generate CPU profile
go test -bench=. -cpuprofile=cpu.prof ./test/benchmark/...

# Analyze with pprof
go tool pprof cpu.prof

# Or launch web interface
go tool pprof -http=:8080 cpu.prof
```

### Memory Profiling

```bash
# Generate memory profile
go test -bench=. -memprofile=mem.prof ./test/benchmark/...

# Analyze allocations
go tool pprof -alloc_objects mem.prof

# Analyze allocated space
go tool pprof -alloc_space mem.prof
```

### Block Profiling

```bash
# Detect blocking operations
go test -bench=. -blockprofile=block.prof ./test/benchmark/...
go tool pprof block.prof
```

### Mutex Profiling

```bash
# Detect mutex contention
go test -bench=. -mutexprofile=mutex.prof ./test/benchmark/...
go tool pprof mutex.prof
```

## Benchmark Categories

### Gateway Benchmarks (`gateway_bench_test.go`)

**Router Benchmarks:**
- `BenchmarkRouterExactMatch` - Static route matching
- `BenchmarkRouterParameterizedMatch` - Dynamic parameter extraction
- `BenchmarkRouterWildcardMatch` - Wildcard path handling
- `BenchmarkRouterLargeRouteSet` - Scale testing with 100 routes
- `BenchmarkRouterHostBasedRouting` - Virtual host routing
- `BenchmarkRouterParallel` - Concurrent routing performance

**Proxy Benchmarks:**
- `BenchmarkProxyThroughput` - Basic forwarding performance
- `BenchmarkProxyParallelThroughput` - Concurrent proxy handling
- `BenchmarkProxyLargeResponse` - Large payload handling

**Load Balancer Benchmarks:**
- `BenchmarkLoadBalancerRoundRobin` - Round-robin algorithm
- `BenchmarkLoadBalancerWeightedRoundRobin` - Weighted distribution
- `BenchmarkLoadBalancerLeastConn` - Least connections
- `BenchmarkLoadBalancerIPHash` - IP-based hashing
- `BenchmarkLoadBalancerConsistentHash` - Consistent hashing
- `BenchmarkLoadBalancerParallel` - Concurrent selection

**WebSocket Benchmarks:**
- `BenchmarkWebSocketUpgradeDetection` - Upgrade header parsing

**End-to-End Benchmarks:**
- `BenchmarkGatewayEndToEnd` - Full request lifecycle
- `BenchmarkGatewayWithStripPath` - Path manipulation

### Plugin Benchmarks (`plugin_bench_test.go`)

**JWT Validation Benchmarks:**
- `BenchmarkJWTValidationHS256` - HMAC-SHA256 validation
- `BenchmarkJWTValidationRS256` - RSA-SHA256 validation
- `BenchmarkJWTValidationWithClaims` - Claim extraction

**Rate Limiting Benchmarks:**
- `BenchmarkRateLimitTokenBucket` - Token bucket algorithm
- `BenchmarkRateLimitFixedWindow` - Fixed window algorithm
- `BenchmarkRateLimitSlidingWindow` - Sliding window algorithm
- `BenchmarkRateLimitLeakyBucket` - Leaky bucket algorithm
- `BenchmarkRateLimitByConsumer` - Consumer-scoped limiting
- `BenchmarkRateLimitByIP` - IP-scoped limiting
- `BenchmarkRateLimitComposite` - Composite scope limiting
- `BenchmarkRateLimitParallel` - Concurrent limiting

**CORS Benchmarks:**
- `BenchmarkCORSSimpleRequest` - Simple request handling
- `BenchmarkCORSPreflightRequest` - Preflight handling
- `BenchmarkCORSWildcardOrigin` - Wildcard matching
- `BenchmarkCORSParallel` - Concurrent CORS handling

**Cache Benchmarks:**
- `BenchmarkCacheGetHit` - Cache hit performance
- `BenchmarkCacheGetMiss` - Cache miss performance
- `BenchmarkCacheSet` - Write performance
- `BenchmarkCacheSetLargeValue` - Large value writes
- `BenchmarkCacheGenerateKey` - Key generation
- `BenchmarkCacheParallel` - Concurrent access

**Pipeline Benchmarks:**
- `BenchmarkPipelineExecution` - Plugin chain execution
- `BenchmarkPipelineParallel` - Concurrent pipeline execution

### Store Benchmarks (`store_bench_test.go`)

**User Repository Benchmarks:**
- `BenchmarkUserCreate` - User insertion
- `BenchmarkUserFindByID` - Primary key lookup
- `BenchmarkUserFindByEmail` - Email lookup
- `BenchmarkUserList` - Paginated listing
- `BenchmarkUserUpdate` - Update operations
- `BenchmarkUserUpdateCreditBalance` - Atomic credit updates

**API Key Repository Benchmarks:**
- `BenchmarkAPIKeyCreate` - Key generation
- `BenchmarkAPIKeyFindByHash` - Hash lookup
- `BenchmarkAPIKeyListByUser` - User key listing
- `BenchmarkAPIKeyResolveUser` - JOIN resolution

**Batch Operations:**
- `BenchmarkBatchInsertUsers` - Batch insertion
- `BenchmarkTransactionThroughput` - Transaction rate

**Concurrent Access:**
- `BenchmarkConcurrentUserReads` - Parallel reads
- `BenchmarkConcurrentCreditUpdates` - Contention testing

**Search Benchmarks:**
- `BenchmarkUserSearch` - Full-text search
- `BenchmarkUserFilterByStatus` - Filtered queries

**Raw SQL Benchmarks:**
- `BenchmarkRawQuery` - Direct SQL performance
- `BenchmarkRawInsert` - Raw insertion speed

**Memory Usage:**
- `BenchmarkLargeDatasetQuery` - Large dataset handling

## Best Practices

### Running Benchmarks

1. **Close other applications** to reduce noise
2. **Run multiple times** for statistical significance
3. **Use `-count=5`** or higher for variance analysis
4. **Run on dedicated hardware** for consistent results
5. **Document environment**: CPU, RAM, disk type

### Benchmark Writing

1. **Use `b.ResetTimer()`** after setup
2. **Use `b.ReportAllocs()`** to track allocations
3. **Avoid `b.N` dependent setup** inside the loop
4. **Use parallel benchmarks** for concurrency testing
5. **Document baselines** in comments

### Interpreting Results

1. **Focus on ns/op** for latency-sensitive operations
2. **Check allocations** for GC pressure
3. **Compare with baseline** for regressions
4. **Use benchstat** for statistical analysis
5. **Profile before optimizing**

## Continuous Integration

Add benchmarks to CI pipeline:

```yaml
# .github/workflows/benchmark.yml
name: Benchmark
on: [push, pull_request]

jobs:
  benchmark:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      
      - name: Run benchmarks
        run: go test -bench=. -benchtime=1s ./test/benchmark/... > benchmark.txt
      
      - name: Upload results
        uses: actions/upload-artifact@v3
        with:
          name: benchmark-results
          path: benchmark.txt
      
      - name: Compare with main
        if: github.event_name == 'pull_request'
        run: |
          git checkout main
          go test -bench=. -benchtime=1s ./test/benchmark/... > main.txt
          go install golang.org/x/perf/cmd/benchstat@latest
          benchstat main.txt benchmark.txt
```

## Troubleshooting

### Benchmarks Running Slowly

- Check if `-short` flag is set (skips long benchmarks)
- Verify `GOMAXPROCS` matches CPU cores
- Check for resource contention (other processes)

### Inconsistent Results

- Run with higher `-count` value
- Disable CPU frequency scaling
- Run on bare metal (not VMs)
- Ensure sufficient warmup

### Memory Benchmarks

- Use `-benchmem` flag explicitly
- Check for memory leaks with multiple runs
- Use `-memprofile` for detailed analysis

## Contributing

When adding new benchmarks:

1. Follow existing naming convention: `Benchmark<Component><Operation>`
2. Include baseline numbers in comments
3. Add to appropriate category file
4. Document in this README
5. Run `./scripts/run-benchmarks.sh` to verify

---

## Historical Performance Data

### Previous Results (2026-04-05)

| Metric | Target | Status |
|--------|--------|--------|
| RPS (Requests Per Second) | 50,000+ | Partial (29,943 RPS achieved) |
| Latency (p99) | <1ms | Gap (8.6ms observed) |
| Memory per 10k connections | <10MB | Pass (8.6MB observed) |
| Binary size | <50MB | Pass (29.7MB) |

See the original README content in git history for detailed historical analysis.
