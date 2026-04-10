# Troubleshooting Guide

## SQLite Database

### "database is locked" errors

**Symptoms**: Audit log entries dropped, billing deductions failing, `SQLITE_BUSY` in logs.

**Causes**:
- Multiple writers competing for SQLite's single-writer lock
- WAL mode not enabled (check `store.journal_mode` is not `MEMORY`)
- `busy_timeout` too low (default: 1s)

**Fix**:
```yaml
store:
  busy_timeout: "5s"      # Increase from default
  journal_mode: "WAL"     # Never use MEMORY for production
  foreign_keys: true
```

**Verify WAL health**: Check that `-wal` and `-shm` files exist alongside `apicerberus.db`. **Do not delete them** while the gateway is running.

### Database corruption

**Symptoms**: `malformed database schema`, `disk I/O error`.

**Recovery**:
```bash
# Stop gateway first
apicerberus stop

# Run integrity check
sqlite3 data/apicerberus.db "PRAGMA integrity_check;"

# If corrupted, restore from backup
./scripts/restore.sh backups/apicerberus_backup_20260410_120000.tar.gz
```

### Slow queries under load

**Diagnosis**:
```bash
# Enable SQLite trace (add to config temporarily)
# Then check query patterns in logs

# Check audit buffer status via admin API
curl -H "X-Admin-Key: your-key" http://localhost:9876/admin/api/v1/audit/status
```

**Fix**: Increase audit buffer and batch size:
```yaml
audit:
  buffer_size: 10000    # Default: 10000, increase for high throughput
  batch_size: 200       # Default: 100
  flush_interval: "2s"  # Default: 1s
```

---

## Redis Connection

### "Redis connection refused" errors

**Symptoms**: Rate limiting falls back to local mode, distributed token bucket degraded.

**Check**:
```bash
redis-cli -h <redis-host> -p <redis-port> ping
# Should return: PONG
```

**Causes**:
- Redis server down or unreachable
- Network partition between gateway and Redis
- Authentication failure (wrong password)

**Fix**:
```yaml
rate_limit:
  redis_url: "redis://:password@host:6379/0"  # Verify URL
  redis_dial_timeout: "5s"                    # Increase if network is slow
```

**Graceful degradation**: The gateway automatically falls back to local in-memory rate limiting when Redis is unavailable. Distributed limits are temporarily lost but service continues.

**Recovery**: When Redis reconnects, the gateway syncs back to distributed mode automatically. No restart needed.

---

## TLS Certificate Renewal

### ACME/Let's Encrypt certificate failures

**Symptoms**: HTTPS endpoints return cert errors, logs show `acme: error`.

**Common causes**:
1. **DNS not pointing to gateway**: ACME HTTP-01 challenge requires `http://<domain>/.well-known/acme-challenge/` to reach this gateway instance
2. **Rate limiting**: Let's Encrypt limits 50 certs/domain/week. Check at [letsdebug.net](https://letsdebug.net)
3. **Port 80 blocked**: HTTP-01 challenge requires port 80 accessible from the internet

**Fix**:
```bash
# Check ACME directory for cached certs
ls -la data/acme/

# Force renewal via admin API
curl -X POST -H "X-Admin-Key: your-key" \
  http://localhost:9876/admin/api/v1/certs/renew \
  -d '{"domain": "api.example.com"}'
```

### mTLS cluster communication failures

**Symptoms**: Raft nodes can't join cluster, logs show `tls: bad certificate`.

**Diagnosis**:
```bash
# Check cert expiry
openssl x509 -in certs/node.crt -noout -dates

# Verify CA trust
openssl verify -CAfile certs/ca.crt certs/node.crt
```

**Fix**:
- **Auto-generated certs**: Set `cluster.mtls.auto_generate: true` — leader regenerates and distributes via Raft log
- **Manual certs**: Replace `ca_cert_path`, `node_cert_path`, `node_key_path` and restart nodes one at a time

---

## Raft Cluster

### Node fails to join cluster

**Symptoms**: `POST /admin/api/v1/cluster/join` returns error, node stuck in candidate state.

**Checklist**:
1. **Leader is reachable**: `curl http://<leader>:12000/health`
2. **mTLS matches**: If leader has mTLS enabled, joining node must have matching CA
3. **Unique node ID**: Each node must have a unique `cluster.node_id`
4. **Same Raft version**: All nodes must run the same binary version

**Fix**:
```bash
# Check cluster status
apicerberus cluster status

# Check Raft logs on leader
curl -H "X-Admin-Key: your-key" http://localhost:9876/admin/api/v1/cluster/status

# Remove stale node and rejoin
curl -X POST -H "X-Admin-Key: your-key" \
  http://localhost:9876/admin/api/v1/cluster/leave/<stale-node-id>
```

### Cluster loses quorum

**Symptoms**: No config changes accepted, all nodes report "not leader".

**Recovery**:
1. Identify the node with the most recent Raft log
2. On that node, force single-node mode:
   ```yaml
   cluster:
     bootstrap: true    # Force bootstrap as single node
   ```
3. Restart the node
4. Re-add other nodes via `cluster/join`

**Warning**: This can cause data loss if the chosen node is behind. Verify Raft log index on all nodes first.

---

## Plugin Pipeline

### Plugin execution timeout

**Symptoms**: Requests hang, 504 Gateway Timeout, logs show `plugin execution timeout`.

**Diagnosis**:
```bash
# Check plugin status via admin API
curl -H "X-Admin-Key: your-key" http://localhost:9876/admin/api/v1/plugins/status
```

**Fix**:
```yaml
plugins:
  timeout: "30s"       # Increase from default
  enable_parallel: true # Parallel execution within phases
```

### WASM plugin fails to load

**Symptoms**: `wasm: invalid magic number`, `failed to compile wasm module`.

**Causes**:
- WASM file corrupted or not valid WebAssembly
- WASI imports not satisfied
- Memory limits exceeded

**Fix**:
```bash
# Validate WASM file
file plugin.wasm
# Should output: WebAssembly (wasm) binary module version 1

# Check file size (should be < 20MB for safe loading)
ls -lh plugin.wasm
```

---

## Gateway Proxy

### Upstream connection refused

**Symptoms**: 502 Bad Gateway, logs show `dial tcp: connection refused`.

**Check**:
1. Upstream is running and listening on configured address
2. No firewall blocking gateway → upstream communication
3. Health check endpoint responds (if configured)

**Fix**:
```yaml
# Check upstream configuration
upstreams:
  - id: my-upstream
    health_check:
      enabled: true
      interval: "10s"
      path: "/health"
    circuit_breaker:
      enabled: true
      threshold: 5        # Open circuit after 5 consecutive failures
      recovery_timeout: "30s"
```

### High latency on specific routes

**Diagnosis**:
```bash
# Check route-specific latency via audit logs
apicerberus audit search --route my-route --since 2026-04-10

# Check analytics
curl -H "X-Admin-Key: your-key" http://localhost:9876/admin/api/v1/analytics/summary
```

**Common causes**:
- Slow upstream response (check upstream directly)
- Rate limiting throttling (check `X-RateLimit-Remaining` header)
- Plugin pipeline overhead (too many PRE_PROXY/PROXY plugins)

---

## Admin API

### "unauthorized" on all admin requests

**Check**:
- Using `X-Admin-Key` header (not `Authorization: Bearer`)
- Key matches `admin.api_key` in config
- Key is at least 32 characters

**Rate-limited?**: After 5 failed auth attempts in 15 minutes, the admin API blocks further attempts for 30 minutes.

```bash
# Use static auth to get a token
curl -H "X-Admin-Key: your-secret-key" \
  http://localhost:9876/admin/api/v1/auth/token

# Then use the token for subsequent requests
curl -H "Authorization: Bearer <token>" \
  http://localhost:9876/admin/api/v1/status
```

### Config reload fails

**Symptoms**: `POST /admin/api/v1/config/reload` returns 500.

**Check**:
- Config file exists at `APICERBERUS_CONFIG` path
- Valid YAML syntax (run `apicerberus config validate`)
- No port conflicts (port bindings require restart, not reload)

---

## Performance

### High memory usage

**Diagnosis**:
```bash
# Check Go runtime stats via /debug/pprof
curl http://localhost:8080/debug/pprof/heap > heap.pprof
go tool pprof -top heap.pprof
```

**Common causes**:
- Audit buffer too large (`audit.buffer_size`)
- Analytics ring buffer growth under sustained traffic
- WebSocket connections not closing

**Fix**:
```yaml
audit:
  buffer_size: 5000    # Reduce from default 10000

analytics:
  ring_buffer_size: 10000  # Reduce from default
```

### CPU spike on startup

**Normal behavior**: The gateway loads all routes into the radix tree, initializes health checks, and warms up the analytics engine on startup. This is a one-time cost.

**If persistent**: Check for too many routes (10K+) — the radix tree initialization scales with route count.
