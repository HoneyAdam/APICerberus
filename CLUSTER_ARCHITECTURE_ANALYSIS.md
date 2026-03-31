# APICerebrus Cluster Architecture Deep Dive

## Questions and Analysis

### 1. Is Cluster Load Balancing Possible Without HAProxy?

**Answer: Yes, with 4 different methods:**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    METHOD 1: DNS ROUND ROBIN                                 │
└─────────────────────────────────────────────────────────────────────────────┘

DNS Config:
api.example.com.  IN  A  10.0.0.1
api.example.com.  IN  A  10.0.0.2
api.example.com.  IN  A  10.0.0.3

Client              DNS Server           APICerebrus Nodes
   │                     │                  ┌─────────┐
   │── Query api.local ─▶│                  │ Node 1  │◀── 10.0.0.1
   │◀──── 10.0.0.1 ─────│                  ├─────────┤
   │                     │                  │ Node 2  │◀── 10.0.0.2
   │── HTTP Request ──────────────────────▶│         │
   │                     │                  ├─────────┤
   │                     │                  │ Node 3  │◀── 10.0.0.3
   │                     │                  └─────────┘

✅ Pros: Simple, no extra architecture
❌ Cons: No health checks, failed nodes still receive traffic
❌ Cons: Uneven distribution due to client caching
```

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    METHOD 2: ANYCAST IP (BGP)                                │
└─────────────────────────────────────────────────────────────────────────────┘

                    ┌─────────────┐
                    │   Anycast   │
                    │    IP:      │
                    │  203.0.113.1│
                    └──────┬──────┘
                           │
           ┌───────────────┼───────────────┐
           │               │               │
           ▼               ▼               ▼
     ┌──────────┐    ┌──────────┐    ┌──────────┐
     │  Node 1  │◀──▶│  Node 2  │◀──▶│  Node 3  │
     │ 10.0.0.1 │    │ 10.0.0.2 │    │ 10.0.0.3 │
     │ 203.0.113│    │ 203.0.113│    │ 203.0.113│
     └──────────┘    └──────────┘    └──────────┘

✅ Pros: Automatic failover, routes to nearest node
✅ Pros: Single IP address, global load balancing
❌ Cons: BGP configuration is complex
❌ Cons: Requires data center level configuration
```

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    METHOD 3: CLIENT-SIDE LOAD BALANCING                      │
└─────────────────────────────────────────────────────────────────────────────┘

Client/SDK Layer:
┌────────────────────────────────────────────────────────────────────────────┐
│                         APICerebrus SDK / Client                            │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────────┐    │
│  │ Node List   │  │ Health Check│  │ Load Balance│  │ Circuit Breaker │    │
│  │ - node1:8080│  │ (active)    │  │ (least-conn)│  │ (failover)      │    │
│  │ - node2:8080│  │             │  │             │  │                 │    │
│  │ - node3:8080│  │             │  │             │  │                 │    │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────────┘    │
└────────────────────────────────────────────────────────────────────────────┘

✅ Pros: No extra server-side layer
✅ Pros: Client can perform node health checks
❌ Cons: Every client must use the SDK
❌ Cons: Client code becomes more complex
```

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    METHOD 4: KUBERNETES NATIVE SERVICE                       │
└─────────────────────────────────────────────────────────────────────────────┘

Kubernetes Config:
┌────────────────────────────────────────────────────────────────────────────┐
│  apiVersion: v1                                                            │
│  kind: Service                                                             │
│  metadata:                                                                 │
│    name: apicerberus                                                       │
│  spec:                                                                     │
│    type: LoadBalancer  ← Cloud provider LB                                 │
│    selector:                                                               │
│      app: apicerberus                                                      │
│    ports:                                                                  │
│      - port: 8080                                                          │
│        targetPort: 8080                                                    │
│  ─────────────────────────────────────                                     │
│  apiVersion: apps/v1                                                       │
│  kind: StatefulSet                                                         │
│  spec:                                                                     │
│    serviceName: apicerberus-headless  ← Direct pod access                  │
│    replicas: 3                                                             │
└────────────────────────────────────────────────────────────────────────────┘

✅ Pros: Cloud provider health checks and load balancing
✅ Pros: Kubernetes native, automatic failover
✅ Pros: Easy external DNS integration
```

---

### 2. How Does ACME Let's Encrypt Work in Cluster Mode?

**Current State Analysis:**

```go
// internal/gateway/tls.go
manager.autocertM = &autocert.Manager{
    Prompt: autocert.AcceptTOS,
    Cache:  autocert.DirCache(cfg.ACMEDir),  // ← Local file!
    Email:  cfg.ACMEEmail,
}
```

**Problem:** Each node saves certificates to its own `acme-certs/` directory!

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    PROBLEM: CERTIFICATE SYNCHRONIZATION                      │
└─────────────────────────────────────────────────────────────────────────────┘

Client ──HTTPS──▶ Node 1 (Leader)              Node 2 (Follower)
                     │                              │
                     ▼                              ▼
              ┌─────────────┐                ┌─────────────┐
              │ autocert    │                │ autocert    │
              │ Cache:      │                │ Cache:      │
              │ /data/      │                │ /data/      │
              │  acme/      │                │  acme/      │
              │             │                │             │
              │ cert.pem ✓  │                │ cert.pem ✗  │ ← MISSING!
              └─────────────┘                └─────────────┘

Issue: When client goes to Node 2, certificate doesn't exist, will try to obtain again!
```

**Solution 1: Shared Storage (NFS/EFS/SMB)**

```yaml
# Kubernetes with shared PVC
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: acme-certs
spec:
  accessModes:
    - ReadWriteMany  ← All nodes can read
  resources:
    requests:
      storage: 1Gi

---
apiVersion: apps/v1
kind: StatefulSet
spec:
  template:
    spec:
      containers:
      - name: apicerberus
        volumeMounts:
        - name: acme-certs
          mountPath: /data/acme
      volumes:
      - name: acme-certs
        persistentVolumeClaim:
          claimName: acme-certs
```

**Solution 2: Raft Certificate Synchronization** (Recommended)

```go
// New: internal/gateway/tls_cluster.go

type ClusterCertCache struct {
    raftNode *raft.Node
    localDir string
}

func (c *ClusterCertCache) Get(ctx context.Context, key string) ([]byte, error) {
    // 1. Check local cache first
    if data, err := c.getLocal(key); err == nil {
        return data, nil
    }

    // 2. Get from Raft FSM (cluster state)
    data := c.raftNode.FSM().GetCertificate(key)

    // 3. Save to local cache
    c.saveLocal(key, data)

    return data, nil
}

func (c *ClusterCertCache) Put(ctx context.Context, key string, data []byte) error {
    // Only LEADER obtains certificates!
    if !c.raftNode.IsLeader() {
        return fmt.Errorf("only leader can obtain certificates")
    }

    // Write to Raft log (will propagate to all nodes)
    cmd := &CertCommand{
        Domain: key,
        Cert:   data,
    }
    return c.raftNode.Propose(cmd)
}
```

**Solution 3: Cert-Manager Integration** (Easiest)

```yaml
# Certificate management with cert-manager
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: apicerberus-tls
spec:
  secretName: apicerberus-tls-secret
  issuerRef:
    name: letsencrypt-prod
    kind: ClusterIssuer
  dnsNames:
    - api.example.com
---
# APICerebrus reads this secret
apiVersion: apps/v1
kind: StatefulSet
spec:
  template:
    spec:
      containers:
      - name: apicerberus
        volumeMounts:
        - name: tls-secret
          mountPath: /etc/apicerberus/tls
      volumes:
      - name: tls-secret
        secret:
          secretName: apicerberus-tls-secret
```

---

### 3. How Does Config/Rate Limit Synchronization Work?

**Raft FSM State Machine:**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    FSM STATE SYNCHRONIZATION                                 │
└─────────────────────────────────────────────────────────────────────────────┘

Leader Node                       Follower Nodes
┌─────────────────────┐          ┌─────────────────────┐
│  1. Config Change   │          │                     │
│     (Admin API)     │          │                     │
└─────────┬───────────┘          │                     │
          │                      │                     │
          ▼                      │                     │
┌─────────────────────┐          │                     │
│  2. Raft Log Entry  │          │                     │
│     Propose         │          │                     │
└─────────┬───────────┘          │                     │
          │ AppendEntries RPC    │                     │
          ├─────────────────────▶│                     │
          ├──────────────────────┼────────────────────▶│
          │                      │                     │
          ◀──────────────────────┼────── Ack ─────────┤
          ◀──────────────────────│────── Ack ─────────┤
          │                      │                     │
          ▼                      ▼                     ▼
┌─────────────────────┐          ┌─────────────────────┐
│  3. Commit          │          │   Apply to FSM      │
│     (Majority)      │─────────▶│   (Async)           │
└─────────┬───────────┘          └─────────┬───────────┘
          │                                │
          ▼                                ▼
┌─────────────────────┐          ┌─────────────────────┐
│  4. Gateway Reload  │          │  4. Gateway Reload  │
│     (Hot reload)    │          │     (Hot reload)    │
└─────────────────────┘          └─────────────────────┘

Commit Index: "Committed" when all nodes reach the same log index
```

**Synchronized State:**

```go
// internal/raft/fsm.go
type GatewayFSM struct {
    // ✅ Static Config (Synchronized)
    Routes    map[string]*RouteConfig
    Services  map[string]*ServiceConfig
    Upstreams map[string]*UpstreamConfig

    // ⚠️ Runtime State (Synchronized but carefully!)
    RateLimitCounters map[string]int64    // ← Writing to Raft log on every request is expensive!
    CreditBalances    map[string]int64    // ← Batch updates recommended

    // ✅ Health State (Synchronized)
    HealthChecks      map[string]*HealthStatus

    // ✅ Analytics (Synchronized)
    RequestCounts     map[string]int64
}
```

**Rate Limiting Synchronization Strategies:**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    RATE LIMIT SYNCHRONIZATION STRATEGIES                     │
└─────────────────────────────────────────────────────────────────────────────┘

Strategy 1: Local Rate Limiting (Current)
┌────────────────────────────────────────────────────────────────────────────┐
│ Each node keeps its own local counter                                      │
│ Rate Limit: 100 req/min per consumer                                       │
│ Node 1: 60 req (local)                                                    │
│ Node 2: 70 req (local)  ← TOTAL 130 req, but each node counts separately! │
│                                                                            │
│ ❌ Problem: Real limit should be 100, but can reach 200                   │
└────────────────────────────────────────────────────────────────────────────┘

Strategy 2: Central Rate Limiting
┌────────────────────────────────────────────────────────────────────────────┐
│ Leader node makes all rate limit decisions                                 │
│ Each request requires RPC to Leader                                        │
│                                                                            │
│ ❌ Problem: Single point of contention, increased latency                 │
└────────────────────────────────────────────────────────────────────────────┘

Strategy 3: Gossip Protocol (Recommended)
┌────────────────────────────────────────────────────────────────────────────┐
│ Nodes periodically share rate limit counters                               │
│ Requests in the last minute are approximately known                        │
│                                                                            │
│ ✅ Pros: Eventual consistency, low overhead                               │
│ ⚠️ Trade-off: Approximate rate limiting instead of exact precision        │
└────────────────────────────────────────────────────────────────────────────┘

Strategy 4: External Store (Redis)
┌────────────────────────────────────────────────────────────────────────────┐
│ Rate limit counters stored in Redis                                        │
│ All nodes write to/read from Redis                                         │
│                                                                            │
│ ✅ Pros: Exact rate limiting                                               │
│ ❌ Cons: Redis dependency, extra latency                                  │
└────────────────────────────────────────────────────────────────────────────┘
```

**Current APICerebrus Strategy:**

```go
// Raft FSM has rate limit counters but:
// - Only config changes are written to log
// - Runtime rate limit counters are NOT written to log
// - Because writing to Raft log on every request is too expensive!

// internal/raft/fsm.go
func (f *GatewayFSM) Apply(entry LogEntry) interface{} {
    switch cmd.Type {
    case CmdUpdateRateLimit:
        // Only when admin API updates rate limit
        return f.applyUpdateRateLimit(cmd.Payload)
    }
}

// Runtime rate limiting is local:
// internal/ratelimit/ratelimit.go
type LocalLimiter struct {
    counters map[string]*localCounter  // ← Local memory, no sync!
}
```

---

### 4. Why No HTTP/3? How to Add It?

**Current State:**

```go
// internal/gateway/tls.go
func (tm *TLSManager) TLSConfig() *tls.Config {
    nextProtos := []string{"h2", "http/1.1"}  // ← NO HTTP/3!
    if tm != nil && tm.cfg.Auto {
        nextProtos = append(nextProtos, acme.ALPNProto)
    }
    return &tls.Config{
        MinVersion:     tls.VersionTLS12,
        GetCertificate: tm.GetCertificate,
        NextProtos:     nextProtos,  // h3 not added
    }
}

// internal/gateway/server.go
func (g *Gateway) newHTTPServer(addr string) *http.Server {
    return &http.Server{
        Addr:           addr,
        Handler:        g,
        // ...
        // No quic.Config for HTTP/3!
    }
}
```

**HTTP/3 Integration Plan:**

```go
// NEW: internal/gateway/http3.go

package gateway

import (
    "net/http"

    "github.com/quic-go/quic-go"
    "github.com/quic-go/quic-go/http3"
)

type HTTP3Server struct {
    server *http3.Server
    addr   string
}

func NewHTTP3Server(addr string, handler http.Handler, tlsConfig *tls.Config) (*HTTP3Server, error) {
    // HTTP/3 QUIC configuration
    quicConf := &quic.Config{
        MaxIdleTimeout:        30 * time.Second,
        HandshakeIdleTimeout:  10 * time.Second,
        MaxIncomingStreams:    100,
    }

    // Add h3 to TLS ALPN
    tlsConf := tlsConfig.Clone()
    tlsConf.NextProtos = append([]string{"h3", "h3-29"}, tlsConf.NextProtos...)

    server := &http3.Server{
        Addr:       addr,
        Handler:    handler,
        TLSConfig:  tlsConf,
        QuicConfig: quicConf,
    }

    return &HTTP3Server{
        server: server,
        addr:   addr,
    }, nil
}

func (h *HTTP3Server) Start() error {
    return h.server.ListenAndServe()
}

func (h *HTTP3Server) Shutdown(ctx context.Context) error {
    return h.server.Shutdown(ctx)
}
```

**HTTP/2 and HTTP/3 Together (Upgrade):**

```go
// Add HTTP3 to Gateway struct
type Gateway struct {
    // ... existing fields ...
    httpServer     *http.Server
    httpsServer    *http.Server
    http3Server    *HTTP3Server  // ← NEW
    tlsManager     *TLSManager
}

// ServeHTTP still handles HTTP/1.1 and HTTP/2
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // ... existing code ...
}

// Start method launches HTTP/3
func (g *Gateway) Start(ctx context.Context) error {
    // ... existing HTTP/HTTPS startup ...

    // Start HTTP/3 (if enabled)
    if g.config.Gateway.HTTP3Enabled && g.tlsManager != nil {
        g.http3Server = NewHTTP3Server(
            g.config.Gateway.HTTP3Addr,  // :8443 (UDP!)
            g,
            g.tlsManager.TLSConfig(),
        )
        go func() {
            errCh <- g.http3Server.Start()
        }()
    }

    // ...
}
```

**QUIC/HTTP3 Features:**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    HTTP/3 vs HTTP/2 Comparison                               │
├─────────────────────────────────────────────────────────────────────────────┤
│ Feature          │ HTTP/2           │ HTTP/3 (QUIC)                         │
├──────────────────┼──────────────────┼───────────────────────────────────────┤
│ Transport        │ TCP              │ QUIC (over UDP)                       │
│ Handshake        │ TLS + TCP        │ 0-RTT (from previous connection)      │
│ Head-of-Line     │ Yes (TCP)        │ No (stream independent)               │
│ Connection       │ IP + Port        │ Connection ID (mobility)              │
│ Migration        │ No               │ Yes (WiFi ↔ 4G handover)              │
│ Encryption       │ TLS 1.2+         │ TLS 1.3 (mandatory)                   │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│                    HTTP/3 Protocol Stack                                     │
└─────────────────────────────────────────────────────────────────────────────┘

HTTP/3 (HTTP over QUIC)
        │
QUIC (Quick UDP Internet Connections)
        │
TLS 1.3 (Encryption)
        │
UDP (Transport)
        │
IP (Network)
```

**Why Not Added Yet?**

1. **Go Standard Library**: `net/http` doesn't support HTTP/3 yet (as of Go 1.24)
2. **Third Party Dependency**: Requires external library like `quic-go`
3. **UDP Support**: Cloud providers may have limited UDP load balancing
4. **TLS 1.3 Requirement**: HTTP/3 requires TLS 1.3
5. **Production Maturity**: QUIC/HTTP3 is still evolving

**Steps to Add HTTP/3:**

```bash
# 1. Add dependency
go get github.com/quic-go/quic-go

# 2. Update config (types.go)
type GatewayConfig struct {
    // ... existing ...
    HTTP3Enabled bool   `yaml:"http3_enabled"`
    HTTP3Addr    string `yaml:"http3_addr"`  // Usually :443 UDP
}

# 3. Update Gateway struct

# 4. Update TLS ALPN (add h3)

# 5. Start UDP listener
```

---

## Summary Recommendations

| Question | Answer | Priority |
|----------|--------|----------|
| HAProxy-less Cluster? | ✅ Use DNS Round Robin, Anycast, or K8s Service | High |
| ACME Cluster? | ⚠️ Use Shared PVC or cert-manager | Medium |
| Rate Limit Sync? | ⚠️ Use Gossip protocol or Redis | Medium |
| HTTP/3? | ❌ Requires quic-go integration | Low |

**Quick Wins:**
1. Kubernetes deployment → Automatic load balancing
2. cert-manager integration → Solves ACME problem
3. Gossip protocol → Rate limit synchronization
4. Cloud Load Balancer → Health check + SSL termination
