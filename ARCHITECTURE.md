# APICerebrus Architecture Documentation

## Table of Contents

1.  [Overview](#1-overview)
2.  [High-Level Architecture](#2-high-level-architecture)
3.  [Core Components](#3-core-components)
4.  [Request Lifecycle](#4-request-lifecycle)
5.  [Plugin System](#5-plugin-system)
6.  [Data Storage](#6-data-storage)
7.  [Clustering & Consensus](#7-clustering--consensus)
8.  [GraphQL Federation](#8-graphql-federation)
9.  [Security Architecture](#9-security-architecture)
10. [Configuration Management](#10-configuration-management)
11. [Observability](#11-observability)
12. [Deployment Architecture](#12-deployment-architecture)

---

## 1. Overview

APICerebrus is a cloud-native API Gateway built in Go that provides a comprehensive solution for managing, securing, and observing API traffic. It combines traditional API gateway capabilities with modern features like GraphQL Federation, Raft-based clustering, and MCP (Model Context Protocol) compatibility.

### Key Features

- **Multi-Protocol Support**: HTTP/HTTPS, HTTP/2, gRPC, WebSocket
- **GraphQL Federation**: Schema composition and distributed query execution
- **Raft Consensus**: Distributed state machine for cluster coordination
- **Plugin Architecture**: Extensible request/response processing pipeline
- **Multi-Tenancy**: Consumer-based authentication and rate limiting
- **Billing & Credits**: Usage-based cost tracking and credit management
- **Audit Logging**: Comprehensive request/response auditing with masking
- **MCP Compatible**: Model Context Protocol server implementation

---

## 2. High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              CLIENT LAYER                                    │
│         (Web Browsers, Mobile Apps, Microservices, AI Agents)                │
└─────────────────────────────────────────────────────────────────────────────┘
                                       │
                                       ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           APICEREBRUS GATEWAY                                │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐    │
│  │   HTTP/1.1   │  │   HTTP/2     │  │    gRPC      │  │  WebSocket   │    │
│  │   Server     │  │   Server     │  │   Server     │  │   Server     │    │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘    │
│         └─────────────────┴─────────────────┴─────────────────┘              │
│                                       │                                      │
│                              ┌────────▼────────┐                            │
│                              │  ROUTER ENGINE  │                            │
│                              │  (Radix Tree)   │                            │
│                              └────────┬────────┘                            │
│                                       │                                      │
│                    ┌──────────────────┼──────────────────┐                  │
│                    ▼                  ▼                  ▼                  │
│           ┌─────────────┐   ┌─────────────────┐   ┌─────────────┐          │
│           │   PLUGIN    │   │    FEDERATION   │   │   PROXY     │          │
│           │   PIPELINE  │   │     ENGINE      │   │   ENGINE    │          │
│           │  (Auth, RL) │   │  (GraphQL)      │   │  (HTTP/gRPC)│          │
│           └──────┬──────┘   └────────┬────────┘   └──────┬──────┘          │
│                  │                   │                   │                  │
│                  └───────────────────┼───────────────────┘                  │
│                                      ▼                                      │
│                           ┌─────────────────────┐                           │
│                           │   UPSTREAM POOLS    │                           │
│                           │ (Load Balancers)    │                           │
│                           └──────────┬──────────┘                           │
└──────────────────────────────────────┼──────────────────────────────────────┘
                                       │
                                       ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                            BACKEND SERVICES                                  │
│              (REST APIs, GraphQL Services, gRPC Services)                    │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│                         CONTROL & MANAGEMENT PLANES                          │
│                                                                              │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐  ┌──────────────┐ │
│  │  ADMIN API    │  │   PORTAL      │  │    MCP        │  │    RAFT      │ │
│  │  (REST)       │  │   (Web UI)    │  │  (AI Agents)  │  │  (Consensus) │ │
│  └───────────────┘  └───────────────┘  └───────────────┘  └──────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 3. Core Components

### 3.1 Gateway (`internal/gateway`)

The Gateway is the main HTTP entry point that orchestrates all request processing.

```go
type Gateway struct {
    config         *config.Config          // Runtime configuration
    router         *Router                 // URL routing engine
    proxy          *Proxy                  // Reverse proxy implementation
    health         *Checker                // Health check manager
    store          *store.Store            // SQLite data store
    billing        *billing.Engine         // Credit/billing engine
    auditLogger    *audit.Logger           // Audit logging system
    analytics      *analytics.Engine       // Metrics collection
    upstreams      map[string]*UpstreamPool // Load balancer pools
    authAPIKey     *plugin.AuthAPIKey      // API key authenticator
    routePipelines map[string][]plugin.PipelinePlugin // Plugin chains
    httpServer     *http.Server            // HTTP listener
    httpsServer    *http.Server            // HTTPS listener
    tlsManager     *TLSManager             // TLS certificate manager
    grpcServer     *grpcpkg.H2CServer      // gRPC server
}
```

#### Responsibilities

- **Request Routing**: Matches incoming requests to configured routes using a radix tree
- **Protocol Translation**: HTTP/1.1, HTTP/2, gRPC, WebSocket support
- **Plugin Execution**: Runs request/response through plugin pipelines
- **Load Balancing**: Distributes traffic across upstream targets
- **TLS Termination**: Automatic certificate management via ACME

### 3.2 Router (`internal/gateway/router.go`)

The Router implements a high-performance radix tree (compressed trie) for URL matching.

```
Route Matching Hierarchy:
┌─────────────────────────────────────────────────────────────┐
│  1. Host Matching                                           │
│     ├── Exact host match (api.example.com)                  │
│     └── Wildcard (*.example.com)                           │
│                                                             │
│  2. Method Matching                                         │
│     ├── Specific methods (GET, POST, etc.)                  │
│     └── Any method (*)                                     │
│                                                             │
│  3. Path Matching                                           │
│     ├── Exact match (/api/users)                           │
│     ├── Prefix match (/api/users/*)                        │
│     ├── Path params (/api/users/{id})                      │
│     └── Regex match (/api/v[0-9]+/users)                   │
└─────────────────────────────────────────────────────────────┘
```

**Algorithm Complexity**:
- Time: O(k) where k is the path length
- Space: O(n) where n is the number of routes

### 3.3 Proxy Engine (`internal/gateway/proxy.go`)

The Proxy handles backend communication with advanced features:

- **Connection Pooling**: Reuses HTTP connections to upstreams
- **Retry Logic**: Configurable retry with exponential backoff
- **Circuit Breaking**: Automatic failover on upstream failures
- **Request/Response Transformation**: Header/body modification
- **Streaming Support**: Handles large payloads efficiently

### 3.4 Health Checker (`internal/gateway/health.go`)

Active health monitoring for upstream targets:

```
Health Check Flow:
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│   Target     │────▶│  HTTP Check  │────▶│  Status      │
│   Pool       │     │  (Interval)  │     │  Update      │
└──────────────┘     └──────────────┘     └──────────────┘
                                                │
                    ┌───────────────────────────┘
                    ▼
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│   Healthy    │◀────│  Threshold   │◀────│  Failure     │
│   Target     │     │  (2 success) │     │  Count       │
└──────────────┘     └──────────────┘     └──────────────┘
```

---

## 4. Request Lifecycle

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         REQUEST LIFECYCLE                                   │
└─────────────────────────────────────────────────────────────────────────────┘

Phase 1: ACCEPT
┌────────────────────────────────────────────────────────────────────────────┐
│ 1. Client connects via HTTP/HTTPS/gRPC/WebSocket                           │
│ 2. TLS termination (if applicable)                                         │
│ 3. Security headers added (HSTS, CSP, X-Frame-Options)                     │
│ 4. Request body size validation (MaxBodyBytes)                             │
└────────────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
Phase 2: ROUTE
┌────────────────────────────────────────────────────────────────────────────┐
│ 1. Host header matching                                                    │
│ 2. HTTP method matching                                                    │
│ 3. Path pattern matching (radix tree)                                      │
│ 4. Path parameter extraction                                               │
│ 5. Route configuration lookup                                              │
└────────────────────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┴───────────────┐
              │                               │
              ▼                               ▼
    ┌─────────────────────┐       ┌─────────────────────┐
    │   GraphQL Route     │       │    REST Route       │
    │   (Federation)      │       │   (Proxy)           │
    └──────────┬──────────┘       └──────────┬──────────┘
               │                             │
               └─────────────┬───────────────┘
                             ▼
Phase 3: PLUGIN PIPELINE
┌────────────────────────────────────────────────────────────────────────────┐
│ Phase    │ Plugins                                                        │
├──────────┼────────────────────────────────────────────────────────────────┤
│ PRE_AUTH │ CORS, IP Whitelist, Request Transform                         │
│ AUTH     │ API Key, JWT, OAuth2, mTLS                                    │
│ POST_AUTH│ Rate Limiting, Billing Check, Permission Check                │
│ PRE_PROXY│ Request Mutation, Header Injection                            │
│ POST_PROXY│ Response Transform, Cache, Audit Logging                     │
└────────────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
Phase 4: UPSTREAM
┌────────────────────────────────────────────────────────────────────────────┐
│ 1. Load balancer selects target (Round Robin, Least Conn, etc.)            │
│ 2. Health status check                                                     │
│ 3. Connection pool acquisition                                             │
│ 4. Request proxying with timeouts                                          │
│ 5. Retry logic on failure                                                  │
│ 6. Response capture                                                        │
└────────────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
Phase 5: RESPONSE
┌────────────────────────────────────────────────────────────────────────────┐
│ 1. Post-proxy plugin execution                                             │
│ 2. Audit logging (async)                                                   │
│ 3. Analytics metrics recording                                             │
│ 4. Response streaming to client                                            │
│ 5. Connection cleanup                                                      │
└────────────────────────────────────────────────────────────────────────────┘
```

---

## 5. Plugin System

The plugin system provides a flexible, phase-based request/response processing pipeline.

### 5.1 Plugin Phases

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         PLUGIN PHASES                                        │
└─────────────────────────────────────────────────────────────────────────────┘

Request ──▶ [PRE_AUTH] ──▶ [AUTH] ──▶ [POST_AUTH] ──▶ [PRE_PROXY] ──▶ Proxy
                                                                         │
Response ◀──[POST_PROXY] ◀───────────────────────────────────────────────┘

Phase Order (Priority):
┌────────────────────────────────────────────────────────────────────────────┐
│ Priority │ Phase       │ Example Plugins                                   │
├──────────┼─────────────┼───────────────────────────────────────────────────┤
│    1     │ PRE_AUTH    │ CORS, Bot Detection, IP Filtering                 │
│   10     │ AUTH        │ API Key, JWT, OAuth2, LDAP, mTLS                  │
│   20     │ POST_AUTH   │ Rate Limit, Quota, Billing, ABAC                  │
│   30     │ PRE_PROXY   │ Request Transform, Header Injection, Cache Lookup │
│   40     │ POST_PROXY  │ Response Transform, Cache Store, Audit Log        │
└────────────────────────────────────────────────────────────────────────────┘
```

### 5.2 Built-in Plugins

| Plugin | Phase | Description |
|--------|-------|-------------|
| `auth-apikey` | AUTH | API Key authentication with consumer lookup |
| `auth-jwt` | AUTH | JWT token validation (HS256, RS256) |
| `rate-limit` | POST_AUTH | Token bucket rate limiting per consumer |
| `cors` | PRE_AUTH | Cross-Origin Resource Sharing |
| `billing` | POST_AUTH | Credit deduction and balance check |
| `request-transform` | PRE_PROXY | Header/body modification |
| `response-transform` | POST_PROXY | Response modification |
| `audit` | POST_PROXY | Request/response logging |
| `cache` | PRE/POST_PROXY | HTTP caching layer |

### 5.3 Plugin Architecture

```go
type PipelineContext struct {
    Request        *http.Request      // Incoming request
    ResponseWriter http.ResponseWriter // Response writer
    Response       *http.Response     // Upstream response
    Route          *config.Route      // Matched route
    Service        *config.Service    // Target service
    Consumer       *config.Consumer   // Authenticated consumer
    CorrelationID  string             // Request tracing ID
    Metadata       map[string]any     // Plugin-shared state
    Aborted        bool               // Pipeline abort flag
    Retry          *Retry             // Retry configuration
}

type PipelinePlugin struct {
    name     string
    phase    Phase
    priority int
    run      func(*PipelineContext) (handled bool, err error)
    after    func(*PipelineContext, error)
}
```

---

## 6. Data Storage

### 6.1 Storage Architecture

APICerebrus uses **SQLite** as its primary data store with the following characteristics:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        STORAGE LAYER                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  SQLite Database (WAL Mode)                                         │   │
│  │                                                                     │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌───────────┐ │   │
│  │  │    users    │  │   api_keys  │  │  sessions   │  │ audit_log │ │   │
│  │  ├─────────────┤  ├─────────────┤  ├─────────────┤  ├───────────┤ │   │
│  │  │ id (PK)     │  │ id (PK)     │  │ id (PK)     │  │ id (PK)   │ │   │
│  │  │ email (UQ)  │  │ user_id(FK) │  │ user_id(FK) │  │ timestamp │ │   │
│  │  │ password    │  │ key_hash    │  │ token_hash  │  │ consumer  │ │   │
│  │  │ role        │  │ name        │  │ expires_at  │  │ route     │ │   │
│  │  │ credits     │  │ status      │  │ client_ip   │  │ request   │ │   │
│  │  │ metadata    │  │ expires_at  │  │ user_agent  │  │ response  │ │   │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  └───────────┘ │   │
│  │                                                                     │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                 │   │
│  │  │  credit_txn │  │   alerts    │  │    ip_wh    │                 │   │
│  │  └─────────────┘  └─────────────┘  └─────────────┘                 │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 6.2 Repository Pattern

```go
// Store provides repository access
type Store struct {
    db  *sql.DB
    cfg config.StoreConfig
}

// Repositories
type UserRepository interface {
    Create(user *User) error
    FindByID(id string) (*User, error)
    FindByEmail(email string) (*User, error)
    Update(user *User) error
    Delete(id string) error
    List(offset, limit int) ([]User, error)
}

type SessionRepository interface {
    Create(session *Session) error
    FindByTokenHash(hash string) (*Session, error)
    DeleteByID(id string) error
    Touch(id string) error // Update last seen
}

type APIKeyRepository interface {
    Create(key *APIKey) error
    FindByKeyHash(hash string) (*APIKey, error)
    ListByUser(userID string) ([]APIKey, error)
    Revoke(id string) error
}
```

---

## 7. Clustering & Consensus

APICerebrus implements a **Raft consensus algorithm** for distributed state management.

### 7.1 Raft Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         RAFT CLUSTER                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │                         LEADER NODE                                │   │
│   │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐              │   │
│   │  │  Log Store   │  │  State Mach. │  │  RPC Server  │              │   │
│   │  │  (WAL)       │  │  (FSM)       │  │  (Peers)     │              │   │
│   │  └──────────────┘  └──────────────┘  └──────────────┘              │   │
│   │                                                                      │   │
│   │  Responsibilities:                                                  │   │
│   │  - Accept client write requests                                     │   │
│   │  - Replicate log entries to followers                               │   │
│   │  - Commit entries on majority                                       │   │
│   │  - Send periodic heartbeats                                         │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                                      │                                      │
│                   ┌──────────────────┼──────────────────┐                   │
│                   │                  │                  │                   │
│                   ▼                  ▼                  ▼                   │
│   ┌─────────────────────┐  ┌─────────────────────┐  ┌─────────────────────┐ │
│   │   FOLLOWER NODE 1   │  │   FOLLOWER NODE 2   │  │   FOLLOWER NODE N   │ │
│   │  ┌──────────────┐   │  │  ┌──────────────┐   │  │  ┌──────────────┐   │ │
│   │  │  Log Store   │   │  │  │  Log Store   │   │  │  │  Log Store   │   │ │
│   │  │  (Replica)   │   │  │  │  (Replica)   │   │  │  │  (Replica)   │   │ │
│   │  └──────────────┘   │  │  └──────────────┘   │  │  └──────────────┘   │ │
│   └─────────────────────┘  └─────────────────────┘  └─────────────────────┘ │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 7.2 Consensus Flow

```
Write Operation:
┌────────┐     ┌────────┐     ┌──────────────┐     ┌────────┐
│ Client │────▶│ Leader │────▶│ AppendEntries │────▶│ Follow │
└────────┘     └────┬───┘     │    RPC        │     │  ers   │
                    │         └──────────────┘     └───┬────┘
                    │                    │             │
                    │                    ▼             │
                    │         ┌──────────────┐         │
                    │         │ Majority Ack │◀────────┘
                    │         └──────┬───────┘
                    │                │
                    ▼                ▼
             ┌────────────┐    ┌───────────┐
             │  Commit    │───▶│  Apply    │
             │  Index     │    │  to FSM   │
             └────────────┘    └───────────┘
```

### 7.3 State Machine (FSM)

The FSM applies committed log entries to the gateway state:

```go
type FSM struct {
    state GatewayState
}

func (f *FSM) Apply(entry LogEntry) error {
    switch entry.Command.Type {
    case "service_add":
        return f.addService(entry.Command.Payload)
    case "service_remove":
        return f.removeService(entry.Command.Payload)
    case "route_add":
        return f.addRoute(entry.Command.Payload)
    case "route_remove":
        return f.removeRoute(entry.Command.Payload)
    case "config_update":
        return f.updateConfig(entry.Command.Payload)
    }
}
```

---

## 8. GraphQL Federation

APICerebrus implements Apollo Federation-compatible schema composition and query planning.

### 8.1 Federation Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     GRAPHQL FEDERATION                                      │
└─────────────────────────────────────────────────────────────────────────────┘

                              ┌──────────────────┐
                              │   Supergraph     │
                              │   Schema         │
                              │  (Composed)      │
                              └────────┬─────────┘
                                       │
                    ┌──────────────────┼──────────────────┐
                    ▼                  ▼                  ▼
           ┌────────────────┐ ┌────────────────┐ ┌────────────────┐
           │  User Service  │ │  Order Service │ │ Product Service│
           │  (Subgraph 1)  │ │  (Subgraph 2)  │ │  (Subgraph 3)  │
           ├────────────────┤ ├────────────────┤ ├────────────────┤
           │ type User {    │ │ type Order {   │ │ type Product { │
           │   id: ID!      │ │   id: ID!      │ │   id: ID!      │
           │   name: String │ │   user: User   │ │   name: String │
           │   orders: []   │ │   products: [] │ │   orders: []   │
           │ }              │ │ }              │ │ }              │
           └────────────────┘ └────────────────┘ └────────────────┘

Query Planning:
┌─────────────────────────────────────────────────────────────────────────────┐
│                                                                             │
│  Query: { user(id: "1") { name orders { id products { name } } } }          │
│                                                                             │
│  Plan:                                                                      │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐                    │
│  │ Fetch User  │───▶│ Fetch Orders│───▶│Fetch Products│                   │
│  │ (Subgraph 1)│    │ (Subgraph 2)│    │ (Subgraph 3) │                   │
│  └─────────────┘    └─────────────┘    └─────────────┘                    │
│        │                  │                  │                              │
│        └──────────────────┴──────────────────┘                              │
│                           │                                                 │
│                           ▼                                                 │
│                    ┌─────────────┐                                          │
│                    │   Merge     │                                          │
│                    │   Results   │                                          │
│                    └─────────────┘                                          │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 8.2 Schema Composition

```go
type Composer struct {
    subgraphs map[string]*SubgraphSchema
}

type SubgraphSchema struct {
    Types      map[string]*TypeDefinition
    Entities   map[string]*EntityDefinition
    Resolvers  map[string]*ResolverMap
}

// Compose merges subgraph schemas into supergraph
func (c *Composer) Compose(subgraphs []*Subgraph) (*SupergraphSchema, error) {
    // 1. Validate subgraph schemas
    // 2. Merge types (detect conflicts)
    // 3. Build entity relationships
    // 4. Generate query plan resolvers
    // 5. Output composed SDL
}
```

---

## 9. Security Architecture

### 9.1 Defense in Depth

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         SECURITY LAYERS                                      │
└─────────────────────────────────────────────────────────────────────────────┘

Layer 1: Transport Security
┌────────────────────────────────────────────────────────────────────────────┐
│ - TLS 1.2/1.3 with strong cipher suites                                     │
│ - Automatic certificate management (ACME/Let's Encrypt)                     │
│ - mTLS support for service-to-service authentication                        │
└────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
Layer 2: Edge Security
┌────────────────────────────────────────────────────────────────────────────┐
│ - Security headers (HSTS, CSP, X-Frame-Options, X-Content-Type-Options)     │
│ - CORS policy enforcement                                                   │
│ - Request size limiting (MaxBodyBytes, MaxHeaderBytes)                      │
│ - IP whitelist/blacklist                                                   │
└────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
Layer 3: Authentication
┌────────────────────────────────────────────────────────────────────────────┐
│ - API Key authentication (header/query/cookie)                              │
│ - JWT validation (HS256, RS256) with JWKS support                          │
│ - OAuth2/OIDC integration                                                   │
│ - Rate limiting per credential (brute force protection)                    │
└────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
Layer 4: Authorization
┌────────────────────────────────────────────────────────────────────────────┐
│ - Consumer-based access control                                             │
│ - Endpoint-level permissions                                                │
│ - IP whitelist per consumer                                                 │
│ - Time-based access restrictions                                            │
└────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
Layer 5: Application Security
┌────────────────────────────────────────────────────────────────────────────┐
│ - Input validation and sanitization                                         │
│ - SQL injection prevention (parameterized queries)                          │
│ - XSS protection                                                            │
│ - Sensitive data masking in logs                                            │
└────────────────────────────────────────────────────────────────────────────┘
```

### 9.2 Authentication Flow

```
┌──────────┐     ┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  Client  │────▶│   Gateway    │────▶│    Auth      │────▶│   Store      │
│          │     │              │     │   Plugin     │     │              │
└──────────┘     └──────────────┘     └──────────────┘     └──────────────┘
     │                  │                    │                    │
     │ 1. Request       │                    │                    │
     │─────────────────▶│                    │                    │
     │                  │ 2. Extract         │                    │
     │                  │    Credentials     │                    │
     │                  │───────────────────▶│                    │
     │                  │                    │ 3. Validate        │
     │                  │                    │───────────────────▶│
     │                  │                    │◀───────────────────│
     │                  │                    │ 4. Consumer        │
     │                  │◀───────────────────│    Context         │
     │                  │                    │                    │
     │                  │ 5. Enrich Request  │                    │
     │                  │    with Consumer   │                    │
     │                  │                    │                    │
     │                  │────────┬───────────▶                    │
     │                  │        │ 6. Continue                    │
     │                  │        │    to Upstream                 │
     │                  │        │                                │
     │                  │◀───────┘                                │
     │ 7. Response      │                                         │
     │◀─────────────────│                                         │
```

### 9.3 Security Headers (Implemented)

```go
// addSecurityHeaders adds essential security headers to all responses
func addSecurityHeaders(w http.ResponseWriter, isHTTPS bool) {
    w.Header().Set("X-Content-Type-Options", "nosniff")
    w.Header().Set("X-Frame-Options", "DENY")
    w.Header().Set("X-XSS-Protection", "1; mode=block")
    w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
    w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")
    w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")

    if isHTTPS {
        w.Header().Set("Strict-Transport-Security",
            "max-age=31536000; includeSubDomains; preload")
    }
}
```

---

## 10. Configuration Management

### 10.1 Configuration Hierarchy

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     CONFIGURATION SOURCES                                    │
│                     (Priority: Low to High)                                  │
└─────────────────────────────────────────────────────────────────────────────┘

    ┌────────────────────────────────────────────────────────────────────┐
    │ 1. Default Values                                                  │
    │    Hardcoded sensible defaults in setDefaults()                    │
    └───────────────────────────────┬────────────────────────────────────┘
                                    │
    ┌───────────────────────────────▼────────────────────────────────────┐
    │ 2. Configuration File (YAML)                                       │
    │    apicerberus.yaml - User-defined settings                        │
    └───────────────────────────────┬────────────────────────────────────┘
                                    │
    ┌───────────────────────────────▼────────────────────────────────────┐
    │ 3. Environment Variables                                           │
    │    APICERBERUS_* prefix overrides                                  │
    └───────────────────────────────┬────────────────────────────────────┘
                                    │
    ┌───────────────────────────────▼────────────────────────────────────┐
    │ 4. Runtime Cluster Updates                                         │
    │    Raft consensus propagated changes                               │
    └────────────────────────────────────────────────────────────────────┘
```

### 10.2 Configuration Structure

```yaml
# Example Configuration

# Layer 1: Gateway Settings
gateway:
  http_addr: ":8080"
  https_addr: ":8443"
  read_timeout: 30s
  write_timeout: 30s
  max_body_bytes: 10485760  # 10MB
  tls:
    auto: true
    acme_email: "admin@example.com"

# Layer 2: Management Interfaces
admin:
  addr: ":9876"
  api_key: "${ADMIN_API_KEY}"
  ui_enabled: true

portal:
  enabled: true
  addr: ":9877"
  session:
    secure: true
    max_age: 24h

# Layer 3: Storage
store:
  path: "./data/apicerberus.db"
  journal_mode: WAL

# Layer 4: Clustering
cluster:
  enabled: true
  node_id: "node-1"
  bind_address: ":12000"
  peers:
    - id: "node-2"
      address: "10.0.0.2:12000"

# Layer 5: Routing
services:
  - id: user-service
    upstream: user-pool

routes:
  - id: users-api
    service: user-service
    paths: ["/api/users/*"]
    methods: ["GET", "POST"]

upstreams:
  - id: user-pool
    algorithm: round_robin
    targets:
      - address: "10.0.1.10:8080"
      - address: "10.0.1.11:8080"
```

---

## 11. Observability

### 11.1 Observability Stack

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       OBSERVABILITY SYSTEM                                   │
└─────────────────────────────────────────────────────────────────────────────┘

┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐
│    METRICS       │  │     LOGGING      │  │     TRACING      │
│  (Prometheus)    │  │   (Structured)   │  │  (Correlation)   │
├──────────────────┤  ├──────────────────┤  ├──────────────────┤
│ - Request count  │  │ - Access logs    │  │ - Request ID     │
│ - Latency (p50,  │  │ - Audit logs     │  │ - Span tracing   │
│   p95, p99)      │  │ - Error logs     │  │ - Cross-service  │
│ - Error rate     │  │ - Security logs  │  │   propagation    │
│ - Active conns   │  │                  │  │                  │
│ - Upstream health│  │                  │  │                  │
└────────┬─────────┘  └────────┬─────────┘  └────────┬─────────┘
         │                     │                     │
         └─────────────────────┼─────────────────────┘
                               ▼
                    ┌─────────────────────┐
                    │   Analytics Engine  │
                    │  (Time-series DB)   │
                    └─────────────────────┘
```

### 11.2 Audit Logging

```go
type AuditLog struct {
    ID            string    // Unique log entry ID
    Timestamp     time.Time // Request timestamp
    CorrelationID string    // Request trace ID

    // Client Info
    ClientIP      string
    UserAgent     string
    ConsumerID    string
    ConsumerName  string

    // Request Details
    Method        string
    Path          string
    Host          string
    Headers       map[string]string // Masked
    Body          []byte           // Truncated/Masked

    // Response Details
    StatusCode    int
    ResponseBody  []byte           // Truncated/Masked
    DurationMs    int64

    // Gateway Context
    RouteID       string
    ServiceID     string
    Upstream      string
    Blocked       bool
    BlockReason   string
}
```

### 11.3 Alerting System

```go
type AlertRule struct {
    ID          string
    Name        string
    Condition   AlertCondition
    Threshold   float64
    Duration    time.Duration
    Action      AlertAction
}

type AlertCondition string
const (
    ConditionErrorRate   AlertCondition = "error_rate"
    ConditionLatencyP95  AlertCondition = "latency_p95"
    ConditionRequestRate AlertCondition = "request_rate"
    ConditionUpstreamDown AlertCondition = "upstream_down"
)
```

---

## 12. Deployment Architecture

### 12.1 Single Node Deployment

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         SINGLE NODE SETUP                                    │
└─────────────────────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────────────────────┐
│                              Host Machine                                   │
│  ┌──────────────────────────────────────────────────────────────────────┐  │
│  │                        APICerebrus Process                           │  │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌──────────────┐ │  │
│  │  │  Gateway    │  │   Admin     │  │   Portal    │  │   MCP        │ │  │
│  │  │  :8080      │  │   :9876     │  │   :9877     │  │   :3000      │ │  │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  └──────────────┘ │  │
│  │                                                                        │  │
│  │  ┌─────────────────────────────────────────────────────────────────┐  │  │
│  │  │  SQLite Database (./data/apicerberus.db)                        │  │  │
│  │  └─────────────────────────────────────────────────────────────────┘  │  │
│  └──────────────────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────────────────┘
```

### 12.2 High Availability Deployment

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    HIGH AVAILABILITY CLUSTER                                 │
└─────────────────────────────────────────────────────────────────────────────┘

     ┌──────────────┐
     │   Load       │
     │   Balancer   │
     │  (HAProxy)   │
     └──────┬───────┘
            │
    ┌───────┴───────┐
    │               │
    ▼               ▼
┌────────┐    ┌────────┐
│ Node 1 │◀──▶│ Node 2 │
│ Leader │    │ Follow │
└───┬────┘    └───┬────┘
    │             │
    │    ┌────────┘
    │    │
    ▼    ▼
┌────────┐
│ Node 3 │
│ Follow │
└────────┘

Raft Consensus: Leader election, log replication, configuration changes
Shared Storage: None (each node has local SQLite, replicated via Raft)
```

### 12.3 Kubernetes Deployment

```yaml
# Simplified Kubernetes Deployment

apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: apicerberus
spec:
  serviceName: apicerberus-headless
  replicas: 3
  template:
    spec:
      containers:
      - name: apicerberus
        image: apicerberus/apicerberus:latest
        ports:
        - containerPort: 8080   # Gateway
        - containerPort: 9876   # Admin API
        - containerPort: 12000  # Raft
        volumeMounts:
        - name: data
          mountPath: /data
        env:
        - name: APICERBERUS_CLUSTER_NODE_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: APICERBERUS_CLUSTER_PEERS
          value: "apicerberus-0:12000,apicerberus-1:12000,apicerberus-2:12000"
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: 10Gi
```

---

## 13. Module Dependencies

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      MODULE DEPENDENCY GRAPH                                 │
└─────────────────────────────────────────────────────────────────────────────┘

                          ┌─────────────┐
                          │    cmd/     │
                          │  (main.go)  │
                          └──────┬──────┘
                                 │
          ┌──────────────────────┼──────────────────────┐
          │                      │                      │
          ▼                      ▼                      ▼
   ┌─────────────┐      ┌─────────────┐       ┌─────────────┐
   │    cli/     │      │   gateway/  │       │    mcp/     │
   └──────┬──────┘      └──────┬──────┘       └──────┬──────┘
          │                    │                      │
    ┌─────┴─────┐        ┌─────┴─────┐          ┌─────┴─────┐
    │           │        │           │          │           │
    ▼           ▼        ▼           ▼          ▼           ▼
┌───────┐  ┌───────┐ ┌───────┐  ┌───────┐  ┌───────┐  ┌───────┐
│config/│  │admin/ │ │plugin/│  │store/ │  │admin/ │  │config/│
└───┬───┘  └───┬───┘ └───┬───┘  └───┬───┘  └───┬───┘  └───┬───┘
    │          │         │          │          │          │
    │          │    ┌────┴────┐     │          │          │
    │          │    │         │     │          │          │
    │          │    ▼         ▼     │          │          │
    │          │ ┌────────┐ ┌─────┐ │          │          │
    │          │ │ratelimit│ │auth/│ │          │          │
    │          │ └────────┘ └─────┘ │          │          │
    │          │                    │          │          │
    └──────────┴────────────────────┴──────────┴──────────┘
                    │
                    ▼
            ┌─────────────┐
            │ internal/pkg│
            │  (shared)   │
            └─────────────┘
```

---

## 14. Performance Characteristics

| Metric | Value | Notes |
|--------|-------|-------|
| Request Routing | ~100ns | Radix tree lookup |
| Plugin Overhead | ~1-5µs | Per plugin execution |
| JWT Validation | ~50-100µs | HS256, cached keys |
| Proxy Throughput | >10k RPS | Per core, dependent on payload |
| Memory Usage | ~100MB | Base + per-route overhead |
| Connection Pool | 100/conns | Per upstream, configurable |
| Health Check | 10s interval | Configurable |

---

## 15. Glossary

| Term | Definition |
|------|------------|
| **Consumer** | An authenticated entity (user/app) consuming APIs |
| **Route** | URL pattern mapping to a service |
| **Service** | Logical backend service definition |
| **Upstream** | Load-balanced pool of backend targets |
| **Plugin** | Request/response processing module |
| **Subgraph** | Individual GraphQL service in federation |
| **Supergraph** | Composed schema from multiple subgraphs |
| **FSM** | Finite State Machine (Raft state machine) |
| **MCP** | Model Context Protocol (AI agent protocol) |

---

## 16. References

- [Raft Consensus Paper](https://raft.github.io/raft.pdf)
- [Apollo Federation Spec](https://www.apollographql.com/docs/federation/)
- [Model Context Protocol](https://modelcontextprotocol.io/)
- [Go HTTP Server Patterns](https://golang.org/doc/articles/http_servers.html)

---

*Document Version: 1.0*
*Last Updated: 2026-03-31*
*APICerebrus Version: 1.0.0*