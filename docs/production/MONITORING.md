# Monitoring Setup Guide

APICerebrus exposes Prometheus-format metrics at `/metrics` on the gateway port (default 8080). This guide covers the available metrics, recommended Prometheus scrape configuration, Grafana dashboards, and alerting.

## Quick Start

```bash
cd deployments/monitoring
cp .env.example .env
# Edit .env with your SMTP/Slack/PagerDuty credentials
docker-compose up -d
```

This starts an 8-container monitoring stack: Prometheus, Grafana, Loki, Promtail, AlertManager, Node Exporter, cAdvisor, and Blackbox Exporter.

## Available Metrics

All metrics use the `gateway_` prefix.

### Counters

| Metric | Labels | Description |
|--------|--------|-------------|
| `gateway_requests_total` | `method`, `status` | Total HTTP requests |
| `gateway_connections_total` | — | Total connections |
| `gateway_backend_requests_total` | `service`, `target` | Backend/upstream requests |
| `gateway_backend_errors_total` | `service`, `target` | Backend errors |
| `gateway_rate_limit_hits_total` | — | Rate limit checks |
| `gateway_rate_limit_exceeds_total` | — | Rate limit rejections |
| `gateway_auth_success_total` | — | Successful authentications |
| `gateway_auth_failures_total` | — | Failed authentications |
| `gateway_federation_requests_total` | — | GraphQL federation requests |
| `gateway_federation_errors_total` | — | GraphQL federation errors |
| `gateway_audit_dropped_total` | — | Audit entries dropped (buffer overflow) |

### Gauges

| Metric | Labels | Description |
|--------|--------|-------------|
| `gateway_active_connections` | — | Currently active connections |

### Histograms

| Metric | Labels | Buckets | Description |
|--------|--------|---------|-------------|
| `gateway_request_duration_seconds` | `method`, `route` | 1ms–10s | Request latency |
| `gateway_request_size_bytes` | `method` | 100B–1MB | Request body size |
| `gateway_response_size_bytes` | `method`, `status` | 100B–10MB | Response body size |
| `gateway_backend_latency_seconds` | `service` | 1ms–2.5s | Backend latency |

## Prometheus Scrape Configuration

```yaml
scrape_configs:
  - job_name: apicerberus-gateway
    metrics_path: /metrics
    static_configs:
      - targets:
          - localhost:8080
    scrape_interval: 15s
    scrape_timeout: 10s

  - job_name: apicerberus-admin
    static_configs:
      - targets:
          - localhost:9876
```

For Docker Swarm service discovery:

```yaml
scrape_configs:
  - job_name: apicerberus-gateway
    metrics_path: /metrics
    dns_sd_configs:
      - names: [tasks.gateway]
        type: A
        port: 8080
```

## Key PromQL Queries

| Purpose | Query |
|---------|-------|
| Request rate (req/s) | `sum(rate(gateway_requests_total[5m]))` |
| Error rate (%) | `sum(rate(gateway_requests_total{status=~"5.."}[5m])) / sum(rate(gateway_requests_total[5m])) * 100` |
| p95 latency | `histogram_quantile(0.95, sum(rate(gateway_request_duration_seconds_bucket[5m])) by (le))` |
| p99 latency by route | `histogram_quantile(0.99, sum(rate(gateway_request_duration_seconds_bucket[5m])) by (le, route))` |
| Auth failure rate | `sum(rate(gateway_auth_failures_total[5m])) / (sum(rate(gateway_auth_success_total[5m])) + sum(rate(gateway_auth_failures_total[5m]))) * 100` |
| Rate limit rejections | `sum(rate(gateway_rate_limit_exceeds_total[5m]))` |
| Backend error rate by service | `sum(rate(gateway_backend_errors_total[5m])) by (service) / sum(rate(gateway_backend_requests_total[5m])) by (service) * 100` |
| Active connections | `gateway_active_connections` |
| Audit entries dropped | `increase(gateway_audit_dropped_total[5m])` |

## Alerting Rules

Alert rules are in `deployments/monitoring/prometheus/rules/apicerberus-alerts.yml`.

| Alert | Severity | Condition |
|-------|----------|-----------|
| GatewayDown | Critical | `up == 0` for 1m |
| AdminDown | Critical | `up == 0` for 2m |
| HighErrorRate | Critical | 5xx rate > 10% for 2m |
| AuditDropped | Critical | > 100 dropped entries in 5m |
| HighLatency | Warning | p95 > 1s for 5m |
| RateLimiting | Warning | > 100 rate limit hits in 5m |
| BackendErrors | Warning | > 50 backend errors in 5m |
| AuthFailures | Warning | > 200 auth failures in 5m (brute force?) |
| DiskSpaceLow | Warning | < 10% disk space |
| HighRequestVolume | Info | > 10K req/s sustained for 10m |

## Grafana Dashboards

Three dashboards are available:

1. **APICereberus API Gateway Dashboard** (`deployments/grafana/dashboard.json`)
   - Request rate, latency percentiles, active connections, error rate
   - Backend requests by service, auth success rate, response status distribution

2. **APICerebrus Overview** (`deployments/monitoring/grafana/dashboards/apicerberus-overview.json`)
   - Gateway status, request rate by route, error rate, memory usage

3. **APICerebrus Detailed** (`deployments/monitoring/grafana/dashboards/apicerberus-detailed.json`)
   - Response status distribution, rate limit hits, auth failures, database metrics

### Auto-provisioning

The monitoring stack auto-provisions dashboards via `deployments/monitoring/grafana/dashboards/dashboards.yml`. To import manually:

1. Open Grafana → Dashboards → Import
2. Upload the JSON file or paste contents
3. Select Prometheus datasource

## Profiling

Go runtime profiling is available at admin debug endpoints (requires Bearer auth):

```bash
# CPU profile (30 seconds)
curl -H "Authorization: Bearer $TOKEN" http://localhost:9876/admin/debug/pprof/profile?seconds=30 -o cpu.prof

# Heap profile
curl -H "Authorization: Bearer $TOKEN" http://localhost:9876/admin/debug/pprof/heap -o heap.prof

# Execution trace (5 seconds)
curl -H "Authorization: Bearer $TOKEN" http://localhost:9876/admin/debug/pprof/trace?seconds=5 -o trace.out

# Analyze
go tool pprof cpu.prof
go tool pprof heap.prof
go tool trace trace.out
```

## Architecture

```
Gateway (:8080) ──/metrics──► Prometheus (:9090) ──► Grafana (:3000)
     │                            │
     │                            ├─► AlertManager (:9093) ──► Slack/Email/PagerDuty
     │                            │
     │                            └─► Rules (apicerberus-alerts.yml)
     │
     └──logs──► Promtail ──► Loki (:3100) ──► Grafana
```
