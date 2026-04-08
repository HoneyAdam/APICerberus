# Kafka Audit Log Streaming

APICerebrus supports streaming audit logs to Apache Kafka for external processing, analytics, and long-term storage.

## Overview

By default, audit logs are stored in SQLite with configurable retention. Kafka streaming enables:

- **Centralized logging**: Stream logs to a central Kafka cluster
- **Real-time analytics**: Process audit events in real-time with stream processing
- **Long-term storage**: Archive logs to external systems (S3, Elasticsearch, etc.)
- **Compliance**: Meet regulatory requirements with immutable audit trails
- **Integration**: Feed audit data into SIEM systems

## Configuration

Add the `kafka` section to your configuration file:

```yaml
kafka:
  enabled: true
  brokers:
    - "kafka-1:9092"
    - "kafka-2:9092"
  topic: "apicerberus.audit"
  client_id: "apicerberus-gateway-1"
  gateway_id: "gw-prod-001"
  region: "us-east-1"
  datacenter: "dc-virginia"
  
  # TLS Configuration (optional)
  tls:
    enabled: true
    skip_verify: false
    server_name: "kafka.example.com"
  
  # SASL Authentication (optional)
  sasl:
    enabled: true
    mechanism: "scram-sha-256"  # or "plain", "scram-sha-512"
    username: "apicerberus"
    password: "${KAFKA_PASSWORD}"
  
  # Batch settings
  batch_size: 100
  buffer_size: 10000
  flush_interval: "1s"
  
  # Connection settings
  dial_timeout: "5s"
  write_timeout: "3s"
  workers: 3
  
  # Behavior settings
  block_on_full: false    # Block producers when buffer is full
  async_connect: true     # Don't fail startup if Kafka is unavailable
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable Kafka streaming |
| `brokers` | []string | `[]` | Kafka broker addresses |
| `topic` | string | `"apicerberus.audit"` | Kafka topic for audit logs |
| `client_id` | string | `""` | Client ID for Kafka connection |
| `gateway_id` | string | `""` | Unique gateway identifier |
| `region` | string | `""` | Deployment region |
| `datacenter` | string | `""` | Data center name |
| `batch_size` | int | `100` | Messages per batch |
| `buffer_size` | int | `10000` | In-memory buffer size |
| `flush_interval` | duration | `"1s"` | Maximum time before flush |
| `dial_timeout` | duration | `"5s"` | Connection timeout |
| `write_timeout` | duration | `"3s"` | Write operation timeout |
| `workers` | int | `3` | Background worker count |
| `block_on_full` | bool | `false` | Block when buffer full |
| `async_connect` | bool | `true` | Connect asynchronously |

## Message Format

Audit logs are sent as JSON messages:

```json
{
  "version": "1.0",
  "type": "audit_log",
  "timestamp": "2026-04-07T10:30:00.123456789Z",
  "gateway_id": "gw-prod-001",
  "audit_entry": {
    "request_id": "req-abc-123",
    "route_id": "route-users",
    "route_name": "Users API",
    "service_name": "user-service",
    "user_id": "user-456",
    "consumer_name": "mobile-app",
    "method": "GET",
    "host": "api.example.com",
    "path": "/api/v1/users",
    "query": "page=1&limit=10",
    "status_code": 200,
    "latency_ms": 45,
    "bytes_in": 0,
    "bytes_out": 1234,
    "client_ip": "192.168.1.100",
    "user_agent": "MobileApp/1.0",
    "blocked": false,
    "block_reason": "",
    "request_headers": {...},
    "request_body": "",
    "response_headers": {...},
    "response_body": "",
    "error_message": "",
    "created_at": "2026-04-07T10:30:00.123456789Z"
  },
  "metadata": {
    "region": "us-east-1",
    "data_center": "dc-virginia"
  }
}
```

## How It Works

### Dual Write Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                      AUDIT LOGGER                               │
│                                                                 │
│  ┌──────────────┐      ┌──────────────┐     ┌──────────────┐   │
│  │   SQLite     │      │   Buffer     │     │    Kafka     │   │
│  │   (Local)    │◄────►│   (Queue)    │────►│  (External)  │   │
│  └──────────────┘      └──────────────┘     └──────────────┘   │
│        │                                              │         │
│        │                                              │         │
│        ▼                                              ▼         │
│  ┌──────────────┐                              ┌──────────────┐│
│  │   Archive    │                              │   Consumer   ││
│  │   (GZIP)     │                              │   (SIEM)     ││
│  └──────────────┘                              └──────────────┘│
└─────────────────────────────────────────────────────────────────┘
```

### Flow

1. **Request Processing**: Gateway processes request through plugin pipeline
2. **Audit Entry Creation**: Audit logger creates entry with masked fields
3. **SQLite Write**: Entry persisted to local SQLite (primary storage)
4. **Kafka Queue**: Entry added to in-memory buffer queue
5. **Batch Send**: Workers batch messages and send to Kafka
6. **Fallback**: If Kafka unavailable, logs remain in SQLite

## Monitoring

### Statistics

Access streaming statistics via the logger:

```go
stats := kafkaWriter.Stats()
fmt.Printf("Sent: %d, Failed: %d, Dropped: %d, Queued: %d\n",
    stats.Sent, stats.Failed, stats.Dropped, stats.Queued)
```

### Health Check

Check Kafka connection health:

```go
if kafkaWriter.IsHealthy() {
    // Kafka connection is healthy
}
```

## Performance Considerations

### Throughput

- Local SQLite: ~10,000 entries/second
- Kafka streaming: Depends on network latency
- Batch size of 100 with 3 workers: ~5,000-10,000 msgs/second

### Latency

- SQLite write: < 1ms
- Kafka queue: < 100µs (in-memory)
- Network round-trip: Depends on broker distance

### Memory Usage

- Buffer size 10,000 × ~2KB per entry = ~20MB max

## Security

### TLS Encryption

Enable TLS for encrypted connections:

```yaml
kafka:
  tls:
    enabled: true
    cert_file: "/etc/apicerberus/kafka-client.crt"
    key_file: "/etc/apicerberus/kafka-client.key"
    ca_file: "/etc/apicerberus/kafka-ca.crt"
```

### SASL Authentication

Support for multiple SASL mechanisms:

- **PLAIN**: Username/password authentication
- **SCRAM-SHA-256**: Secure salted challenge-response
- **SCRAM-SHA-512**: Stronger variant

### Field Masking

Sensitive fields are masked before streaming:

- Headers: `Authorization`, `Cookie`, `X-API-Key`
- Body fields: `password`, `token`, `secret`, `credit_card`

## Troubleshooting

### Connection Failures

**Symptom**: Logs show "failed to connect to any broker"

**Solutions**:
- Verify broker addresses and ports
- Check network connectivity: `telnet kafka-host 9092`
- Verify TLS certificates if enabled
- Check SASL credentials

### High Memory Usage

**Symptom**: Memory growing continuously

**Solutions**:
- Reduce `buffer_size` if messages aren't being consumed
- Check Kafka broker health
- Enable `block_on_full: false` to drop instead of buffer

### Message Loss

**Symptom**: Some audit logs missing in Kafka

**Solutions**:
- Check `Dropped` count in stats
- Increase `buffer_size` if dropped due to full buffer
- Reduce `flush_interval` for more frequent sends
- Check network stability

## Testing

### Local Testing with Docker

Start Kafka with Docker Compose:

```yaml
version: '3'
services:
  zookeeper:
    image: confluentinc/cp-zookeeper:latest
    environment:
      ZOOKEEPER_CLIENT_PORT: 2181
  
  kafka:
    image: confluentinc/cp-kafka:latest
    ports:
      - "9092:9092"
    environment:
      KAFKA_BROKER_ID: 1
      KAFKA_ZOOKEEPER_CONNECT: zookeeper:2181
      KAFKA_ADVERTISED_LISTENERS: PLAINTEXT://localhost:9092
      KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR: 1
```

Create topic:

```bash
kafka-topics --create --topic apicerberus.audit --bootstrap-server localhost:9092
```

Consume messages:

```bash
kafka-console-consumer --topic apicerberus.audit --from-beginning --bootstrap-server localhost:9092
```

## Integration Examples

### Elasticsearch Pipeline

```
Kafka ──► Logstash ──► Elasticsearch ──► Kibana
```

Logstash configuration:

```ruby
input {
  kafka {
    bootstrap_servers => "kafka:9092"
    topics => ["apicerberus.audit"]
    codec => "json"
  }
}

output {
  elasticsearch {
    hosts => ["elasticsearch:9200"]
    index => "apicerberus-audit-%{+YYYY.MM.dd}"
  }
}
```

### SIEM Integration

Forward to Splunk, Datadog, or other SIEM tools using Kafka Connect or custom consumers.

---

*Document Version: 1.0*  
*APICerebrus Version: 1.0.0*
