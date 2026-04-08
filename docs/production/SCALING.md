# APICerebrus Horizontal Scaling Guide

This guide covers strategies for horizontally scaling APICerebrus to handle high traffic loads.

## Table of Contents

1. [Scaling Concepts](#scaling-concepts)
2. [Vertical vs Horizontal Scaling](#vertical-vs-horizontal-scaling)
3. [Raft Clustering](#raft-clustering)
4. [Load Balancing Strategies](#load-balancing-strategies)
5. [Database Scaling](#database-scaling)
6. [Caching Strategies](#caching-strategies)
7. [Rate Limiting at Scale](#rate-limiting-at-scale)
8. [Multi-Region Deployment](#multi-region-deployment)
9. [Performance Tuning](#performance-tuning)
10. [Capacity Planning](#capacity-planning)

## Scaling Concepts

APICerebrus is designed to scale horizontally while maintaining consistency through Raft consensus.

### Key Metrics for Scaling

| Metric | Threshold | Action |
|--------|-----------|--------|
| CPU > 70% | Sustained 5 min | Scale up |
| Memory > 80% | Sustained 5 min | Scale up |
| Latency p99 > 1s | Sustained 2 min | Investigate |
| Error rate > 1% | Immediate | Scale up + investigate |
| Queue depth > 1000 | Sustained 1 min | Scale up |

## Vertical vs Horizontal Scaling

### Vertical Scaling (Scale Up)

**Pros:**
- Simple to implement
- No code changes
- Good for CPU-bound workloads

**Cons:**
- Hardware limits
- Single point of failure
- Downtime required

**When to use:**
- Initial deployment
- Database-heavy workloads
- Before horizontal scaling

### Horizontal Scaling (Scale Out)

**Pros:**
- Near-unlimited scaling
- High availability
- No downtime

**Cons:**
- More complex
- Requires load balancer
- Data consistency challenges

**When to use:**
- Production workloads
- High availability requirements
- Traffic > 10,000 req/s

## Raft Clustering

APICerebrus uses Raft for distributed consensus. Recommended cluster sizes:

| Nodes | Fault Tolerance | Use Case |
|-------|-----------------|----------|
| 1 | None | Development |
| 3 | 1 node | Small production |
| 5 | 2 nodes | Medium production |
| 7 | 3 nodes | Large production |

### Bootstrap a Cluster

**Node 1 (Bootstrap node):**

```yaml
raft:
  enabled: true
  node_id: "node1"
  bind_addr: ":12000"
  data_dir: "/var/lib/apicerberus/raft"
  bootstrap: true
  peers:
    - "node1:12000"
    - "node2:12000"
    - "node3:12000"
```

**Nodes 2 & 3:**

```yaml
raft:
  enabled: true
  node_id: "node2"  # or "node3"
  bind_addr: ":12000"
  data_dir: "/var/lib/apicerberus/raft"
  bootstrap: false
  peers:
    - "node1:12000"
    - "node2:12000"
    - "node3:12000"
```

### Adding Nodes to Running Cluster

```bash
# On existing node, add new peer
curl -X POST http://admin:9876/v1/raft/peers \
  -H "X-API-Key: $ADMIN_API_KEY" \
  -d '{"id": "node4", "address": "node4:12000"}'

# Start new node with bootstrap: false
```

### Removing Nodes

```bash
# Gracefully remove node
curl -X DELETE http://admin:9876/v1/raft/peers/node4 \
  -H "X-API-Key: $ADMIN_API_KEY"
```

## Load Balancing Strategies

### Layer 4 (Transport) Load Balancing

**HAProxy:**

```haproxy
global
    maxconn 4096

defaults
    mode tcp
    timeout connect 5s
    timeout client 30s
    timeout server 30s

backend apicerberus
    balance roundrobin
    option tcp-check
    server node1 node1:8080 check
    server node2 node2:8080 check
    server node3 node3:8080 check
```

**Nginx Stream:**

```nginx
stream {
    upstream apicerberus {
        least_conn;
        server node1:8080;
        server node2:8080;
        server node3:8080;
    }

    server {
        listen 80;
        proxy_pass apicerberus;
        proxy_timeout 3s;
        proxy_connect_timeout 1s;
    }
}
```

### Layer 7 (Application) Load Balancing

**Nginx (with health checks):**

```nginx
upstream apicerberus {
    least_conn;
    server node1:8080 weight=5 max_fails=3 fail_timeout=30s;
    server node2:8080 weight=5 max_fails=3 fail_timeout=30s;
    server node3:8080 weight=5 max_fails=3 fail_timeout=30s backup;
    keepalive 32;
}

server {
    location / {
        proxy_pass http://apicerberus;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        
        # Health check
        health_check interval=5s fails=3 passes=2;
    }
}
```

### DNS Load Balancing

```bind
; Round-robin DNS
api.example.com.    IN  A   10.0.1.10
api.example.com.    IN  A   10.0.1.11
api.example.com.    IN  A   10.0.1.12
```

### Cloud Load Balancers

**AWS ALB:**

```bash
aws elbv2 create-load-balancer \
    --name apicerberus-alb \
    --subnets subnet-123456 subnet-789012 \
    --security-groups sg-123456

aws elbv2 create-target-group \
    --name apicerberus-tg \
    --protocol HTTP \
    --port 8080 \
    --vpc-id vpc-123456 \
    --health-check-path /health
```

**GCP Load Balancer:**

```bash
gcloud compute backend-services create apicerberus-backend \
    --protocol=HTTP \
    --health-checks=apicerberus-health-check \
    --global

gcloud compute backend-services add-backend apicerberus-backend \
    --instance-group=apicerberus-ig \
    --global
```

## Database Scaling

### SQLite Limitations

SQLite works well for:
- Single-node deployments
- Read-heavy workloads
- < 1M requests/day

For higher scale, consider:
- Read replicas
- Connection pooling
- WAL mode optimization

### Read Replicas

APICerebrus supports read replicas for analytics queries:

```yaml
store:
  path: "/var/lib/apicerberus/apicerberus.db"
  read_replicas:
    - "/var/lib/apicerberus/replica1.db"
    - "/var/lib/apicerberus/replica2.db"
```

### Database Sharding (Future)

For very large deployments, consider:
- User-based sharding
- Geographic sharding
- Time-based partitioning

## Caching Strategies

### Redis Cluster

```yaml
redis:
  enabled: true
  cluster_addresses:
    - "redis-node1:6379"
    - "redis-node2:6379"
    - "redis-node3:6379"
  password: "${REDIS_PASSWORD}"
  pool_size: 50
```

### Response Caching

```yaml
global_plugins:
  - name: "cache"
    config:
      type: "redis"
      ttl: "5m"
      max_size: 10000
      key_prefix: "response:"
      cacheable_methods:
        - "GET"
        - "HEAD"
      cacheable_status_codes:
        - 200
        - 301
        - 404
```

### Cache Warming

```bash
# Pre-populate cache
for endpoint in /api/v1/users /api/v1/products; do
    curl -s "http://localhost:8080$endpoint" > /dev/null
done
```

## Rate Limiting at Scale

### Distributed Rate Limiting

```yaml
redis:
  enabled: true
  address: "redis-cluster:6379"
  fallback_to_local: true
  sync_local_on_miss: true

routes:
  - name: "api-route"
    plugins:
      - name: "rate-limit"
        config:
          algorithm: "token_bucket"
          scope: "consumer"
          limit: 10000
          window: "1m"
          redis_sync_interval: "10s"
```

### Rate Limiting Strategies

| Algorithm | Best For | Pros | Cons |
|-----------|----------|------|------|
| Token Bucket | Burst traffic | Smooth, allows bursts | Complex |
| Fixed Window | Simple use cases | Easy to understand | Thundering herd |
| Sliding Window | Accurate limiting | Precise | More memory |
| Leaky Bucket | Constant rate | Predictable | No bursts |

## Multi-Region Deployment

### Architecture

```
                    Global Load Balancer
                           |
        -------------------------------------
        |                |                  |
    US-East          EU-West           APAC
        |                |                  |
    [Node 1]         [Node 4]          [Node 7]
    [Node 2]         [Node 5]          [Node 8]
    [Node 3]         [Node 6]          [Node 9]
        |                |                  |
    [Redis]          [Redis]           [Redis]
        \                |                  /
         ---------------[Cluster]-----------
```

### Geo-Routing

```yaml
routes:
  - name: "users-api"
    plugins:
      - name: "geo-routing"
        config:
          prefer_local: true
          fallback_regions:
            - "us-west"
            - "eu-west"
          health_check_interval: "30s"
```

### Data Replication

For cross-region consistency:

1. **Raft across regions** (higher latency)
2. **Async replication** (eventual consistency)
3. **Regional SQLite** with sync jobs

## Performance Tuning

### Kernel Tuning

```bash
# /etc/sysctl.conf

# Increase file descriptors
fs.file-max = 2097152

# TCP optimization
net.core.somaxconn = 65535
net.ipv4.tcp_max_syn_backlog = 65535
net.ipv4.ip_local_port_range = 1024 65535

# Connection tracking
net.netfilter.nf_conntrack_max = 1000000

# Apply
sysctl -p
```

### APICerebrus Tuning

```yaml
gateway:
  # Connection limits
  max_connections: 10000
  max_requests_per_connection: 1000
  
  # Timeouts
  read_timeout: "30s"
  write_timeout: "30s"
  idle_timeout: "120s"

  # Buffer sizes
  read_buffer_size: 8192
  write_buffer_size: 8192

store:
  # SQLite optimization
  busy_timeout: "10s"
  journal_mode: "WAL"
  cache_size: 10000
  mmap_size: 30000000000
```

### Worker Tuning

```bash
# Set GOMAXPROCS to match CPU cores
export GOMAXPROCS=$(nproc)

# Set GOGC for memory tuning
export GOGC=100
```

## Capacity Planning

### Single Node Capacity

| Resource | Light Load | Medium Load | Heavy Load |
|----------|------------|-------------|------------|
| CPU | 2 cores | 4 cores | 8+ cores |
| RAM | 2 GB | 4 GB | 8+ GB |
| Requests/s | 1,000 | 5,000 | 10,000+ |
| Concurrent | 100 | 500 | 2,000+ |

### Cluster Capacity

| Nodes | Requests/s | Concurrent | Storage |
|-------|------------|------------|---------|
| 3 | 15,000 | 3,000 | 30 GB |
| 5 | 25,000 | 5,000 | 50 GB |
| 7 | 35,000 | 7,000 | 70 GB |
| 10 | 50,000 | 10,000 | 100 GB |

### Scaling Triggers

```yaml
# Kubernetes HPA example
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: apicerberus
  minReplicas: 3
  maxReplicas: 20
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          averageUtilization: 70
          type: Utilization
    - type: Pods
      pods:
        metric:
          name: http_requests_per_second
        target:
          averageValue: 1000
          type: AverageValue
```

## Testing Scalability

### Load Testing

```bash
# Using k6
k6 run --vus 1000 --duration 5m script.js

# Using wrk
wrk -t12 -c400 -d30s http://api.example.com/health

# Using ab
ab -n 100000 -c 1000 http://api.example.com/health
```

### Chaos Testing

```bash
# Kill random node
kubectl delete pod -l app=apicerberus --grace-period=0

# Network partition
iptables -A INPUT -s node2 -j DROP

# High latency
tc qdisc add dev eth0 root netem delay 100ms
```

## Monitoring Scale

### Key Metrics

```promql
# Request rate
sum(rate(http_requests_total[5m]))

# Error rate
sum(rate(http_requests_total{status=~"5.."}[5m]))

# P99 latency
histogram_quantile(0.99, sum(rate(http_request_duration_seconds_bucket[5m])) by (le))

# Active connections
apicerberus_active_connections

# Raft cluster health
apicerberus_raft_cluster_health
```

### Alerting Rules

```yaml
groups:
  - name: scaling
    rules:
      - alert: HighRequestRate
        expr: sum(rate(http_requests_total[5m])) > 8000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High request rate - consider scaling"
          
      - alert: RaftClusterDegraded
        expr: apicerberus_raft_cluster_health < 1
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Raft cluster is degraded"
```

## Best Practices

1. **Start small**: Begin with 3 nodes, scale as needed
2. **Monitor everything**: Metrics drive scaling decisions
3. **Test failover**: Regularly verify HA works
4. **Plan for growth**: Design for 10x current load
5. **Document topology**: Keep architecture diagrams updated
6. **Automate scaling**: Use auto-scaling where possible
7. **Cache aggressively**: Reduce database load
8. **Use connection pooling**: Reduce connection overhead
9. **Optimize SQLite**: WAL mode, proper pragmas
10. **Test thoroughly**: Load test before production

## Common Pitfalls

1. **Even cluster size**: Always use odd number of nodes
2. **Split brain**: Ensure proper network partitions
3. **Hot spots**: Distribute load evenly
4. **Resource contention**: Monitor CPU/memory/disk
5. **Network latency**: Minimize inter-node latency
6. **Data growth**: Plan for database size increase
7. **Backup at scale**: Distributed backup strategies
8. **Certificate management**: Automate cert rotation

## Support

For scaling assistance:
- Review architecture with team
- Load test in staging
- Monitor production closely
- Have rollback plan ready
