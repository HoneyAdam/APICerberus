# Redis-Backed Distributed Rate Limiting

APICerebrus supports distributed rate limiting using Redis, enabling consistent rate limiting across multiple gateway instances in a cluster deployment.

## Overview

By default, APICerebrus uses in-memory rate limiting algorithms (token bucket, sliding window, fixed window, leaky bucket). While these are fast and efficient for single-instance deployments, they don't share state across multiple gateway instances.

Redis-backed rate limiting provides:

- **Distributed state**: All gateway instances share the same rate limit counters
- **Persistence**: Rate limit state survives gateway restarts
- **Consistency**: Same rate limits applied regardless of which instance handles the request
- **Fallback support**: Automatically falls back to local rate limiting if Redis is unavailable

## Configuration

Add the `redis` section to your configuration file:

```yaml
redis:
  enabled: true
  address: "redis-cluster:6379"
  password: "your-redis-password"
  database: 0
  max_retries: 3
  dial_timeout: "5s"
  read_timeout: "3s"
  write_timeout: "3s"
  pool_size: 10
  min_idle_conns: 2
  key_prefix: "ratelimit:"
  fallback_to_local: true
  sync_local_on_miss: true
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable Redis-backed rate limiting |
| `address` | string | `"localhost:6379"` | Redis server address |
| `password` | string | `""` | Redis password (empty for no auth) |
| `database` | int | `0` | Redis database number |
| `max_retries` | int | `3` | Maximum retry attempts for failed operations |
| `dial_timeout` | duration | `"5s"` | Connection timeout |
| `read_timeout` | duration | `"3s"` | Read operation timeout |
| `write_timeout` | duration | `"3s"` | Write operation timeout |
| `pool_size` | int | `10` | Connection pool size |
| `min_idle_conns` | int | `2` | Minimum idle connections in pool |
| `key_prefix` | string | `"ratelimit:"` | Prefix for all Redis keys |
| `fallback_to_local` | bool | `true` | Fall back to local rate limiting on Redis failure |
| `sync_local_on_miss` | bool | `true` | Sync local state with Redis on cache miss |

## Algorithms

### Distributed Token Bucket

The distributed token bucket algorithm uses Redis hash structures to store:
- `tokens`: Current available tokens (float)
- `last`: Last update timestamp

When a request arrives:
1. Calculate elapsed time since last update
2. Refill tokens based on rate
3. Check if at least 1 token is available
4. Consume token and update state

All operations are performed atomically using Lua scripts.

### Distributed Sliding Window

The distributed sliding window uses Redis sorted sets (ZSET) to track requests:
- Each request is added as a member with timestamp as score
- Old entries outside the window are removed
- Current count is compared against the limit

This provides accurate rate limiting over rolling time windows.

## Redis Key Structure

Keys are prefixed with the configured `key_prefix`:

```
ratelimit:tb:{key}    # Token bucket state (hash)
ratelimit:sw:{key}    # Sliding window requests (sorted set)
```

Where `{key}` is the rate limit key (e.g., API key, IP address, or route).

## Fallback Behavior

When `fallback_to_local: true`, the system will:

1. Attempt to use Redis for rate limiting
2. If Redis is unavailable, fall back to in-memory rate limiting
3. Continue attempting to reconnect to Redis
4. Resume distributed rate limiting when Redis comes back online

This ensures rate limiting continues to work even during Redis outages, though limits may temporarily become per-instance rather than global.

## Performance Considerations

### Latency

Redis operations add network latency:
- Local rate limiting: ~1-5 microseconds
- Redis rate limiting: ~1-5 milliseconds (depends on network)

### Optimization Tips

1. **Use connection pooling**: The Redis client maintains a connection pool
2. **Enable pipelining**: Multiple operations can be batched
3. **Set appropriate timeouts**: Balance between reliability and responsiveness
4. **Use Redis Cluster**: For high availability and scalability
5. **Monitor Redis latency**: Use `redis-cli --latency` to check

### Redis Memory Usage

Memory usage depends on:
- Number of rate limit keys
- Window size and granularity
- Cleanup/expiry settings

Example memory usage:
- Token bucket per key: ~100 bytes
- Sliding window per key: ~100 bytes + ~50 bytes per request in window

With 1M active keys and 10 requests per window: ~1.5GB RAM

## Monitoring

### Redis Metrics to Watch

- `connected_clients`: Number of connected clients
- `used_memory`: Memory usage
- `keyspace_hits` / `keyspace_misses`: Cache efficiency
- `instantaneous_ops_per_sec`: Operation rate

### Application Metrics

The rate limiter tracks:
- Requests allowed/denied
- Redis operation latency
- Fallback events (switching to local mode)

## High Availability

### Redis Sentinel

For high availability, use Redis Sentinel:

```yaml
redis:
  enabled: true
  address: "sentinel-host:26379"
  # Sentinel configuration handled by Redis client
```

### Redis Cluster

For horizontal scaling:

```yaml
redis:
  enabled: true
  address: "redis-node1:6379,redis-node2:6379,redis-node3:6379"
```

## Testing

### Local Testing

Start Redis with Docker:

```bash
docker run -d --name redis -p 6379:6379 redis:latest
```

Test the connection:

```bash
redis-cli ping
```

### Load Testing

When load testing distributed rate limiting:

1. Ensure Redis can handle the connection count
2. Monitor Redis CPU and memory
3. Watch for connection pool exhaustion
4. Measure p99 latency impact

## Migration from Local to Distributed

To migrate from local to Redis-backed rate limiting:

1. Deploy Redis instance
2. Update configuration with `enabled: true`
3. Enable `fallback_to_local: true` for safety
4. Monitor for any issues
5. Once stable, can disable fallback if desired

Note: Rate limit state is not migrated from local to Redis - counters reset during migration.

## Troubleshooting

### Redis Connection Failures

**Symptom**: Logs show "redis connection failed"

**Solutions**:
- Check Redis is running: `redis-cli ping`
- Verify address and port
- Check firewall rules
- Verify password if authentication enabled

### High Latency

**Symptom**: Requests are slower with Redis enabled

**Solutions**:
- Check network latency to Redis: `redis-cli --latency`
- Increase connection pool size
- Enable Redis pipelining
- Consider Redis instance sizing

### Memory Issues

**Symptom**: Redis memory usage growing

**Solutions**:
- Set appropriate key TTLs
- Enable key eviction policies
- Monitor key count growth
- Consider shorter rate limit windows

## Example Use Cases

### Per-User Rate Limiting Across Instances

```yaml
redis:
  enabled: true
  address: "redis:6379"

routes:
  - name: "api-route"
    plugins:
      - name: "rate-limit"
        config:
          algorithm: "token_bucket"
          scope: "consumer"
          requests_per_second: 10
          burst: 20
```

All gateway instances now share the same token bucket per user.

### Global Rate Limiting

```yaml
routes:
  - name: "api-route"
    plugins:
      - name: "rate-limit"
        config:
          algorithm: "sliding_window"
          scope: "global"
          limit: 10000
          window: "1m"
```

Total requests across all instances limited to 10,000 per minute.
