# APICerebrus API Documentation

Complete reference for the APICerebrus Admin REST API.

## Table of Contents

- [Overview](#overview)
- [Authentication](#authentication)
- [Core Endpoints](#core-endpoints)
- [Gateway Management](#gateway-management)
- [User Management](#user-management)
- [Credit Management](#credit-management)
- [Audit Logging](#audit-logging)
- [Analytics](#analytics)
- [Billing Configuration](#billing-configuration)
- [GraphQL Federation](#graphql-federation)
- [Alerts](#alerts)
- [Webhooks](#webhooks)
- [Bulk Operations](#bulk-operations)
- [Advanced Analytics](#advanced-analytics)
- [GraphQL Admin API](#graphql-admin-api)
- [WebSocket](#websocket)
- [Portal API](#portal-api)
- [Error Handling](#error-handling)

---

## Overview

**Base URL:** `http://localhost:9876/admin/api/v1`

**Content-Type:** `application/json`

The Admin API provides complete management capabilities for APICerebrus gateway, including configuration, users, credits, audit logs, and analytics.

---

## Authentication

All Admin API endpoints require authentication via the `X-Admin-Key` header.

```bash
X-Admin-Key: your-admin-api-key
```

The admin key is configured in `apicerberus.yaml`:

```yaml
admin:
  addr: ":9876"
  api_key: "your-secure-admin-key"
```

### Failed Authentication

After 5 failed authentication attempts from a single IP, that IP will be rate-limited for 15 minutes.

**Response:**
```json
{
  "error": "rate_limited",
  "message": "Too many failed authentication attempts. Please try again later."
}
```

---

## Core Endpoints

### Get Status

Returns the current gateway status and health information.

```http
GET /admin/api/v1/status
```

**Response:**
```json
{
  "status": "healthy",
  "version": "1.0.0",
  "uptime": "72h15m30s",
  "started_at": "2024-01-15T08:30:00Z",
  "services": 5,
  "routes": 12,
  "upstreams": 3,
  "active_connections": 42
}
```

### Get Info

Returns version and build information.

```http
GET /admin/api/v1/info
```

**Response:**
```json
{
  "version": "1.0.0",
  "commit": "abc123",
  "build_time": "2024-01-15T08:00:00Z",
  "go_version": "go1.25.0",
  "platform": "linux/amd64"
}
```

### Reload Configuration

Hot reloads the configuration file without restarting the gateway.

```http
POST /admin/api/v1/config/reload
```

**Response:**
```json
{
  "reloaded": true
}
```

### Export Configuration

Exports the current active configuration as YAML.

```http
GET /admin/api/v1/config/export
```

**Response:** YAML configuration file

### Import Configuration

Imports and applies a new configuration.

```http
POST /admin/api/v1/config/import
Content-Type: application/json
```

**Request Body:**
```json
{
  "gateway": {
    "http_addr": ":8080"
  },
  "services": [...],
  "routes": [...],
  "upstreams": [...]
}
```

**Response:**
```json
{
  "imported": true,
  "services_added": 2,
  "routes_added": 5,
  "upstreams_added": 1
}
```

---

## Gateway Management

### Services

#### List Services

```http
GET /admin/api/v1/services
```

**Response:**
```json
[
  {
    "id": "user-service",
    "name": "user-service",
    "protocol": "http",
    "upstream": "user-upstream",
    "connect_timeout": "5s",
    "read_timeout": "30s",
    "write_timeout": "30s"
  }
]
```

#### Create Service

```http
POST /admin/api/v1/services
Content-Type: application/json
```

**Request Body:**
```json
{
  "id": "user-service",
  "name": "user-service",
  "protocol": "http",
  "upstream": "user-upstream",
  "connect_timeout": "5s",
  "read_timeout": "30s",
  "write_timeout": "30s"
}
```

**Response:** `201 Created`

#### Get Service

```http
GET /admin/api/v1/services/{id}
```

**Response:** Service object or `404 Not Found`

#### Update Service

```http
PUT /admin/api/v1/services/{id}
Content-Type: application/json
```

**Request Body:** Service object (must include matching `id`)

**Response:** Updated service object

#### Delete Service

```http
DELETE /admin/api/v1/services/{id}
```

**Response:** `204 No Content`

**Errors:**
- `409 Conflict` - Service is in use by routes

---

### Routes

#### List Routes

```http
GET /admin/api/v1/routes
```

**Response:**
```json
[
  {
    "id": "users-api",
    "name": "users-api",
    "service": "user-service",
    "hosts": ["api.example.com"],
    "paths": ["/api/v1/users", "/api/v1/users/*"],
    "methods": ["GET", "POST"],
    "strip_path": false,
    "priority": 100,
    "plugins": [
      {
        "name": "rate-limit",
        "config": {
          "algorithm": "fixed_window",
          "scope": "route",
          "limit": 300,
          "window": "1s"
        }
      }
    ]
  }
]
```

#### Create Route

```http
POST /admin/api/v1/routes
Content-Type: application/json
```

**Request Body:**
```json
{
  "id": "users-api",
  "name": "users-api",
  "service": "user-service",
  "hosts": ["api.example.com"],
  "paths": ["/api/v1/users/*"],
  "methods": ["GET", "POST", "PUT", "DELETE"],
  "strip_path": false,
  "priority": 100,
  "plugins": []
}
```

**Response:** `201 Created`

#### Get Route

```http
GET /admin/api/v1/routes/{id}
```

#### Update Route

```http
PUT /admin/api/v1/routes/{id}
Content-Type: application/json
```

#### Delete Route

```http
DELETE /admin/api/v1/routes/{id}
```

---

### Upstreams

#### List Upstreams

```http
GET /admin/api/v1/upstreams
```

**Response:**
```json
[
  {
    "id": "user-upstream",
    "name": "user-upstream",
    "algorithm": "round_robin",
    "targets": [
      {
        "id": "target-1",
        "address": "10.0.1.10:8080",
        "weight": 100,
        "healthy": true
      },
      {
        "id": "target-2",
        "address": "10.0.1.11:8080",
        "weight": 100,
        "healthy": true
      }
    ],
    "health_check": {
      "active": {
        "path": "/health",
        "interval": "10s",
        "timeout": "2s",
        "healthy_threshold": 2,
        "unhealthy_threshold": 3
      }
    }
  }
]
```

#### Create Upstream

```http
POST /admin/api/v1/upstreams
Content-Type: application/json
```

**Request Body:**
```json
{
  "id": "user-upstream",
  "name": "user-upstream",
  "algorithm": "round_robin",
  "targets": [
    {
      "address": "10.0.1.10:8080",
      "weight": 100
    }
  ],
  "health_check": {
    "active": {
      "path": "/health",
      "interval": "10s",
      "timeout": "2s",
      "healthy_threshold": 2,
      "unhealthy_threshold": 3
    }
  }
}
```

#### Get Upstream

```http
GET /admin/api/v1/upstreams/{id}
```

#### Update Upstream

```http
PUT /admin/api/v1/upstreams/{id}
Content-Type: application/json
```

#### Delete Upstream

```http
DELETE /admin/api/v1/upstreams/{id}
```

#### Add Target

```http
POST /admin/api/v1/upstreams/{id}/targets
Content-Type: application/json
```

**Request Body:**
```json
{
  "address": "10.0.1.12:8080",
  "weight": 100
}
```

#### Remove Target

```http
DELETE /admin/api/v1/upstreams/{id}/targets/{targetId}
```

#### Get Health Status

```http
GET /admin/api/v1/upstreams/{id}/health
```

**Response:**
```json
{
  "upstream_id": "user-upstream",
  "overall_status": "healthy",
  "targets": [
    {
      "id": "target-1",
      "address": "10.0.1.10:8080",
      "healthy": true,
      "last_check": "2024-01-15T10:30:00Z",
      "latency_ms": 5
    },
    {
      "id": "target-2",
      "address": "10.0.1.11:8080",
      "healthy": false,
      "last_check": "2024-01-15T10:30:00Z",
      "error": "connection refused"
    }
  ]
}
```

---

## User Management

### List Users

```http
GET /admin/api/v1/users?page=1&limit=50&search=john&role=admin&status=active
```

**Query Parameters:**
- `page` - Page number (default: 1)
- `limit` - Items per page (default: 50, max: 100)
- `search` - Search by name or email
- `role` - Filter by role
- `status` - Filter by status (active, suspended)

**Response:**
```json
{
  "users": [
    {
      "id": "user-123",
      "email": "john@example.com",
      "name": "John Doe",
      "role": "user",
      "status": "active",
      "credits": 1000,
      "rate_limit_rps": 100,
      "created_at": "2024-01-01T00:00:00Z",
      "updated_at": "2024-01-15T10:00:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 50,
    "total": 150,
    "pages": 3
  }
}
```

### Create User

```http
POST /admin/api/v1/users
Content-Type: application/json
```

**Request Body:**
```json
{
  "email": "john@example.com",
  "name": "John Doe",
  "role": "user",
  "credits": 1000,
  "rate_limit_rps": 100
}
```

**Response:** `201 Created`

### Get User

```http
GET /admin/api/v1/users/{id}
```

### Update User

```http
PUT /admin/api/v1/users/{id}
Content-Type: application/json
```

**Request Body:**
```json
{
  "name": "John Updated",
  "rate_limit_rps": 200
}
```

### Delete User

```http
DELETE /admin/api/v1/users/{id}
```

### Suspend User

```http
POST /admin/api/v1/users/{id}/suspend
```

**Response:**
```json
{
  "id": "user-123",
  "status": "suspended",
  "suspended_at": "2024-01-15T10:30:00Z"
}
```

### Activate User

```http
POST /admin/api/v1/users/{id}/activate
```

### Reset Password

```http
POST /admin/api/v1/users/{id}/reset-password
Content-Type: application/json
```

**Request Body:**
```json
{
  "new_password": "new-secure-password"
}
```

---

### API Keys

#### List User API Keys

```http
GET /admin/api/v1/users/{id}/api-keys
```

**Response:**
```json
{
  "keys": [
    {
      "id": "key-123",
      "name": "Production Key",
      "key": "ck_live_abc123...",
      "mode": "live",
      "created_at": "2024-01-01T00:00:00Z",
      "last_used_at": "2024-01-15T10:00:00Z",
      "expires_at": null
    }
  ]
}
```

#### Create API Key

```http
POST /admin/api/v1/users/{id}/api-keys
Content-Type: application/json
```

**Request Body:**
```json
{
  "name": "Mobile App Key",
  "mode": "live",
  "expires_at": "2025-01-01T00:00:00Z"
}
```

**Response:**
```json
{
  "id": "key-456",
  "name": "Mobile App Key",
  "key": "ck_live_xyz789...",
  "mode": "live",
  "created_at": "2024-01-15T10:30:00Z",
  "expires_at": "2025-01-01T00:00:00Z"
}
```

**Note:** The full key is only returned on creation.

#### Revoke API Key

```http
DELETE /admin/api/v1/users/{id}/api-keys/{keyId}
```

---

### Permissions

#### List User Permissions

```http
GET /admin/api/v1/users/{id}/permissions
```

**Response:**
```json
{
  "permissions": [
    {
      "id": "perm-123",
      "route": "users-api",
      "methods": ["GET", "POST"],
      "allowed_days": ["monday", "tuesday", "wednesday", "thursday", "friday"],
      "allowed_hours": { "start": "09:00", "end": "17:00" },
      "created_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

#### Grant Permission

```http
POST /admin/api/v1/users/{id}/permissions
Content-Type: application/json
```

**Request Body:**
```json
{
  "route": "users-api",
  "methods": ["GET", "POST"],
  "allowed_days": ["monday", "tuesday", "wednesday", "thursday", "friday"],
  "allowed_hours": { "start": "09:00", "end": "17:00" }
}
```

#### Update Permission

```http
PUT /admin/api/v1/users/{id}/permissions/{permissionId}
Content-Type: application/json
```

#### Revoke Permission

```http
DELETE /admin/api/v1/users/{id}/permissions/{permissionId}
```

#### Bulk Assign Permissions

```http
POST /admin/api/v1/users/{id}/permissions/bulk
Content-Type: application/json
```

**Request Body:**
```json
{
  "permissions": [
    {
      "route": "users-api",
      "methods": ["GET", "POST"]
    },
    {
      "route": "orders-api",
      "methods": ["GET"]
    }
  ]
}
```

---

### IP Whitelist

#### List IP Whitelist

```http
GET /admin/api/v1/users/{id}/ip-whitelist
```

**Response:**
```json
{
  "ips": [
    {
      "ip": "192.168.1.0/24",
      "description": "Office network",
      "created_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

#### Add IP to Whitelist

```http
POST /admin/api/v1/users/{id}/ip-whitelist
Content-Type: application/json
```

**Request Body:**
```json
{
  "ip": "10.0.0.0/8",
  "description": "VPN network"
}
```

#### Remove IP from Whitelist

```http
DELETE /admin/api/v1/users/{id}/ip-whitelist/{ip}
```

---

## Credit Management

### Credit Overview

```http
GET /admin/api/v1/credits/overview
```

**Response:**
```json
{
  "total_credits_issued": 100000,
  "total_credits_consumed": 45000,
  "active_users": 150,
  "average_balance": 366,
  "transactions_today": 5234,
  "credits_consumed_today": 8900
}
```

### Get User Credit Balance

```http
GET /admin/api/v1/users/{id}/credits/balance
```

**Response:**
```json
{
  "user_id": "user-123",
  "balance": 1500,
  "currency": "credits",
  "last_updated": "2024-01-15T10:30:00Z"
}
```

### Top Up Credits

```http
POST /admin/api/v1/users/{id}/credits/topup
Content-Type: application/json
```

**Request Body:**
```json
{
  "amount": 1000,
  "reason": "Monthly allocation",
  "reference": "monthly-2024-01"
}
```

**Response:**
```json
{
  "transaction_id": "txn-123",
  "user_id": "user-123",
  "type": "credit",
  "amount": 1000,
  "balance_after": 2500,
  "reason": "Monthly allocation",
  "created_at": "2024-01-15T10:30:00Z"
}
```

### Deduct Credits

```http
POST /admin/api/v1/users/{id}/credits/deduct
Content-Type: application/json
```

**Request Body:**
```json
{
  "amount": 100,
  "reason": "API usage charge",
  "reference": "usage-2024-01-15"
}
```

### List Credit Transactions

```http
GET /admin/api/v1/users/{id}/credits/transactions?page=1&limit=50
```

**Response:**
```json
{
  "transactions": [
    {
      "id": "txn-123",
      "type": "debit",
      "amount": 5,
      "balance_after": 1495,
      "reason": "API request: users-api",
      "route": "users-api",
      "created_at": "2024-01-15T10:30:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 50,
    "total": 234
  }
}
```

---

## Audit Logging

### Search Audit Logs

```http
GET /admin/api/v1/audit-logs?user=user-123&route=users-api&method=GET&status=200&since=2024-01-01T00:00:00Z&until=2024-01-15T23:59:59Z&page=1&limit=100
```

**Query Parameters:**
- `user` - Filter by user ID
- `route` - Filter by route name
- `method` - Filter by HTTP method
- `status` - Filter by response status code
- `since` - Start date (ISO 8601)
- `until` - End date (ISO 8601)
- `page` - Page number
- `limit` - Items per page

**Response:**
```json
{
  "logs": [
    {
      "id": "audit-123",
      "timestamp": "2024-01-15T10:30:00Z",
      "user_id": "user-123",
      "api_key_id": "key-456",
      "route": "users-api",
      "method": "GET",
      "path": "/api/v1/users",
      "status_code": 200,
      "response_time_ms": 45,
      "client_ip": "192.168.1.100",
      "user_agent": "Mozilla/5.0...",
      "request_headers": {
        "Accept": "application/json",
        "Authorization": "***REDACTED***"
      },
      "credits_consumed": 1
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 100,
    "total": 5234
  }
}
```

### Get Audit Log Detail

```http
GET /admin/api/v1/audit-logs/{id}
```

**Response:** Full audit log entry including request/response bodies (if captured)

### Export Audit Logs

```http
GET /admin/api/v1/audit-logs/export?format=jsonl&since=2024-01-01&until=2024-01-15
```

**Query Parameters:**
- `format` - Export format: `json`, `jsonl`, `csv`
- `since` - Start date
- `until` - End date
- `user` - Filter by user
- `route` - Filter by route

**Response:** Exported file download

### Get Audit Statistics

```http
GET /admin/api/v1/audit-logs/stats?since=2024-01-01&until=2024-01-15
```

**Response:**
```json
{
  "total_requests": 52340,
  "unique_users": 150,
  "unique_routes": 12,
  "status_codes": {
    "200": 45000,
    "201": 3000,
    "400": 2000,
    "401": 500,
    "429": 840
  },
  "average_response_time_ms": 45,
  "total_credits_consumed": 52340
}
```

### Cleanup Old Audit Logs

```http
DELETE /admin/api/v1/audit-logs/cleanup?older_than_days=90
```

**Response:**
```json
{
  "deleted_count": 150000,
  "archived_count": 50000
}
```

### Search User Audit Logs

```http
GET /admin/api/v1/users/{id}/audit-logs?page=1&limit=50
```

---

## Analytics

### Analytics Overview

```http
GET /admin/api/v1/analytics/overview?period=24h
```

**Query Parameters:**
- `period` - Time period: `1h`, `24h`, `7d`, `30d`

**Response:**
```json
{
  "period": "24h",
  "total_requests": 52340,
  "successful_requests": 48000,
  "error_requests": 4340,
  "average_latency_ms": 45,
  "p95_latency_ms": 120,
  "p99_latency_ms": 250,
  "requests_per_second": 12.5,
  "total_credits_consumed": 52340,
  "unique_consumers": 150
}
```

### Time Series Data

```http
GET /admin/api/v1/analytics/timeseries?metric=requests&interval=1h&since=2024-01-15T00:00:00Z&until=2024-01-15T23:59:59Z
```

**Query Parameters:**
- `metric` - Metric name: `requests`, `latency`, `errors`, `credits`
- `interval` - Aggregation interval: `1m`, `5m`, `15m`, `1h`, `1d`
- `since` - Start time
- `until` - End time

**Response:**
```json
{
  "metric": "requests",
  "interval": "1h",
  "data": [
    {
      "timestamp": "2024-01-15T00:00:00Z",
      "value": 1200
    },
    {
      "timestamp": "2024-01-15T01:00:00Z",
      "value": 980
    }
  ]
}
```

### Top Routes

```http
GET /admin/api/v1/analytics/top-routes?period=24h&limit=10
```

**Response:**
```json
{
  "routes": [
    {
      "route": "users-api",
      "requests": 15000,
      "avg_latency_ms": 35,
      "error_rate": 0.02,
      "credits_consumed": 15000
    },
    {
      "route": "orders-api",
      "requests": 12000,
      "avg_latency_ms": 55,
      "error_rate": 0.01,
      "credits_consumed": 24000
    }
  ]
}
```

### Top Consumers

```http
GET /admin/api/v1/analytics/top-consumers?period=24h&limit=10
```

**Response:**
```json
{
  "consumers": [
    {
      "user_id": "user-123",
      "name": "John Doe",
      "requests": 5000,
      "credits_consumed": 8000,
      "top_route": "orders-api"
    }
  ]
}
```

### Error Analytics

```http
GET /admin/api/v1/analytics/errors?period=24h&group_by=status_code
```

**Response:**
```json
{
  "total_errors": 4340,
  "error_breakdown": [
    {
      "status_code": 429,
      "count": 2000,
      "percentage": 46.1,
      "top_route": "users-api"
    },
    {
      "status_code": 401,
      "count": 1500,
      "percentage": 34.6,
      "top_route": "orders-api"
    }
  ]
}
```

### Latency Metrics

```http
GET /admin/api/v1/analytics/latency?period=24h&route=users-api
```

**Response:**
```json
{
  "period": "24h",
  "route": "users-api",
  "average_ms": 45,
  "min_ms": 12,
  "max_ms": 500,
  "p50_ms": 38,
  "p95_ms": 120,
  "p99_ms": 250,
  "histogram": [
    { "bucket": "0-10ms", "count": 100 },
    { "bucket": "10-50ms", "count": 8000 },
    { "bucket": "50-100ms", "count": 5000 },
    { "bucket": "100-500ms", "count": 1900 }
  ]
}
```

### Throughput Metrics

```http
GET /admin/api/v1/analytics/throughput?period=24h&interval=1h
```

**Response:**
```json
{
  "period": "24h",
  "total_requests": 52340,
  "peak_rps": 45.5,
  "peak_time": "2024-01-15T14:30:00Z",
  "average_rps": 12.5,
  "by_interval": [
    {
      "timestamp": "2024-01-15T14:00:00Z",
      "requests": 1500,
      "rps": 41.7
    }
  ]
}
```

### Status Code Distribution

```http
GET /admin/api/v1/analytics/status-codes?period=24h
```

**Response:**
```json
{
  "distribution": {
    "2xx": 48000,
    "3xx": 0,
    "4xx": 4340,
    "5xx": 0
  },
  "breakdown": {
    "200": 45000,
    "201": 3000,
    "400": 2000,
    "401": 500,
    "429": 1840
  }
}
```

---

## Billing Configuration

### Get Billing Config

```http
GET /admin/api/v1/billing/config
```

**Response:**
```json
{
  "enabled": true,
  "default_cost": 1,
  "method_multipliers": {
    "GET": 1.0,
    "POST": 1.5,
    "PUT": 1.5,
    "DELETE": 1.0
  },
  "test_mode_enabled": true,
  "zero_balance_action": "reject"
}
```

### Update Billing Config

```http
PUT /admin/api/v1/billing/config
Content-Type: application/json
```

**Request Body:**
```json
{
  "enabled": true,
  "default_cost": 1,
  "method_multipliers": {
    "GET": 1.0,
    "POST": 2.0
  },
  "test_mode_enabled": false,
  "zero_balance_action": "reject"
}
```

### Get Route Costs

```http
GET /admin/api/v1/billing/route-costs
```

**Response:**
```json
{
  "route_costs": {
    "users-api": 1,
    "orders-api": 2,
    "ai-route": 10
  }
}
```

### Update Route Costs

```http
PUT /admin/api/v1/billing/route-costs
Content-Type: application/json
```

**Request Body:**
```json
{
  "route_costs": {
    "users-api": 1,
    "orders-api": 3,
    "premium-api": 5
  }
}
```

---

## GraphQL Federation

### List Subgraphs

```http
GET /admin/api/v1/subgraphs
```

**Response:**
```json
{
  "subgraphs": [
    {
      "id": "users-subgraph",
      "name": "Users Service",
      "url": "http://users-service:4001/graphql",
      "schema": "type User { ... }",
      "status": "active",
      "added_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

### Add Subgraph

```http
POST /admin/api/v1/subgraphs
Content-Type: application/json
```

**Request Body:**
```json
{
  "id": "orders-subgraph",
  "name": "Orders Service",
  "url": "http://orders-service:4002/graphql",
  "schema": "type Order { ... }"
}
```

### Get Subgraph

```http
GET /admin/api/v1/subgraphs/{id}
```

### Remove Subgraph

```http
DELETE /admin/api/v1/subgraphs/{id}
```

### Compose Subgraphs

```http
POST /admin/api/v1/subgraphs/compose
```

**Response:**
```json
{
  "composed": true,
  "supergraph_schema": "...",
  "subgraphs_included": ["users-subgraph", "orders-subgraph"],
  "warnings": []
}
```

---

## Alerts

### List Alert Rules

```http
GET /admin/api/v1/alerts
```

**Response:**
```json
{
  "alerts": [
    {
      "id": "alert-123",
      "name": "High Error Rate",
      "condition": {
        "metric": "error_rate",
        "operator": "greater_than",
        "threshold": 0.05,
        "duration": "5m"
      },
      "enabled": true,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

### Create Alert Rule

```http
POST /admin/api/v1/alerts
Content-Type: application/json
```

**Request Body:**
```json
{
  "name": "High Latency",
  "condition": {
    "metric": "p95_latency",
    "operator": "greater_than",
    "threshold": 500,
    "duration": "10m"
  },
  "enabled": true
}
```

### Update Alert Rule

```http
PUT /admin/api/v1/alerts/{id}
Content-Type: application/json
```

### Delete Alert Rule

```http
DELETE /admin/api/v1/alerts/{id}
```

### Get Alert History

```http
GET /admin/api/v1/alerts/history?since=2024-01-01&limit=50
```

**Response:**
```json
{
  "history": [
    {
      "id": "history-123",
      "alert_id": "alert-123",
      "alert_name": "High Error Rate",
      "status": "firing",
      "value": 0.08,
      "threshold": 0.05,
      "fired_at": "2024-01-15T10:30:00Z",
      "resolved_at": null
    }
  ]
}
```

---

## WebSocket

### Real-time Updates

Connect to the WebSocket endpoint for real-time gateway updates.

```
WS /admin/api/v1/ws
```

**Headers:**
```
X-Admin-Key: your-admin-key
```

### Message Types

**Client -> Server:**
```json
{
  "type": "subscribe",
  "channel": "analytics"
}
```

**Server -> Client:**
```json
{
  "type": "update",
  "channel": "analytics",
  "data": {
    "requests_per_second": 12.5,
    "active_connections": 42
  },
  "timestamp": "2024-01-15T10:30:00Z"
}
```

### Available Channels

- `analytics` - Real-time metrics
- `audit` - Live audit log entries
- `alerts` - Alert notifications
- `health` - Health status changes

---

## Error Handling

All errors follow a consistent format:

```json
{
  "error": "error_code",
  "message": "Human-readable error message",
  "details": {
    "field": "additional context"
  }
}
```

### HTTP Status Codes

| Status | Description |
|--------|-------------|
| 200 OK | Successful GET, PUT |
| 201 Created | Successful POST |
| 204 No Content | Successful DELETE |
| 400 Bad Request | Invalid request body or parameters |
| 401 Unauthorized | Missing or invalid admin key |
| 403 Forbidden | Insufficient permissions |
| 404 Not Found | Resource not found |
| 409 Conflict | Resource conflict (e.g., in use) |
| 429 Too Many Requests | Rate limit exceeded |
| 500 Internal Server Error | Server error |

### Common Error Codes

| Code | Description |
|------|-------------|
| `invalid_payload` | Request body is invalid |
| `invalid_service` | Service validation failed |
| `invalid_route` | Route validation failed |
| `invalid_upstream` | Upstream validation failed |
| `service_not_found` | Service does not exist |
| `route_not_found` | Route does not exist |
| `upstream_not_found` | Upstream does not exist |
| `service_in_use` | Service is referenced by routes |
| `upstream_in_use` | Upstream is referenced by services |
| `user_not_found` | User does not exist |
| `insufficient_credits` | User has insufficient credits |
| `rate_limited` | Too many requests |
| `admin_unauthorized` | Invalid admin key |

---

## Rate Limiting

The Admin API implements rate limiting to prevent abuse:

- **Authenticated requests:** 1000 requests per minute per IP
- **Failed authentication:** 5 attempts per 15 minutes per IP

Rate limit headers are included in all responses:

```http
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 999
X-RateLimit-Reset: 1642245600
```

---

## Pagination

List endpoints support pagination with the following parameters:

- `page` - Page number (default: 1)
- `limit` - Items per page (default: 50, max: 100)

Paginated responses include a `pagination` object:

```json
{
  "data": [...],
  "pagination": {
    "page": 1,
    "limit": 50,
    "total": 150,
    "pages": 3,
    "has_next": true,
    "has_prev": false
  }
}
```

---

## OpenAPI Specification

A complete OpenAPI 3.0 specification is available at:

- File: [`docs/api/openapi.yaml`](./docs/api/openapi.yaml)
- Import into Postman, Swagger UI, or other OpenAPI tools

---

## SDKs and Client Libraries

### cURL Examples

See the [CLI Reference](#) for command-line usage.

### Go Client

```go
package main

import (
    "net/http"
    "time"
)

func main() {
    client := &http.Client{Timeout: 30 * time.Second}
    req, _ := http.NewRequest("GET", "http://localhost:9876/admin/api/v1/status", nil)
    req.Header.Set("X-Admin-Key", "your-admin-key")
    
    resp, err := client.Do(req)
    // Handle response
}
```

### Python Client

```python
import requests

headers = {
    "X-Admin-Key": "your-admin-key",
    "Content-Type": "application/json"
}

response = requests.get(
    "http://localhost:9876/admin/api/v1/status",
    headers=headers
)
print(response.json())
```

---

## Webhooks

Manage event webhooks for real-time notifications.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/admin/api/v1/webhooks` | List all webhooks |
| `POST` | `/admin/api/v1/webhooks` | Create a webhook |
| `GET` | `/admin/api/v1/webhooks/{id}` | Get webhook details |
| `PUT` | `/admin/api/v1/webhooks/{id}` | Update a webhook |
| `DELETE` | `/admin/api/v1/webhooks/{id}` | Delete a webhook |
| `GET` | `/admin/api/v1/webhooks/events` | List webhook event types |
| `GET` | `/admin/api/v1/webhooks/{id}/deliveries` | List delivery attempts |
| `POST` | `/admin/api/v1/webhooks/{id}/test` | Send test event |
| `POST` | `/admin/api/v1/webhooks/{id}/rotate-secret` | Rotate HMAC secret |

**Create webhook:**
```json
POST /admin/api/v1/webhooks
{
  "url": "https://example.com/webhook",
  "events": ["request.completed", "user.created", "credit.low"],
  "secret": "hmac-signing-secret",
  "enabled": true
}
```

Webhook deliveries are signed with HMAC-SHA256. Validate using the `X-Webhook-Signature` header:
```
X-Webhook-Signature: sha256=<hex-hmac>
```

---

## Bulk Operations

Batch CRUD operations for efficient mass updates.

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/admin/api/v1/bulk/services` | Bulk create/update services |
| `POST` | `/admin/api/v1/bulk/routes` | Bulk create/update routes |
| `POST` | `/admin/api/v1/bulk/delete` | Bulk delete resources |
| `POST` | `/admin/api/v1/bulk/plugins` | Bulk plugin configuration |
| `POST` | `/admin/api/v1/bulk/import` | Bulk import from config file |

**Bulk delete:**
```json
POST /admin/api/v1/bulk/delete
{
  "resources": [
    {"type": "route", "id": "route-1"},
    {"type": "service", "id": "svc-2"},
    {"type": "upstream", "id": "up-3"}
  ]
}
```

---

## Advanced Analytics

Predictive and analytical endpoints beyond the basic overview.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/admin/api/v1/analytics/forecast` | Traffic forecasting |
| `GET` | `/admin/api/v1/analytics/anomalies` | Anomaly detection |
| `GET` | `/admin/api/v1/analytics/correlations` | Metric correlations |
| `GET` | `/admin/api/v1/analytics/exports` | Export analytics data |

---

## GraphQL Admin API

Query and mutate gateway configuration using GraphQL.

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/admin/graphql` | Execute GraphQL query/mutation |
| `GET` | `/admin/graphql` | Execute GraphQL query (via URL params) |

**Example query:**
```graphql
{
  services {
    id
    name
    routes {
      id
      name
      paths
    }
  }
}
```

**Example mutation:**
```graphql
mutation {
  createRoute(input: {
    id: "route-new"
    name: "New Route"
    service: "svc-users"
    paths: ["/api/v2/*"]
    methods: ["GET"]
  }) {
    id
    name
  }
}
```

---

## Changelog

### v1.0.0

- Initial stable release
- 100+ Admin API endpoints
- Complete user management with permissions
- Credit system with atomic transactions
- Audit logging with field masking and GZIP compression
- Real-time analytics with WebSocket streaming
- GraphQL Federation support (Apollo-compatible)
- Raft clustering with mTLS
- Webhook system with HMAC signing
- Bulk operations for mass CRUD
- Advanced analytics (forecasting, anomaly detection)
- 20+ plugin pipeline with 5 execution phases
- 11 load balancing algorithms including SubnetAware

---

## Portal API

The User Portal API provides self-service endpoints for API consumers. It runs on a separate port (default 9877) and uses session-based authentication with CSRF protection.

**Base URL:** `http://localhost:9877/portal/api/v1`

### Authentication Model

All Portal endpoints use **session cookie authentication** (`apicerberus_session`). Login sets the session cookie and returns a CSRF token. Subsequent state-changing requests (POST/PUT/DELETE) require the `X-CSRF-Token` header matching the `csrf_token` cookie value.

| Auth Level | Endpoints | Requirement |
|------------|-----------|-------------|
| Public | `POST /auth/login` | None |
| Session | All `GET` endpoints | Valid session cookie |
| Session + CSRF | All `POST`/`PUT`/`DELETE` | Session cookie + `X-CSRF-Token` header |

**Login rate limiting:** 5 failed attempts per IP within 15 minutes triggers a 30-minute block.

### Authentication

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/auth/login` | Login with email/password |
| POST | `/auth/logout` | Logout and clear session |
| GET | `/auth/me` | Get current user profile |
| GET | `/auth/csrf` | Refresh CSRF token |
| PUT | `/auth/password` | Change password |

#### POST /auth/login

```json
// Request
{ "email": "user@example.com", "password": "secret" }

// Response 200
{
  "user": { "id": "...", "email": "...", "name": "...", "role": "...", "credit_balance": 1000 },
  "csrf_token": "abc123",
  "session": { "id": "...", "expires_at": "2026-04-16T10:00:00Z" }
}
```

#### PUT /auth/password

```json
// Request
{ "old_password": "old", "new_password": "new" }

// Response 200
{ "password_changed": true }
```

### API Keys

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api-keys` | List my API keys |
| POST | `/api-keys` | Create new API key |
| PUT | `/api-keys/{id}` | Rename API key |
| DELETE | `/api-keys/{id}` | Revoke API key |

#### GET /api-keys

```json
// Response 200
{
  "items": [
    { "id": "key-1", "name": "Production", "key_prefix": "ck_live_****", "status": "active", "expires_at": null, "last_used_at": "...", "created_at": "..." }
  ],
  "total": 1
}
```

#### POST /api-keys

```json
// Request
{ "name": "Production Key", "mode": "live" }

// Response 201
{
  "token": "ck_live_abcdef123456...",
  "key": { "id": "key-1", "name": "Production Key", "key_prefix": "ck_live_****", "status": "active" }
}
```

> **Note:** The full `token` is only returned once at creation time. Store it securely.

#### PUT /api-keys/{id}

```json
// Request
{ "name": "Renamed Key" }

// Response 200
{ "id": "key-1", "name": "Renamed Key", "renamed": true }
```

### API Catalog

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/apis` | List APIs I have access to |
| GET | `/apis/{routeId}` | Get API route detail |

#### GET /apis

```json
// Response 200
{
  "items": [
    { "route_id": "route-1", "route_name": "Users API", "service_name": "user-svc", "methods": ["GET","POST"], "paths": ["/api/v1/users/*"], "credit_cost": 1, "allowed": true }
  ],
  "total": 1
}
```

#### GET /apis/{routeId}

```json
// Response 200
{
  "route": { "id": "route-1", "name": "Users API", "paths": ["/api/v1/users/*"], "methods": ["GET","POST"], "credit_cost": 1 },
  "service": { "id": "svc-1", "name": "user-svc", "protocol": "http", "upstream": "upstream-1" },
  "permission": { "allowed": true }
}
```

### Playground

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/playground/send` | Send test request through gateway |
| GET | `/playground/templates` | List saved request templates |
| POST | `/playground/templates` | Save/update template |
| DELETE | `/playground/templates/{id}` | Delete template |

#### POST /playground/send

Sends a test request through the gateway using one of your API keys.

```json
// Request
{
  "method": "GET",
  "path": "/api/v1/users",
  "query": { "limit": "10" },
  "headers": { "Accept": "application/json" },
  "body": "",
  "api_key": "ck_live_abcdef...",
  "timeout_ms": 5000
}

// Response 200
{
  "request": { "method": "GET", "url": "http://localhost:8080/api/v1/users?limit=10" },
  "response": { "status_code": 200, "headers": { "Content-Type": "application/json" }, "body": "...", "latency_ms": 45 }
}
```

### Usage & Analytics

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/usage/overview` | Aggregate usage stats |
| GET | `/usage/timeseries` | Time-bucketed usage data |
| GET | `/usage/top-endpoints` | Most-used API endpoints |
| GET | `/usage/errors` | Error breakdown by status code |

**Common query parameters:** `from` (RFC3339), `to` (RFC3339), `window` (Go duration, default `24h`).

#### GET /usage/overview

```json
// Query: ?from=2026-04-14T00:00:00Z&to=2026-04-15T00:00:00Z
// Response 200
{
  "from": "2026-04-14T00:00:00Z",
  "to": "2026-04-15T00:00:00Z",
  "total_requests": 12345,
  "error_requests": 67,
  "error_rate": 0.0054,
  "avg_latency_ms": 42.3,
  "credit_balance": 8500
}
```

#### GET /usage/timeseries

```json
// Query: ?from=...&to=...&granularity=1h
// Response 200
{
  "from": "...", "to": "...", "granularity": "1h",
  "items": [
    { "timestamp": "2026-04-14T10:00:00Z", "requests": 520, "errors": 3, "avg_latency_ms": 38.1 }
  ]
}
```

#### GET /usage/errors

```json
// Response 200
{
  "from": "...", "to": "...",
  "status_map": { "401": 23, "404": 12, "500": 8, "429": 45 }
}
```

### Logs

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/logs` | Search/list audit logs |
| GET | `/logs/{id}` | Get single log entry |
| GET | `/logs/export` | Export logs as JSONL/JSON/CSV |

**Log filter parameters:** `route`, `method`, `client_ip`, `q` (full-text search), `status_min`, `status_max`, `from`, `to`, `limit` (default 50), `offset` (default 0).

#### GET /logs

```json
// Query: ?method=GET&status_min=400&limit=10
// Response 200
{
  "entries": [
    { "id": "log-1", "request_id": "req-1", "method": "GET", "path": "/api/v1/users", "status_code": 401, "user_id": "...", "timestamp": "..." }
  ],
  "total": 234
}
```

#### GET /logs/export

**Query:** filter params + `format` (jsonl|json|csv, default jsonl).

Returns a file download with `Content-Disposition: attachment` header.

### Credits

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/credits/balance` | Current credit balance |
| GET | `/credits/transactions` | Transaction history |
| GET | `/credits/forecast` | Balance depletion forecast |
| POST | `/credits/purchase` | Add credits to account |

#### GET /credits/balance

```json
// Response 200
{ "user_id": "user-1", "balance": 8500 }
```

#### GET /credits/transactions

```json
// Query: ?limit=20&offset=0&type=deduction
// Response 200
{
  "transactions": [
    { "id": "tx-1", "type": "deduction", "amount": -1, "balance_after": 8499, "description": "GET /api/v1/users", "created_at": "..." }
  ],
  "total": 150
}
```

#### GET /credits/forecast

```json
// Response 200
{
  "balance": 8500,
  "average_daily_consumption": 250.5,
  "projected_days_remaining": 33.9,
  "consumption_days_considered": 7
}
```

#### POST /credits/purchase

```json
// Request
{ "amount": 1000, "description": "Monthly top-up" }

// Response 200
{ "purchased": 1000, "new_balance": 9500 }
```

### Security

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/security/ip-whitelist` | List my IP whitelist |
| POST | `/security/ip-whitelist` | Add IP(s) to whitelist |
| DELETE | `/security/ip-whitelist/{ip}` | Remove IP from whitelist |
| GET | `/security/activity` | Recent security activity |

#### GET /security/ip-whitelist

```json
// Response 200
{ "user_id": "user-1", "ips": ["10.0.0.1", "192.168.1.0/24"] }
```

#### POST /security/ip-whitelist

```json
// Request (single IP)
{ "ip": "10.0.0.2" }

// Request (multiple IPs)
{ "ips": ["10.0.0.2", "10.0.0.3"] }

// Response 200
{ "user_id": "user-1", "ips": ["10.0.0.1", "10.0.0.2", "10.0.0.3"] }
```

#### GET /security/activity

```json
// Response 200
{
  "items": [
    { "type": "login", "timestamp": "...", "client_ip": "10.0.0.1", "user_agent": "Mozilla/5.0..." }
  ],
  "total": 42
}
```

### Settings

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/settings/profile` | Get profile settings |
| PUT | `/settings/profile` | Update profile |
| PUT | `/settings/notifications` | Update notification preferences |

#### GET /settings/profile

```json
// Response 200
{
  "user": { "id": "...", "email": "user@example.com", "name": "John Doe", "company": "Acme", "role": "consumer", "credit_balance": 8500 }
}
```

#### PUT /settings/profile

```json
// Request
{ "name": "Jane Doe", "company": "NewCo", "metadata": { "timezone": "US/Eastern" } }

// Response 200
{ "user": { "id": "...", "name": "Jane Doe", "company": "NewCo", ... } }
```

#### PUT /settings/notifications

```json
// Request
{ "notifications": { "credit_low": true, "credit_threshold": 100, "weekly_report": true } }

// Response 200
{ "updated": true, "notifications": { "credit_low": true, "credit_threshold": 100, "weekly_report": true } }
```

### Example Requests

```bash
# Login
curl -X POST http://localhost:9877/portal/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "user@example.com", "password": "secret"}'

# List my API keys (session from login cookie jar)
curl -b "apicerberus_session=<session>" \
  http://localhost:9877/portal/api/v1/api-keys

# Create API key (requires CSRF token from login response)
curl -X POST \
  -H "X-CSRF-Token: <token>" \
  -H "Content-Type: application/json" \
  -b "apicerberus_session=<session>" \
  -d '{"name": "Production Key", "mode": "live"}' \
  http://localhost:9877/portal/api/v1/api-keys

# Send test request via playground
curl -X POST \
  -H "X-CSRF-Token: <token>" \
  -H "Content-Type: application/json" \
  -b "apicerberus_session=<session>" \
  -d '{"method":"GET","path":"/api/v1/users","api_key":"ck_live_..."}' \
  http://localhost:9877/portal/api/v1/playground/send

# Check credit balance
curl -b "apicerberus_session=<session>" \
  http://localhost:9877/portal/api/v1/credits/balance

# Export logs as CSV
curl -b "apicerberus_session=<session>" \
  "http://localhost:9877/portal/api/v1/logs/export?format=csv&from=2026-04-01T00:00:00Z" \
  -o audit_logs.csv
```

---

For more information, visit the [APICerebrus documentation](https://github.com/APICerberus/APICerebrus/tree/main/docs).
