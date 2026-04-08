# APICerebrus Configuration Examples

This directory contains example configurations for various deployment scenarios.

## Available Configurations

### Minimal Production (`config-minimal.yaml`)

Single-node deployment suitable for small to medium workloads.

**Features:**
- Single upstream service
- Basic rate limiting
- Local SQLite database
- Automatic HTTPS via Let's Encrypt
- Essential audit logging

**Use when:**
- Getting started with APICerebrus
- Small deployments (< 1000 req/s)
- Single region, single node
- Development/staging environments

**Quick start:**
```bash
# Set required environment variables
export ADMIN_API_KEY="your-secure-admin-key"
export SESSION_SECRET="your-secure-session-secret"

# Start APICerebrus
./apicerberus -c config-minimal.yaml
```

---

### High Availability (`config-ha.yaml`)

Multi-node deployment with clustering for production workloads.

**Features:**
- Raft-based clustering
- Redis for distributed rate limiting
- Multiple upstream services with health checks
- OpenTelemetry tracing
- Advanced audit logging with S3 archiving
- Multiple consumers with tiered rate limits

**Use when:**
- Production workloads
- High availability requirements
- Multiple services to proxy
- Need for distributed rate limiting
- Multi-node deployments

**Prerequisites:**
- Redis cluster for rate limiting
- Raft cluster (3+ nodes recommended)
- Jaeger for tracing (optional)

**Quick start:**
```bash
# Set environment variables
export ADMIN_API_KEY="your-secure-admin-key"
export SESSION_SECRET="your-secure-session-secret"
export REDIS_PASSWORD="your-redis-password"
export RAFT_NODE_ID="node1"
export VERSION="1.0.0"

# Start on each node
./apicerberus -c config-ha.yaml
```

---

### Multi-Region (`config-multi-region.yaml`)

Geographically distributed deployment for global scale.

**Features:**
- Region-aware routing
- Cross-region Raft clustering
- Redis Cluster for global state
- S3-compatible audit archiving
- Geo-routing plugin
- Regional upstream selection

**Use when:**
- Global user base
- Multi-region deployment
- Low-latency requirements worldwide
- Disaster recovery needs

**Prerequisites:**
- Redis Cluster across regions
- Raft nodes in each region
- S3-compatible storage
- Regional backend services

**Quick start:**
```bash
# Set environment variables
export REGION="us-east"
export AZ="us-east-1a"
export REDIS_CLUSTER_ENDPOINT="redis-cluster.example.com:6379"
export OTEL_COLLECTOR_ENDPOINT="http://otel-collector:4318"
export AUDIT_BUCKET="apicerberus-audit-logs"

# Deploy in each region
./apicerberus -c config-multi-region.yaml
```

---

## Environment Variables

All configurations use environment variables for sensitive data:

| Variable | Description | Required |
|----------|-------------|----------|
| `ADMIN_API_KEY` | Admin API authentication key | Yes |
| `SESSION_SECRET` | Portal session encryption key | Yes |
| `REDIS_PASSWORD` | Redis authentication password | For HA/Multi-region |
| `REDIS_CLUSTER_ENDPOINT` | Redis cluster endpoint | For Multi-region |
| `RAFT_NODE_ID` | Unique node identifier | For HA/Multi-region |
| `VERSION` | Application version tag | Optional |
| `REGION` | Deployment region | For Multi-region |
| `AZ` | Availability zone | For Multi-region |
| `OTEL_COLLECTOR_ENDPOINT` | OpenTelemetry endpoint | Optional |
| `AUDIT_BUCKET` | S3 bucket for audit logs | For Multi-region |

---

## Kubernetes Deployment

The `kubernetes-deployment.yaml` file provides a complete Kubernetes setup.

### Features
- ConfigMap for configuration
- Secret management
- Persistent storage for SQLite
- Horizontal Pod Autoscaler
- Ingress with TLS
- Network policies
- Pod disruption budget

### Deployment

```bash
# Create namespace and deploy
kubectl apply -f kubernetes-deployment.yaml

# Update secrets
kubectl create secret generic apicerberus-secrets \
  --from-literal=ADMIN_API_KEY="your-key" \
  --from-literal=SESSION_SECRET="your-secret" \
  -n apicerberus --dry-run=client -o yaml | kubectl apply -f -

# Check deployment
kubectl get pods -n apicerberus
kubectl logs -f deployment/apicerberus -n apicerberus
```

### Scaling

```bash
# Manual scaling
kubectl scale deployment apicerberus --replicas=3 -n apicerberus

# View HPA status
kubectl get hpa -n apicerberus
```

---

## Configuration Tips

### Security

1. **Never commit secrets** - Use environment variables
2. **Use strong keys** - Generate with: `openssl rand -base64 32`
3. **Enable HTTPS** - Always use TLS in production
4. **Restrict admin access** - Use IP whitelisting
5. **Rotate secrets regularly** - Use `rotate-secrets.sh`

### Performance

1. **Enable WAL mode** - Better concurrency for SQLite
2. **Use connection pooling** - Configure Redis pool size
3. **Enable compression** - For audit logs and responses
4. **Tune rate limits** - Based on your capacity
5. **Monitor metrics** - Use the monitoring stack

### Reliability

1. **Health checks** - Configure for all upstreams
2. **Circuit breakers** - Prevent cascade failures
3. **Retry policies** - Handle transient errors
4. **Backup regularly** - Use backup scripts
5. **Test failover** - Verify HA setup works

---

## Customization Guide

### Adding a New Route

```yaml
routes:
  - name: "my-new-route"
    service: "my-service"
    hosts:
      - "api.example.com"
    paths:
      - "/api/v1/new-endpoint"
    methods:
      - "GET"
      - "POST"
    plugins:
      - name: "rate-limit"
        config:
          limit: 100
          window: "1m"
```

### Adding Rate Limiting

```yaml
plugins:
  - name: "rate-limit"
    config:
      algorithm: "token_bucket"  # or: fixed_window, sliding_window
      scope: "consumer"          # or: ip, route, global
      limit: 1000
      window: "1m"
      burst: 150  # for token_bucket
```

### Enabling Authentication

```yaml
global_plugins:
  - name: "auth-apikey"
    config:
      key_names:
        - "X-API-Key"
      exclude_paths:
        - "/health"
        - "/public/*"
```

---

## Troubleshooting

### Database Locked
```bash
# Check for other processes
lsof apicerberus.db

# Restart with cleanup
systemctl restart apicerberus
```

### High Memory Usage
```yaml
# Reduce buffer sizes in config
audit:
  buffer_size: 5000  # Reduce from 10000
store:
  busy_timeout: "2s"  # Reduce from 5s
```

### Slow Requests
```yaml
# Add caching plugin
global_plugins:
  - name: "cache"
    config:
      ttl: "5m"
      max_size: 1000
```

---

## Migration Between Configurations

### Minimal to HA

1. Set up Redis cluster
2. Configure Raft nodes
3. Update environment variables
4. Deploy new configuration
5. Verify clustering works
6. Update DNS/load balancer

### Single Region to Multi-Region

1. Deploy to new regions
2. Set up Redis Cluster
3. Configure cross-region Raft
4. Update upstream definitions
5. Configure geo-routing
6. Test failover scenarios

---

## Support

For configuration assistance:
- See main documentation: `/docs`
- Check examples: `/deployments/examples`
- Review tests: `/test`
