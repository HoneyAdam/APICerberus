# APICerebrus New API Features

## GraphQL Admin API

The GraphQL Admin API provides a flexible, type-safe interface for querying and mutating gateway resources.

### Endpoint

```http
POST /admin/graphql
GET /admin/graphql
```

### Authentication

Same as REST API - use `X-Admin-Key` header.

### Example Queries

#### Get all services

```graphql
query {
  services {
    id
    name
    protocol
    upstream
  }
}
```

#### Get specific service

```graphql
query {
  service(id: "user-service") {
    id
    name
    protocol
    upstream
    connectTimeout
    readTimeout
    writeTimeout
  }
}
```

#### Get audit logs with pagination

```graphql
query {
  auditLogs(limit: 10, offset: 0) {
    entries {
      id
      routeName
      method
      path
      statusCode
      latencyMs
      createdAt
    }
    total
  }
}
```

### Example Mutations

#### Create a service

```graphql
mutation {
  createService(input: {
    name: "payment-service"
    protocol: "http"
    upstream: "payment-upstream"
  }) {
    id
    name
    protocol
  }
}
```

#### Update a route

```graphql
mutation {
  updateRoute(id: "route-123", input: {
    name: "updated-route"
    service: "user-service"
    paths: ["/api/v1/users"]
    methods: ["GET", "POST"]
  }) {
    id
    name
    paths
    methods
  }
}
```

#### Delete an upstream

```graphql
mutation {
  deleteUpstream(id: "upstream-123")
}
```

### Schema Types

- `Service` - Gateway service configuration
- `Route` - Route definitions with paths, methods, plugins
- `Upstream` - Upstream targets with load balancing
- `Consumer` - API consumers with keys and ACLs
- `User` - Portal users with credits and permissions
- `AuditLog` - Request/response audit entries
- `GatewayInfo` - Gateway statistics summary

---

## Bulk Operations API

The Bulk Operations API allows efficient management of multiple resources in a single request with transactional guarantees.

### Bulk Create Services

```http
POST /admin/api/v1/bulk/services
```

**Request:**
```json
{
  "services": [
    {
      "id": "svc-1",
      "name": "service-1",
      "protocol": "http",
      "upstream": "upstream-1"
    },
    {
      "id": "svc-2",
      "name": "service-2",
      "protocol": "http",
      "upstream": "upstream-2"
    }
  ]
}
```

**Response:**
```json
{
  "success": true,
  "created": 2,
  "updated": 0,
  "deleted": 0,
  "failed": 0,
  "errors": [],
  "timestamp": "2024-01-15T10:30:00Z"
}
```

### Bulk Create Routes

```http
POST /admin/api/v1/bulk/routes
```

**Request:**
```json
{
  "routes": [
    {
      "name": "route-1",
      "service": "svc-1",
      "paths": ["/api/v1/users"],
      "methods": ["GET", "POST"]
    },
    {
      "name": "route-2",
      "service": "svc-2",
      "paths": ["/api/v1/orders"],
      "methods": ["GET"]
    }
  ]
}
```

### Bulk Delete Resources

```http
POST /admin/api/v1/bulk/delete
```

**Request:**
```json
{
  "resources": [
    {"type": "service", "id": "svc-1"},
    {"type": "route", "id": "route-1"},
    {"type": "upstream", "id": "upstream-1"}
  ]
}
```

**Resource Types:** `service`, `route`, `upstream`, `consumer`

### Bulk Apply Plugins

```http
POST /admin/api/v1/bulk/plugins
```

**Request:**
```json
{
  "route_ids": ["route-1", "route-2", "route-3"],
  "plugins": [
    {
      "name": "rate_limit",
      "enabled": true,
      "config": {
        "requests_per_second": 100,
        "burst": 150
      }
    }
  ],
  "mode": "merge"
}
```

**Modes:**
- `append` - Add plugins to existing ones
- `replace` - Replace all plugins
- `merge` - Update existing plugins by name, add new ones

### Bulk Import

```http
POST /admin/api/v1/bulk/import
```

**Request:**
```json
{
  "mode": "upsert",
  "services": [...],
  "routes": [...],
  "upstreams": [...],
  "consumers": [...]
}
```

**Modes:** `create`, `upsert`, `replace`

---

## Advanced Analytics API

### Traffic Forecasting

Predict future traffic patterns based on historical data.

```http
GET /admin/api/v1/analytics/forecast?metric=requests&horizon=24&route_id=route-123
```

**Parameters:**
- `metric` - `requests`, `latency`, or `errors`
- `horizon` - Hours to forecast (1-168, default: 24)
- `route_id` - Optional route filter

**Response:**
```json
{
  "metric": "requests",
  "route_id": "route-123",
  "horizon": 24,
  "forecast": [
    {
      "timestamp": "2024-01-15T11:00:00Z",
      "value": 1250.5,
      "lower": 1000.4,
      "upper": 1500.6
    }
  ],
  "confidence": 0.85,
  "trend": "up",
  "seasonality": 0.15,
  "generated_at": "2024-01-15T10:00:00Z"
}
```

### Anomaly Detection

Detect unusual patterns in traffic, latency, or error rates.

```http
GET /admin/api/v1/analytics/anomalies?metric=requests&threshold=2.5
```

**Parameters:**
- `metric` - `requests`, `latency`, or `error_rate`
- `threshold` - Z-score threshold (default: 2.5)
- `route_id` - Optional route filter
- `start_time` - ISO 8601 timestamp
- `end_time` - ISO 8601 timestamp

**Response:**
```json
{
  "metric": "requests",
  "threshold": 2.5,
  "anomalies": [
    {
      "timestamp": "2024-01-15T08:30:00Z",
      "value": 5000,
      "expected": 1200,
      "z_score": 3.2,
      "severity": "high"
    }
  ],
  "total_checked": 1440,
  "anomaly_count": 3,
  "anomaly_rate": 0.0021,
  "generated_at": "2024-01-15T10:00:00Z"
}
```

### Metric Correlations

Analyze correlations between different metrics.

```http
GET /admin/api/v1/analytics/correlations?metrics=requests,latency,error_rate
```

**Response:**
```json
{
  "correlations": [
    {
      "metric_1": "requests",
      "metric_2": "latency",
      "coefficient": 0.75,
      "strength": "strong",
      "direction": "positive"
    }
  ],
  "generated_at": "2024-01-15T10:00:00Z"
}
```

### Data Export

Export analytics data in JSON or CSV format.

```http
GET /admin/api/v1/analytics/exports?format=csv&start_time=2024-01-01T00:00:00Z&end_time=2024-01-15T00:00:00Z
```

**Parameters:**
- `format` - `json` or `csv`
- `start_time` - ISO 8601 timestamp
- `end_time` - ISO 8601 timestamp
- `route_id` - Optional route filter
- `user_id` - Optional user filter
- `limit` - Maximum records (max: 50000)

---

## Webhook Management API

### List Webhooks

```http
GET /admin/api/v1/webhooks
```

### Create Webhook

```http
POST /admin/api/v1/webhooks
```

**Request:**
```json
{
  "name": "Production Alerts",
  "url": "https://hooks.example.com/webhook",
  "events": ["route.created", "service.updated", "alert.triggered"],
  "headers": {
    "X-Custom-Header": "value"
  },
  "retry_count": 3,
  "retry_interval": 60,
  "timeout": 30
}
```

### Get Webhook

```http
GET /admin/api/v1/webhooks/{id}
```

### Update Webhook

```http
PUT /admin/api/v1/webhooks/{id}
```

### Delete Webhook

```http
DELETE /admin/api/v1/webhooks/{id}
```

### List Webhook Events

```http
GET /admin/api/v1/webhooks/events
```

**Response:**
```json
[
  {"type": "route.created", "description": "Route created", "category": "route"},
  {"type": "route.updated", "description": "Route updated", "category": "route"},
  {"type": "service.created", "description": "Service created", "category": "service"},
  {"type": "alert.triggered", "description": "Alert triggered", "category": "alert"}
]
```

### Get Delivery History

```http
GET /admin/api/v1/webhooks/{id}/deliveries?limit=50
```

**Response:**
```json
[
  {
    "id": "del_abc123",
    "webhook_id": "wh_def456",
    "event_type": "route.created",
    "status": "success",
    "status_code": 200,
    "attempt": 1,
    "max_attempts": 4,
    "created_at": "2024-01-15T10:00:00Z",
    "completed_at": "2024-01-15T10:00:01Z"
  }
]
```

### Test Webhook

```http
POST /admin/api/v1/webhooks/{id}/test
```

Sends a test event to the webhook URL.

**Response:**
```json
{
  "success": true,
  "status_code": 200,
  "response": "OK"
}
```

### Rotate Webhook Secret

```http
POST /admin/api/v1/webhooks/{id}/rotate-secret
```

**Response:**
```json
{
  "webhook_id": "wh_def456",
  "secret": "new-secret-value",
  "message": "Secret rotated successfully. Store this secret securely as it will not be shown again."
}
```

### Webhook Payload Format

Webhook deliveries include these headers:

- `X-Webhook-ID` - Webhook configuration ID
- `X-Webhook-Event` - Event type that triggered the delivery
- `X-Webhook-Delivery` - Unique delivery ID
- `X-Webhook-Timestamp` - Unix timestamp of delivery
- `X-Webhook-Signature` - HMAC-SHA256 signature (if secret configured)

**Example Payload:**
```json
{
  "event": "route.created",
  "timestamp": "2024-01-15T10:00:00Z",
  "data": {
    "id": "route-123",
    "name": "new-route",
    "service": "user-service",
    "paths": ["/api/users"]
  }
}
```

### Signature Verification

Verify webhook signatures using HMAC-SHA256:

```go
import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
)

func verifySignature(payload []byte, signature, secret string) bool {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write(payload)
    expected := hex.EncodeToString(mac.Sum(nil))
    return hmac.Equal([]byte(signature), []byte(expected))
}
```

---

## Changelog

### v1.1.0

- Added GraphQL Admin API at `/admin/graphql`
- Added Bulk Operations API for batch resource management
- Added Advanced Analytics API with forecasting and anomaly detection
- Added Webhook Management API with delivery history and retry logic
