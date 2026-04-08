# OpenTelemetry Tracing

APICerebrus supports distributed tracing via OpenTelemetry, enabling you to monitor request flows through the gateway and gain insights into performance bottlenecks.

## Overview

The tracing integration provides:

- **Automatic request tracing**: All HTTP requests are traced with detailed span attributes
- **Distributed context propagation**: Trace context is propagated to upstream services
- **Plugin pipeline visibility**: Each plugin phase can be traced
- **Upstream request tracing**: Outbound proxy requests include trace headers
- **Multiple exporters**: Support for stdout, OTLP HTTP, and OTLP gRPC

## Configuration

Add the `tracing` section to your configuration file:

```yaml
tracing:
  enabled: true
  service_name: "apicerebrus"
  service_version: "1.0.0"
  exporter: "otlp-http"
  otlp_endpoint: "http://jaeger:4318"
  sampling_rate: 1.0
  attributes:
    environment: "production"
    region: "us-east-1"
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable/disable tracing |
| `service_name` | string | `"apicerebrus"` | Service name in traces |
| `service_version` | string | `"1.0.0"` | Service version in traces |
| `exporter` | string | `"stdout"` | Exporter type: `stdout`, `otlp-http`, `otlp-grpc` |
| `otlp_endpoint` | string | `""` | OTLP collector endpoint |
| `otlp_headers` | map | `{}` | Additional headers for OTLP requests |
| `sampling_rate` | float | `1.0` | Sampling rate (0.0-1.0) |
| `batch_timeout` | duration | `"5s"` | Batch timeout for span export |
| `max_queue_size` | int | `2048` | Maximum queue size for pending spans |
| `max_export_batch_size` | int | `512` | Maximum batch size for export |
| `attributes` | map | `{}` | Custom attributes added to all spans |

## Exporters

### Stdout Exporter

Outputs traces to stdout in JSON format. Useful for development and debugging:

```yaml
tracing:
  enabled: true
  exporter: "stdout"
```

### OTLP HTTP Exporter

Sends traces to an OTLP-compatible collector over HTTP:

```yaml
tracing:
  enabled: true
  exporter: "otlp-http"
  otlp_endpoint: "http://localhost:4318"
```

### OTLP gRPC Exporter

Sends traces to an OTLP-compatible collector over gRPC:

```yaml
tracing:
  enabled: true
  exporter: "otlp-grpc"
  otlp_endpoint: "localhost:4317"
```

## Jaeger Integration

To send traces to Jaeger:

```yaml
tracing:
  enabled: true
  exporter: "otlp-http"
  otlp_endpoint: "http://jaeger:4318"
  sampling_rate: 1.0
  attributes:
    environment: "production"
```

Run Jaeger with Docker:

```bash
docker run -d --name jaeger \
  -e COLLECTOR_OTLP_ENABLED=true \
  -p 16686:16686 \
  -p 4318:4318 \
  jaegertracing/all-in-one:latest
```

Access the Jaeger UI at http://localhost:16686

## Span Attributes

Each request span includes the following attributes:

### HTTP Request Attributes

- `http.method` - HTTP method (GET, POST, etc.)
- `http.url` - Full request URL
- `http.target` - Request path
- `http.host` - Request host
- `http.scheme` - URL scheme (http/https)
- `http.flavor` - HTTP protocol version
- `http.user_agent` - User agent string
- `http.client_ip` - Client IP address

### HTTP Response Attributes

- `http.status_code` - Response status code
- `http.response_content_length` - Response body size
- `http.request_content_length` - Request body size
- `http.duration_ms` - Request duration in milliseconds

### APICerebrus-Specific Attributes

- `api.key_prefix` - Prefix of the API key used (masked)
- `upstream.target_id` - Upstream target ID
- `upstream.target_address` - Upstream target address

## Context Propagation

APICerebrus uses W3C Trace Context propagation by default. Trace context is:

1. **Extracted** from incoming request headers (`traceparent`, `tracestate`)
2. **Injected** into outgoing upstream requests

This enables distributed tracing across multiple services.

## Sampling

Control the volume of traces with sampling:

```yaml
tracing:
  enabled: true
  sampling_rate: 0.1  # Trace 10% of requests
```

## Performance Considerations

- Tracing adds minimal overhead (~1-2% latency)
- Use sampling in high-traffic production environments
- The batch exporter queues spans to minimize impact
- Disable tracing if not needed for maximum performance

## Troubleshooting

### No traces appearing

1. Verify `tracing.enabled: true` in configuration
2. Check exporter endpoint connectivity
3. Ensure sampling rate is > 0
4. Check collector logs for errors

### High memory usage

1. Reduce `max_queue_size`
2. Decrease `batch_timeout` for faster export
3. Reduce `sampling_rate`

### Schema URL conflicts

If you see errors about conflicting schema URLs, ensure you're using compatible versions of OpenTelemetry packages. The tracing package uses semantic conventions v1.40.0.
