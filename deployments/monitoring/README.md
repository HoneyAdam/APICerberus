# APICerebrus Monitoring Stack

Complete monitoring solution for APICerebrus API Gateway using Prometheus, Grafana, Loki, and AlertManager.

## Components

- **Prometheus** - Metrics collection and alerting
- **Grafana** - Visualization and dashboards
- **Loki** - Log aggregation
- **Promtail** - Log collection
- **AlertManager** - Alert routing and management
- **Node Exporter** - System metrics
- **cAdvisor** - Container metrics
- **Blackbox Exporter** - Endpoint probing

## Quick Start

### 1. Configure Environment

```bash
cp .env.example .env
# Edit .env with your settings
```

### 2. Start the Stack

```bash
docker-compose up -d
```

### 3. Access Services

- **Grafana**: http://localhost:3000 (admin/admin)
- **Prometheus**: http://localhost:9090
- **AlertManager**: http://localhost:9093
- **Loki**: http://localhost:3100

## Configuration

### Prometheus

Edit `prometheus/prometheus.yml` to:
- Add additional scrape targets
- Configure remote storage
- Modify retention settings

### AlertManager

Edit `alertmanager/alertmanager.yml` to:
- Configure notification channels (email, Slack, PagerDuty)
- Set up routing rules
- Define inhibition rules

### Grafana

Dashboards are automatically provisioned from `grafana/dashboards/`.

To add custom dashboards:
1. Create dashboard in Grafana UI
2. Export as JSON
3. Save to `grafana/dashboards/`
4. Restart Grafana

### Loki

Edit `loki/loki.yml` to:
- Adjust retention settings
- Configure storage backend
- Set resource limits

## Alerting Rules

Alerts are defined in `prometheus/rules/apicerberus-alerts.yml`:

- **Critical**: Gateway down, high error rate, database errors
- **Warning**: High latency, rate limiting, memory/disk usage
- **Info**: High request volume, large audit logs

## Dashboards

### APICerebrus Overview
- Gateway and Admin API status
- Request rate and error rate
- Response time percentiles
- Memory usage

### APICerebrus Detailed Metrics
- Response status distribution
- Requests by status code
- Rate limiting metrics
- Authentication failures
- Database query latency

## Log Queries (Loki)

### View All APICerebrus Logs
```
{job="apicerberus"}
```

### View Error Logs Only
```
{job="apicerberus"} |= "ERROR"
```

### View Logs for Specific Route
```
{job="apicerberus"} |= "/api/v1/users"
```

### View Audit Logs
```
{job="apicerberus-audit"}
```

### View Logs by Request ID
```
{job="apicerberus"} |= "request_id=\"abc123\""
```

## Maintenance

### Backup Grafana Dashboards
```bash
docker exec apicerberus-grafana grafana-cli admin export-dashboards
```

### Update Stack
```bash
docker-compose pull
docker-compose up -d
```

### View Logs
```bash
# All services
docker-compose logs -f

# Specific service
docker-compose logs -f prometheus
```

### Reset Data
```bash
# WARNING: This will delete all metrics and logs
docker-compose down -v
```

## Production Considerations

### Security
1. Change default passwords in `.env`
2. Enable HTTPS for external access
3. Use reverse proxy (nginx/traefik) with authentication
4. Restrict network access to monitoring ports

### Storage
1. Mount persistent volumes for data retention
2. Configure appropriate retention periods
3. Set up backup for Prometheus/Grafana data
4. Monitor disk usage

### High Availability
1. Run multiple Prometheus instances (federation)
2. Use external AlertManager cluster
3. Configure remote storage (Thanos/Cortex)
4. Set up Grafana HA with shared database

### Performance
1. Adjust scrape intervals based on needs
2. Configure recording rules for complex queries
3. Use Prometheus remote write for long-term storage
4. Monitor monitoring stack performance

## Troubleshooting

### No Metrics in Grafana
1. Check Prometheus targets: http://localhost:9090/targets
2. Verify APICerebrus metrics endpoint: http://apicerberus:8080/metrics
3. Check Prometheus logs: `docker-compose logs prometheus`

### No Logs in Loki
1. Check Promtail status: `docker-compose ps promtail`
2. Verify log file paths in `promtail.yml`
3. Check Loki logs: `docker-compose logs loki`

### Alerts Not Firing
1. Check AlertManager status: http://localhost:9093/#/status
2. Verify alert rules in Prometheus: http://localhost:9090/rules
3. Check AlertManager logs: `docker-compose logs alertmanager`

### High Memory Usage
1. Reduce Prometheus retention: `--storage.tsdb.retention.time=15d`
2. Adjust Loki limits in `loki.yml`
3. Increase scrape intervals
4. Add memory limits in `docker-compose.yml`

## Integration with APICerebrus

### Enable Metrics Endpoint

Ensure APICerebrus configuration has metrics enabled:

```yaml
metrics:
  enabled: true
  path: /metrics
```

### Configure Log Format

For best results with Loki, use structured logging:

```yaml
logging:
  format: json
  level: info
```

### Custom Metrics

APICerebrus exposes these custom metrics:

- `http_requests_total` - Total HTTP requests
- `http_request_duration_seconds` - Request latency
- `rate_limit_hits_total` - Rate limiting events
- `auth_failures_total` - Authentication failures
- `db_query_duration_seconds` - Database query latency
- `db_errors_total` - Database errors

## Support

For issues and feature requests, please refer to the main APICerebrus documentation.
