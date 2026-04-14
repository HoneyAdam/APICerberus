# Rate Limiting Guide

## Overview

Rate limiting in APICerebrus is **plugin-based and opt-in**. It is not applied to any route unless explicitly configured via `global_plugins` or per-route `plugins`.

### Quick Start

```yaml
# Apply to all routes:
global_plugins:
  - name: rate-limit
    config:
      algorithm: token_bucket
      scope: ip
      requests_per_second: 100
      burst: 150

# Or per-route:
routes:
  - name: my-route
    plugins:
      - name: rate-limit
        config:
          algorithm: sliding_window
          scope: consumer
          limit: 100
          window: 60s
```

---

## Configuration Reference

### Plugin-Level Config

| Key | Default | Description |
|-----|---------|-------------|
| `algorithm` | `token_bucket` | `token_bucket`, `fixed_window`, `sliding_window`, `leaky_bucket` |
| `scope` | `global` | `global`, `consumer`, `ip`, `route`, `composite` |
| `requests_per_second` | `10` | Token bucket / leaky bucket refill rate |
| `burst` | same as rps | Token bucket capacity / leaky bucket queue size |
| `limit` | `10` | Fixed/sliding window max count |
| `window` | `1s` | Fixed/sliding window duration |
| `composite_scopes` | `["consumer","ip","route"]` | Dimensions for composite scope |

### Scope Keys

| Scope | Key Source | Example Key |
|-------|-----------|-------------|
| `global` | Fixed string | `"global"` |
| `consumer` | Authenticated consumer ID | `"consumer:abc123"` |
| `ip` | Client IP address | `"ip:192.168.1.1"` |
| `route` | Route ID | `"route:route-1"` |
| `composite` | Combined from `composite_scopes` | `"consumer:abc:ip:1.2.3.4:route:r1"` |

### Algorithm Comparison

| Algorithm | Best For | Key Params | Smoothness |
|-----------|----------|------------|------------|
| **Token Bucket** | General use, burst-tolerant | `requests_per_second`, `burst` | Allows controlled bursts |
| **Fixed Window** | Simple quota enforcement | `limit`, `window` | Spikes at window boundaries |
| **Sliding Window** | Accurate rate tracking | `limit`, `window` | Smooth, no boundary spikes |
| **Leaky Bucket** | Constant throughput | `requests_per_second`, `burst` | Enforces steady rate |

---

## How Rate Limiting Is Applied

Rate limiting runs in the **PRE_PROXY** phase (after auth, before upstream proxy).
The plugin pipeline processes requests as follows:

```
Request → Route Match → Plugin Pipeline
  PRE_AUTH: IP restriction, bot detection, CORS
  AUTH: API key, JWT
  PRE_PROXY: endpoint-permission (priority 15) → rate-limit (priority 20)
  PROXY: upstream request
```

### Opt-In Model

Rate limiting is **never auto-applied**. Three ways to enable it:

1. **Global plugins** — applies to every route:
   ```yaml
   global_plugins:
     - name: rate-limit
       config: { ... }
   ```

2. **Per-route plugins** — route-specific config overrides global:
   ```yaml
   routes:
     - name: api-route
       plugins:
         - name: rate-limit
           config: { ... }
   ```

3. **Per-permission overrides** — stored in the permissions table, injected
   by the `endpoint-permission` plugin into request metadata. These take
   highest priority over global and route-level config.

### Override Priority (highest to lowest)

1. Permission-level rate limit override (from `permissions.rate_limits` column)
2. Consumer-level rate limit (from user metadata)
3. Route-level plugin config
4. Global plugin config
5. No rate limiting (default)

---

## Response Headers

When rate limiting is active, these headers are always included:

```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 87
X-RateLimit-Reset: 1713091200
```

When rate limited (HTTP 429):

```
HTTP/1.1 429 Too Many Requests
Content-Type: application/json
Retry-After: 30

{"error": "rate_limit_exceeded", "message": "Rate limit exceeded"}
```

---

## Distributed Rate Limiting (Redis)

For multi-instance deployments, Redis-backed rate limiting provides shared state.
See [REDIS_RATE_LIMITING.md](./REDIS_RATE_LIMITING.md) for full configuration.

**Note:** Distributed rate limiting is available for `token_bucket` and
`sliding_window` algorithms. `fixed_window` and `leaky_bucket` always use
in-memory limiters even with Redis configured.

---

## Common Patterns

### Protect Public API from Abuse

```yaml
global_plugins:
  - name: rate-limit
    config:
      algorithm: sliding_window
      scope: ip
      limit: 100
      window: 60s
```

### Per-User Tiered Rate Limits

```yaml
# Default tier
global_plugins:
  - name: rate-limit
    config:
      algorithm: token_bucket
      scope: consumer
      requests_per_second: 10
      burst: 20

# Premium route with higher limits
routes:
  - name: premium-api
    plugins:
      - name: rate-limit
        config:
          algorithm: token_bucket
          scope: consumer
          requests_per_second: 100
          burst: 200
```

### Global Throughput Cap

```yaml
routes:
  - name: heavy-route
    plugins:
      - name: rate-limit
        config:
          algorithm: fixed_window
          scope: global
          limit: 5000
          window: 60s
```

### Multi-Dimensional Rate Limiting

```yaml
plugins:
  - name: rate-limit
    config:
      algorithm: token_bucket
      scope: composite
      composite_scopes: ["consumer", "ip"]
      requests_per_second: 50
      burst: 75
```

This creates a unique rate limit key per consumer+IP combination.
