# APICerebrus Troubleshooting Guide

This guide helps diagnose and resolve common issues with APICerebrus.

## Table of Contents

1. [Quick Diagnostics](#quick-diagnostics)
2. [Installation Issues](#installation-issues)
3. [Configuration Issues](#configuration-issues)
4. [Database Issues](#database-issues)
5. [Performance Issues](#performance-issues)
6. [Network Issues](#network-issues)
7. [Security Issues](#security-issues)
8. [Raft Cluster Issues](#raft-cluster-issues)
9. [Common Error Messages](#common-error-messages)
10. [Getting Help](#getting-help)

## Quick Diagnostics

### Health Check Script

```bash
# Run comprehensive health check
./scripts/ops/health-check.sh -v

# Check specific components
./scripts/ops/health-check.sh --only database,gateway
```

### Basic Status Check

```bash
# Check service status
sudo systemctl status apicerberus

# View recent logs
sudo journalctl -u apicerberus -n 100 -f

# Check ports
sudo netstat -tlnp | grep apicerberus

# Check processes
ps aux | grep apicerberus
```

### API Health Endpoints

```bash
# Gateway health
curl http://localhost:8080/health

# Admin health
curl http://localhost:9876/health

# Metrics
curl http://localhost:8080/metrics
```

## Installation Issues

### Binary Not Found

**Symptom:** `command not found: apicerberus`

**Solutions:**
```bash
# Check if binary exists
which apicerberus
ls -la /usr/local/bin/apicerberus

# Add to PATH
export PATH=$PATH:/usr/local/bin

# Or use full path
/usr/local/bin/apicerberus --version
```

### Permission Denied

**Symptom:** `permission denied` when starting

**Solutions:**
```bash
# Fix binary permissions
sudo chmod +x /usr/local/bin/apicerberus

# Fix directory ownership
sudo chown -R apicerberus:apicerberus /var/lib/apicerberus
sudo chown -R apicerberus:apicerberus /var/log/apicerberus

# Check SELinux (RHEL/CentOS)
sudo setenforce 0  # Temporary
sudo chcon -t bin_t /usr/local/bin/apicerberus  # Permanent
```

### Missing Dependencies

**Symptom:** `error while loading shared libraries`

**Solutions:**
```bash
# Install dependencies (Ubuntu/Debian)
sudo apt-get update
sudo apt-get install -y ca-certificates

# Install dependencies (RHEL/CentOS)
sudo yum install -y ca-certificates

# Verify glibc version
ldd --version
```

## Configuration Issues

### Config File Not Found

**Symptom:** `config file not found`

**Solutions:**
```bash
# Check config file exists
ls -la /etc/apicerberus/apicerberus.yaml

# Create from example
sudo cp /usr/share/apicerberus/apicerberus.example.yaml /etc/apicerberus/apicerberus.yaml

# Specify config path explicitly
apicerberus -c /path/to/config.yaml
```

### YAML Syntax Errors

**Symptom:** `yaml: line X: error message`

**Solutions:**
```bash
# Validate YAML syntax
yamllint /etc/apicerberus/apicerberus.yaml

# Check with Python
python3 -c "import yaml; yaml.safe_load(open('/etc/apicerberus/apicerberus.yaml'))"

# Common fixes:
# - Use spaces, not tabs
# - Quote strings with special characters
# - Check indentation (2 spaces)
```

### Invalid Configuration Values

**Symptom:** `invalid config: field validation error`

**Solutions:**
```bash
# Validate configuration
apicerberus -c /etc/apicerberus/apicerberus.yaml --validate

# Check environment variables
echo $ADMIN_API_KEY
echo $SESSION_SECRET

# Verify file permissions
ls -la /etc/apicerberus/
```

## Database Issues

### Database Locked

**Symptom:** `database is locked` or `busy timeout`

**Solutions:**
```bash
# Check for other processes
lsof /var/lib/apicerberus/apicerberus.db
fuser /var/lib/apicerberus/apicerberus.db

# Kill hanging processes
sudo kill -9 <PID>

# Clear WAL files
sudo systemctl stop apicerberus
cd /var/lib/apicerberus
rm -f apicerberus.db-wal apicerberus.db-shm
sudo systemctl start apicerberus

# Increase busy timeout
# In config:
store:
  busy_timeout: "10s"
```

### Database Corruption

**Symptom:** `database disk image is malformed`

**Solutions:**
```bash
# Stop service
sudo systemctl stop apicerberus

# Backup current database
cp /var/lib/apicerberus/apicerberus.db /var/backups/apicerberus-corrupt-$(date +%Y%m%d).db

# Try to repair
sqlite3 /var/lib/apicerberus/apicerberus.db ".recover" | sqlite3 /var/lib/apicerberus/apicerberus-recovered.db

# Verify recovered database
sqlite3 /var/lib/apicerberus/apicerberus-recovered.db "PRAGMA integrity_check;"

# Replace if successful
mv /var/lib/apicerberus/apicerberus-recovered.db /var/lib/apicerberus/apicerberus.db
sudo systemctl start apicerberus
```

### Disk Full

**Symptom:** `database or disk is full`

**Solutions:**
```bash
# Check disk space
df -h

# Find large files
du -sh /var/lib/apicerberus/*
du -sh /var/log/apicerberus/*

# Clean up old logs
./scripts/ops/rotate-logs.sh cleanup -r 7

# Vacuum database
sqlite3 /var/lib/apicerberus/apicerberus.db "VACUUM;"

# Archive old audit logs
./scripts/ops/cleanup-old-data.sh -r 30
```

### Slow Queries

**Symptom:** High database latency

**Solutions:**
```bash
# Analyze database
sqlite3 /var/lib/apicerberus/apicerberus.db "ANALYZE;"

# Check for missing indexes
sqlite3 /var/lib/apicerberus/apicerberus.db ".indexes"

# Enable query logging (temporary)
# Add to config:
logging:
  level: "debug"

# Monitor slow queries
sqlite3 /var/lib/apicerberus/apicerberus.db "PRAGMA compile_options;"
```

## Performance Issues

### High CPU Usage

**Symptom:** CPU usage consistently > 80%

**Diagnosis:**
```bash
# Profile CPU usage
curl http://localhost:9876/debug/pprof/profile > cpu.prof
go tool pprof cpu.prof

# Check goroutines
curl http://localhost:9876/debug/pprof/goroutine?debug=1

# Monitor in real-time
top -p $(pgrep apicerberus)
```

**Solutions:**
```yaml
# Reduce worker threads
gateway:
  max_connections: 5000

# Enable caching
global_plugins:
  - name: "cache"
    config:
      ttl: "5m"

# Rate limiting
routes:
  - plugins:
      - name: "rate-limit"
        config:
          limit: 1000
```

### High Memory Usage

**Symptom:** Memory usage growing continuously

**Diagnosis:**
```bash
# Memory profile
curl http://localhost:9876/debug/pprof/heap > heap.prof
go tool pprof heap.prof

# Check allocations
curl http://localhost:9876/debug/pprof/allocs > allocs.prof
```

**Solutions:**
```bash
# Set memory limits
export GOGC=50  # More aggressive GC

# Reduce buffer sizes
# In config:
audit:
  buffer_size: 5000

# Restart service
sudo systemctl restart apicerberus
```

### Slow Response Times

**Symptom:** High latency (p99 > 1s)

**Diagnosis:**
```bash
# Check upstream latency
curl -w "@curl-format.txt" -o /dev/null -s http://upstream/health

# Database latency
sqlite3 /var/lib/apicerberus/apicerberus.db "PRAGMA busy_timeout;"

# Network latency
mtr -n upstream-host
```

**Solutions:**
```yaml
# Increase timeouts
gateway:
  read_timeout: "60s"
  write_timeout: "60s"

# Enable connection pooling
services:
  - name: "backend"
    connect_timeout: "10s"
    read_timeout: "60s"

# Add caching
global_plugins:
  - name: "cache"
    config:
      ttl: "1m"
```

## Network Issues

### Port Already in Use

**Symptom:** `bind: address already in use`

**Solutions:**
```bash
# Find process using port
sudo lsof -i :8080
sudo netstat -tlnp | grep 8080

# Kill process
sudo kill -9 <PID>

# Or change port in config
gateway:
  http_addr: ":8081"
```

### Connection Refused

**Symptom:** `connection refused` errors

**Solutions:**
```bash
# Check service is running
sudo systemctl status apicerberus

# Check firewall
sudo iptables -L -n | grep 8080
sudo ufw status

# Test locally
curl http://localhost:8080/health

# Check binding
gateway:
  http_addr: "0.0.0.0:8080"  # Bind all interfaces
```

### TLS/SSL Issues

**Symptom:** `TLS handshake error` or certificate warnings

**Solutions:**
```bash
# Check certificate
openssl x509 -in /path/to/cert.pem -text -noout
openssl x509 -in /path/to/cert.pem -dates -noout

# Test SSL connection
openssl s_client -connect localhost:8443 -servername api.example.com

# Renew certificate
# If using Let's Encrypt:
sudo certbot renew

# Check certificate chain
openssl verify -CAfile /path/to/ca.pem /path/to/cert.pem
```

## Security Issues

### Unauthorized Access

**Symptom:** `unauthorized` or `forbidden` errors

**Solutions:**
```bash
# Check API key
curl -H "X-API-Key: your-key" http://localhost:9876/health

# Verify key in database
sqlite3 /var/lib/apicerberus/apicerberus.db \
  "SELECT * FROM api_keys WHERE key_prefix = 'prefix';"

# Check ACLs
sqlite3 /var/lib/apicerberus/apicerberus.db \
  "SELECT * FROM endpoint_permissions WHERE user_id = 'user_id';"
```

### Rate Limiting

**Symptom:** `429 Too Many Requests`

**Solutions:**
```bash
# Check rate limit status
curl -H "X-API-Key: your-key" http://localhost:9876/v1/ratelimit/status

# Increase limits
routes:
  - plugins:
      - name: "rate-limit"
        config:
          limit: 10000
          window: "1m"

# Whitelist IPs
global_plugins:
  - name: "ip-whitelist"
    config:
      whitelist:
        - "10.0.0.0/8"
```

## Raft Cluster Issues

### Node Not Joining

**Symptom:** Node stuck in single-node mode

**Solutions:**
```bash
# Check Raft status
curl http://localhost:9876/v1/raft/status

# Check network connectivity
nc -zv node2 12000

# Verify configuration
# All nodes must have same peers list

# Check logs
sudo journalctl -u apicerberus | grep -i raft

# Force rejoin
# 1. Stop node
# 2. Clear Raft data: rm -rf /var/lib/apicerberus/raft/*
# 3. Start node
```

### Split Brain

**Symptom:** Cluster reports different leaders

**Solutions:**
```bash
# Check leader on each node
for node in node1 node2 node3; do
  echo "$node:"
  curl -s http://$node:9876/v1/raft/status | jq '.leader'
done

# If split brain:
# 1. Stop all nodes
# 2. Clear Raft data on minority nodes
# 3. Start majority nodes first
# 4. Start minority nodes
```

### Slow Consensus

**Symptom:** Operations timing out

**Solutions:**
```bash
# Check network latency
ping node2
mtr node2

# Check disk I/O
iostat -x 1

# Increase Raft timeouts
raft:
  heartbeat_timeout: "500ms"
  election_timeout: "1s"
  commit_timeout: "100ms"
```

## Common Error Messages

### "no such table"

**Cause:** Database migrations not applied

**Fix:**
```bash
# Run migrations
./scripts/migrations/migrate-up.sh -d /var/lib/apicerberus/apicerberus.db

# Or restart service (auto-migrates)
sudo systemctl restart apicerberus
```

### "constraint failed"

**Cause:** Duplicate key or invalid foreign key

**Fix:**
```bash
# Check existing data
sqlite3 /var/lib/apicerberus/apicerberus.db \
  "SELECT * FROM table WHERE id = 'value';"

# Fix or remove conflicting data
```

### "context deadline exceeded"

**Cause:** Operation timeout

**Fix:**
```yaml
# Increase timeouts
gateway:
  read_timeout: "60s"
  write_timeout: "60s"
```

### "too many open files"

**Cause:** File descriptor limit reached

**Fix:**
```bash
# Increase limits
ulimit -n 65535

# Permanent fix in /etc/security/limits.conf:
apicerberus soft nofile 65535
apicerberus hard nofile 65535
```

## Getting Help

### Collect Diagnostic Information

```bash
# Create diagnostic bundle
#!/bin/bash
OUTPUT="apicerberus-diagnostics-$(date +%Y%m%d-%H%M%S).tar.gz"

# Collect logs
sudo journalctl -u apicerberus --since "24 hours ago" > /tmp/apicerberus.log

# Collect config (sanitized)
cp /etc/apicerberus/apicerberus.yaml /tmp/apicerberus-config.yaml
sed -i 's/api_key:.*/api_key: "***REDACTED***"/g' /tmp/apicerberus-config.yaml

# Collect metrics
curl -s http://localhost:8080/metrics > /tmp/metrics.txt

# Collect system info
uname -a > /tmp/system-info.txt
df -h >> /tmp/system-info.txt
free -h >> /tmp/system-info.txt

# Create archive
tar -czf "$OUTPUT" -C /tmp apicerberus.log apicerberus-config.yaml metrics.txt system-info.txt

echo "Diagnostic bundle created: $OUTPUT"
```

### Debug Mode

```bash
# Enable debug logging
# In config:
logging:
  level: "debug"

# Or via environment
export APICERBERUS_LOG_LEVEL=debug

# Enable pprof
curl http://localhost:9876/debug/pprof/
```

### Support Channels

1. **GitHub Issues:** https://github.com/APICerberus/APICerebrus/issues
2. **Documentation:** https://docs.apicerberus.io
3. **Community Discord:** [Link]
4. **Commercial Support:** support@apicerberus.io

### Information to Include

When reporting issues:

1. APICerebrus version
2. Operating system and version
3. Configuration (sanitized)
4. Error messages
5. Steps to reproduce
6. Diagnostic bundle
7. Recent changes

## Emergency Procedures

### Complete Outage

1. Check infrastructure (network, disk, memory)
2. Restart service: `sudo systemctl restart apicerberus`
3. If still down, restore from backup
4. Contact support if needed

### Data Loss

1. Stop service immediately
2. Assess extent of loss
3. Restore from most recent backup
4. Replay audit logs if needed

### Security Breach

1. Revoke all API keys
2. Rotate all secrets
3. Review audit logs
4. Notify affected users
5. Implement additional security measures
