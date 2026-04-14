# APICerebrus Operations Runbook

## Quick Reference

| Component | Command | Check |
|-----------|---------|-------|
| Health | `curl http://localhost:8080/health` | Should return 200 |
| Metrics | `curl http://localhost:8080/metrics` | Prometheus format |
| Version | `apicerberus version` | Show version |

---

## Alerts and Response Procedures

### 🔴 CRITICAL: APICerebrusInstanceDown

**Symptoms:**
- Health check endpoint returns non-200
- Metrics endpoint unreachable
- Service not responding to requests

**Diagnosis:**
```bash
# Check if process is running
ps aux | grep apicerberus

# Check logs
journalctl -u apicerberus -f
docker logs apicerberus-gateway

# Check resource usage
free -h
df -h
top -p $(pgrep apicerberus)
```

**Resolution:**
1. Check disk space: `df -h`
2. Check memory: `free -h`
3. Restart service: `systemctl restart apicerberus`
4. Check for panic in logs
5. If persistent, restore from backup

---

### 🔴 CRITICAL: APICerebrusRaftNoLeader

**Symptoms:**
- Cluster has no elected leader
- Configuration changes not propagating
- Split-brain scenario

**Diagnosis:**
```bash
# Check Raft status on each node
apicerberus raft status

# Check logs for election events
grep "raft" /var/log/apicerberus/*.log
```

**Resolution:**
1. Ensure majority of nodes are running (N/2 + 1)
2. Check network connectivity between nodes
3. Restart follower nodes first, then leader
4. If split-brain, stop all nodes and restart with clean state

---

### 🔴 CRITICAL: APICerebrusCertificateExpired

**Symptoms:**
- HTTPS requests failing
- Certificate validation errors
- Clients cannot connect

**Diagnosis:**
```bash
# Check certificate expiry
openssl s_client -connect localhost:8443 -servername api.example.com < /dev/null 2>/dev/null | openssl x509 -noout -dates

# Check ACME logs
grep "acme" /var/log/apicerberus/*.log
```

**Resolution:**
1. Force certificate renewal:
   ```bash
   apicerberus cert renew --force
   ```
2. Check ACME rate limits (Let's Encrypt: 5 certs/domain/week)
3. Verify DNS records point to server
4. Check firewall allows port 80 for ACME challenge

---

### 🟡 WARNING: APICerebrusHighErrorRate

**Symptoms:**
- Error rate > 5% for 2 minutes
- HTTP 5xx responses

**Diagnosis:**
```bash
# Check error logs
grep "error" /var/log/apicerberus/*.log | tail -50

# Check upstream health
curl http://localhost:8080/admin/api/v1/upstreams

# Check recent requests
curl http://localhost:8080/admin/api/v1/audit-logs?limit=100
```

**Resolution:**
1. Identify failing upstreams
2. Check upstream service health
3. Review recent configuration changes
4. Check for circuit breaker trips
5. Scale upstream services if needed

---

### 🟡 WARNING: APICerebrusHighLatency

**Symptoms:**
- P95 latency > 1 second
- Slow client responses

**Diagnosis:**
```bash
# Check current latency metrics
curl http://localhost:8080/metrics | grep request_duration

# Check database performance
sqlite3 /data/apicerberus.db "PRAGMA integrity_check;"

# Check upstream response times
curl http://localhost:8080/admin/api/v1/upstreams
```

**Resolution:**
1. Check database query performance
2. Verify upstream service capacity
3. Review rate limiting settings
4. Check for resource exhaustion
5. Scale horizontally if needed

---

### 🟡 WARNING: APICerebrusHighAuthFailureRate

**Symptoms:**
- > 10 auth failures/5min
- Possible brute force attack

**Diagnosis:**
```bash
# Check audit logs for failed auth
grep "failed.*auth" /var/log/apicerberus/audit.log | tail -20

# Check IP patterns
awk '/failed.*auth/ {print $NF}' /var/log/apicerberus/audit.log | sort | uniq -c | sort -rn | head -10
```

**Resolution:**
1. Block suspicious IPs at firewall level
2. Enable stricter rate limiting
3. Rotate API keys if compromise suspected
4. Review audit logs for patterns

---

## Backup and Recovery

### Automated Backup

```bash
# Run backup script
./scripts/backup.sh /backups

# Schedule with cron (daily at 2 AM)
0 2 * * * /opt/apicerberus/scripts/backup.sh /backups
```

### Manual Backup

```bash
# Backup while running (SQLite supports online backup)
sqlite3 /data/apicerberus.db ".backup '/backups/apicerberus.db.backup'"

# Backup certificates
tar -czf /backups/acme-$(date +%Y%m%d).tar.gz /data/acme
```

### Restore Procedure

```bash
# Stop service
systemctl stop apicerberus

# Restore from backup
./scripts/restore.sh /backups/apicerberus_backup_20240331_120000.tar.gz /data

# Start service
systemctl start apicerberus

# Verify health
curl http://localhost:8080/health
```

---

## Maintenance Procedures

### Rolling Update (Zero Downtime)

```bash
# Update followers first
for node in node2 node3; do
  ssh $node "systemctl stop apicerberus && systemctl start apicerberus"
  sleep 5
done

# Update leader last (will trigger election)
ssh node1 "systemctl stop apicerberus && systemctl start apicerberus"
```

### Certificate Renewal

```bash
# Automatic (via ACME)
# Certificates auto-renew 30 days before expiry

# Manual renewal
apicerberus cert renew --domain api.example.com

# Verify renewal
openssl s_client -connect localhost:8443 < /dev/null 2>/dev/null | openssl x509 -noout -text
```

### Database Maintenance

```bash
# Check integrity
sqlite3 /data/apicerberus.db "PRAGMA integrity_check;"

# Vacuum (reclaim space)
sqlite3 /data/apicerberus.db "VACUUM;"

# Analyze for query optimization
sqlite3 /data/apicerberus.db "ANALYZE;"

# Archive old audit logs
./scripts/archive-audit-logs.sh --older-than 90d
```

---

## Troubleshooting

### Common Issues

#### Port Already in Use
```bash
# Find process using port
lsof -i :8080
netstat -tlnp | grep 8080

# Kill process or change port
systemctl stop conflicting-service
```

#### Permission Denied
```bash
# Fix permissions
chown -R apicerberus:apicerberus /data/apicerberus
chmod 600 /data/apicerberus/apicerberus.db
chmod 700 /data/apicerberus/acme
```

#### Out of Memory
```bash
# Check memory usage
smem -P apicerberus

# Limit memory with systemd
systemctl edit apicerberus
# Add:
# [Service]
# MemoryMax=2G
```

#### Raft Join Failure
```bash
# Reset Raft state (WARNING: Data loss if not follower)
rm -rf /data/raft/*

# Rejoin cluster
apicerberus cluster join --peers "node1:12000,node2:12000"
```

---

## Circuit Breaker Tuning Guide

### Default Configuration

```yaml
plugins:
  - name: circuit-breaker
    enabled: true
    config:
      error_threshold: 0.5       # Error rate (0.0–1.0) that trips the breaker
      volume_threshold: 20       # Min requests in window before evaluating
      sleep_window: "10s"        # Duration breaker stays open before probing
      half_open_requests: 1      # Trial requests allowed in half-open state
      window: "30s"              # Sliding window for error rate calculation
```

### How It Works

The circuit breaker uses a **three-state model**:

1. **Closed** (normal) — Requests flow through. A sliding time window tracks
   success/failure events. When `volume_threshold` requests accumulate and the
   error rate reaches `error_threshold`, the breaker trips to Open.

2. **Open** (blocking) — All requests are rejected immediately with HTTP 503.
   After `sleep_window` elapses, the breaker transitions to Half-Open on the
   next request.

3. **Half-Open** (probing) — Allows up to `half_open_requests` trial requests.
   If all succeed, the breaker returns to Closed. If any fails, it immediately
   re-trips to Open for another `sleep_window`.

### Recommended Thresholds by Traffic Profile

| Profile | error_threshold | volume_threshold | sleep_window | half_open_requests | window |
|---------|----------------|-----------------|--------------|--------------------|--------|
| **High traffic** (>1K req/s) | 0.3 | 50 | 5s | 3 | 10s |
| **Medium traffic** (100–1K req/s) | 0.5 | 20 | 10s | 1 | 30s |
| **Low traffic** (<100 req/s) | 0.5 | 10 | 30s | 1 | 60s |
| **Critical upstream** (payment, auth) | 0.2 | 10 | 5s | 5 | 15s |
| **Batch/async upstream** | 0.8 | 50 | 30s | 1 | 60s |

### Tuning Guidelines

- **`error_threshold`**: Lower values make the breaker more sensitive. For
  critical services, use 0.2 (trip at 20% error rate). For resilient services
  that self-recover, 0.5–0.8 avoids unnecessary tripping.

- **`volume_threshold`**: Prevents the breaker from tripping on low sample sizes.
  Increase for high-traffic routes to avoid false positives from transient
  errors. Decrease for low-traffic routes so the breaker activates sooner.

- **`sleep_window`**: Controls how long the upstream is given to recover. Shorter
  windows mean faster recovery attempts but more rejected requests if the
  upstream is still down. Longer windows give more recovery time but extend the
  outage window.

- **`half_open_requests`**: Higher values give more confidence in recovery but
  risk more failed requests if the upstream is not ready. For critical services,
  use 3–5 to confirm recovery before fully re-opening.

- **`window`**: The sliding window for error rate calculation. Shorter windows
  react faster to spikes but may trip on brief transient errors. Longer windows
  smooth out spikes but react slower.

### Monitoring

Monitor these metrics to tune circuit breaker settings:

```
# Circuit breaker state transitions
apicerberus_circuit_breaker_state{route="...", state="open"}

# Rejected requests
apicerberus_circuit_breaker_rejected_total{route="..."}

# Error rate within window
apicerberus_circuit_breaker_error_rate{route="..."}
```

---

## Emergency Contacts

| Role | Contact | Escalation |
|------|---------|------------|
| On-call Engineer | oncall@example.com | +1-555-0100 |
| Security Team | security@example.com | +1-555-0200 |
| Database Admin | dba@example.com | +1-555-0300 |

---

## Change Log

| Date | Version | Changes |
|------|---------|---------|
| 2026-03-31 | 1.0.0 | Initial runbook |

---

## References

- [API Documentation](./docs/api/)
- [Architecture Guide](./ARCHITECTURE.md)
- [Security Audit](./docs/SECURITY_AUDIT.md)
