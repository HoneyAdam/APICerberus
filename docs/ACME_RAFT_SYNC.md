# APICerebrus ACME Raft Synchronization

This document explains how ACME certificates can be synchronized between nodes using **Raft consensus** without external shared storage like NFS/EFS.

## Concept

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    RAFT-BASED CERTIFICATE SYNCHRONIZATION                   │
│                                                                             │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │                        RAFT CLUSTER                                  │   │
│   │                                                                      │   │
│   │   ┌─────────────┐                                                   │   │
│   │   │   LEADER    │  1. Get certificate from                         │   │
│   │   │  (Node 1)   │     Let's Encrypt                                │   │
│   │   │             │◄─────────────────────────────────────────────┐   │   │
│   │   │ ┌─────────┐ │                                              │   │   │
│   │   │ │Log:     │ │  2. Replicate as Raft log entry              │   │   │
│   │   │ │[Cert]   │────────────────────┬───────────────────────────┘   │   │
│   │   │ │[Update] │◄───────────────────┼───────────────────┐           │   │   │
│   │   │ │[Renew]  │                    │                   │           │   │   │
│   │   │ └─────────┘                    │                   │           │   │   │
│   │   └──────┬─────────────────────────┘                   │           │   │
│   │          │ Raft AppendEntries                           │           │   │
│   │          │                                              │           │   │
│   │   ┌──────┴──────┐                              ┌──────┴──────┐    │   │
│   │   │  FOLLOWER   │◄─────────────────────────────│  FOLLOWER   │    │   │
│   │   │  (Node 2)   │                              │  (Node 3)   │    │   │
│   │   │             │                              │             │    │   │
│   │   │ ┌─────────┐ │                              │ ┌─────────┐ │    │   │
│   │   │ │Log:     │ │                              │ │Log:     │ │    │   │
│   │   │ │[Cert]   │ │                              │ │[Cert]   │ │    │   │
│   │   │ │[Update] │ │                              │ │[Update] │ │    │   │
│   │   │ │[Renew]  │ │                              │ │[Renew]  │ │    │   │
│   │   │ └────┬────┘ │                              │ └────┬────┘ │    │   │
│   │   │      │      │                              │      │      │    │   │
│   │   │ 3. Write   │                              │ 3. Write   │    │   │
│   │   │  to local  │                              │  to local  │    │   │
│   │   │  disk      │                              │  disk      │    │   │
│   │   └──────┬─────┘                              └──────┬─────┘    │   │
│   └──────────┼───────────────────────────────────────────┼──────────┘   │
│              │                                           │              │
│   ┌──────────┴───────────────────────────────────────────┴──────────┐   │
│   │                     LOCAL DISK (On Each Node)                    │   │
│   │  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐         │   │
│   │  │/data/acme/  │    │/data/acme/  │    │/data/acme/  │         │   │
│   │  │  cert.pem   │    │  cert.pem   │    │  cert.pem   │         │   │
│   │  │  key.pem    │    │  key.pem    │    │  key.pem    │         │   │
│   │  │  (sync)     │    │  (sync)     │    │  (sync)     │         │   │
│   │  └─────────────┘    └─────────────┘    └─────────────┘         │   │
│   └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

## Flow

### 1. Certificate Acquisition (Leader)

```
┌─────────────┐     ┌─────────────┐     ┌─────────────────────────────┐
│   Leader    │────►│   ACME      │────►│  Let's Encrypt              │
│   Node      │     │   Client    │     │                             │
└─────────────┘     └─────────────┘     └─────────────────────────────┘
        │
        │ Certificate received
        ▼
┌─────────────────────────────────────────────────────────────────────┐
│  Raft Log Entry                                                     │
│  ─────────────────                                                  │
│  Type: CERTIFICATE_UPDATE                                           │
│  Data: {                                                            │
│    domain: "api.example.com",                                       │
│    cert: "-----BEGIN CERTIFICATE-----\n...",                        │
│    key: "-----BEGIN PRIVATE KEY-----\n...",                         │
│    issued_at: "2026-03-31T10:00:00Z",                               │
│    expires_at: "2026-06-29T10:00:00Z"                               │
│  }                                                                  │
└─────────────────────────────────────────────────────────────────────┘
```

### 2. Raft Replication

```go
// Raft log entry type
type LogEntryType int

const (
    LogEntryConfigUpdate LogEntryType = iota
    LogEntryRouteAdd
    LogEntryRouteDelete
    LogEntryCertificateUpdate  // ← NEW
    LogEntryAPIKeyRevoke
)

type LogEntry struct {
    Index   uint64
    Term    uint64
    Type    LogEntryType
    Data    []byte
}

// Certificate update log
type CertificateUpdateLog struct {
    Domain     string    `json:"domain"`
    CertPEM    string    `json:"cert_pem"`
    KeyPEM     string    `json:"key_pem"`
    IssuedAt   time.Time `json:"issued_at"`
    ExpiresAt  time.Time `json:"expires_at"`
    IssuedBy   string    `json:"issued_by"`  // Node ID
}
```

### 3. Apply (On All Nodes)

```go
// Raft FSM Apply function
func (f *FSM) Apply(log *raft.Log) interface{} {
    entry := parseLogEntry(log.Data)

    switch entry.Type {
    case LogEntryCertificateUpdate:
        return f.applyCertificateUpdate(entry.Data)
    // ... other cases
    }
}

func (f *FSM) applyCertificateUpdate(data []byte) error {
    var update CertificateUpdateLog
    if err := json.Unmarshal(data, &update); err != nil {
        return err
    }

    // Write to local disk
    certPath := fmt.Sprintf("/data/acme/%s/cert.pem", update.Domain)
    keyPath := fmt.Sprintf("/data/acme/%s/key.pem", update.Domain)

    // Atomic write: write to temp first, then rename
    if err := atomicWriteFile(certPath, []byte(update.CertPEM)); err != nil {
        return err
    }
    if err := atomicWriteFile(keyPath, []byte(update.KeyPEM)); err != nil {
        return err
    }

    // Notify TLS manager about new certificate
    f.tlsManager.ReloadCertificate(update.Domain)

    return nil
}
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            LEADER NODE                                       │
│                                                                              │
│  ┌─────────────────────┐    ┌─────────────────────┐    ┌─────────────────┐  │
│  │   ACME Manager      │    │   Raft Node         │    │   TLS Manager   │  │
│  │                     │    │                     │    │                 │  │
│  │ - Renewal scheduler │    │ - Propose log       │    │ - Cert cache    │  │
│  │ - ACME client       │───►│ - Replicate         │───►│ - Hot reload    │  │
│  │ - Domain tracking   │    │ - Commit            │    │ - SNI handler   │  │
│  └─────────────────────┘    └─────────────────────┘    └─────────────────┘  │
│           │                          │                      │               │
│           │ 1. Get cert              │ 2. Raft log          │ 4. Load       │
│           ▼                          ▼                      ▼               │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                     LOCAL STORAGE                                     │   │
│  │  /data/acme/api.example.com/cert.pem                                 │   │
│  │  /data/acme/api.example.com/key.pem                                  │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
                                       │
                                       │ Raft AppendEntries
                                       ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           FOLLOWER NODE                                     │
│                                                                              │
│  ┌─────────────────────┐    ┌─────────────────────┐    ┌─────────────────┐  │
│  │   ACME Manager      │    │   Raft Node         │    │   TLS Manager   │  │
│  │                     │    │                     │    │                 │  │
│  │ - Renewal scheduler │    │ - Receive log       │    │ - Cert cache    │  │
│  │   (passive)         │    │ - Apply FSM         │───►│ - Hot reload    │  │
│  │                     │    │ - Follower          │    │ - SNI handler   │  │
│  └─────────────────────┘    └─────────────────────┘    └─────────────────┘  │
│                                      │                      │               │
│                                      │ 3. Apply log         │ 4. Load       │
│                                      ▼                      ▼               │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                     LOCAL STORAGE                                     │   │
│  │  /data/acme/api.example.com/cert.pem  ( received from Raft )        │   │
│  │  /data/acme/api.example.com/key.pem   ( received from Raft )        │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

## Renewal Locking (Via Raft)

Instead of Redis lock, **Raft log sequencing** is used:

```go
func (m *ACMEManager) RenewCertificate(domain string) error {
    // Acquire lock via Raft (as log entry)
    lockEntry := &LogEntry{
        Type: LogEntryACMERenewalLock,
        Data: json.Marshal(ACMERenewalLock{
            Domain:   domain,
            NodeID:   m.nodeID,
            Deadline: time.Now().Add(5 * time.Minute),
        }),
    }

    // Propose lock log - only leader can propose
    future := m.raft.Apply(lockEntry, 10*time.Second)
    if err := future.Error(); err != nil {
        return fmt.Errorf("failed to acquire renewal lock: %w", err)
    }

    // Lock successful, get certificate
    cert, key, err := m.acmeClient.FetchCertificate(domain)
    if err != nil {
        return err
    }

    // Replicate certificate as Raft log
    certEntry := &LogEntry{
        Type: LogEntryCertificateUpdate,
        Data: json.Marshal(CertificateUpdateLog{
            Domain:    domain,
            CertPEM:   cert,
            KeyPEM:    key,
            IssuedAt:  time.Now(),
            ExpiresAt: time.Now().Add(90 * 24 * time.Hour),
            IssuedBy:  m.nodeID,
        }),
    }

    future = m.raft.Apply(certEntry, 30*time.Second)
    return future.Error()
}
```

## Configuration

```yaml
# config.yaml
cluster:
  enabled: true
  node_id: "node1"
  raft:
    bind_address: "0.0.0.0:12000"
    peers: ["node2:12000", "node3:12000"]

  # ACME certificate synchronization
  certificate_sync:
    enabled: true
    storage_path: "/data/acme"  # Local path, separate on each node
    raft_replication: true      # Sync via Raft
    # nfs_storage: ""           # Not using NFS

acme:
  enabled: true
  email: "admin@example.com"
  directory_url: "https://acme-v02.api.letsencrypt.org/directory"
  renewal_lock_via_raft: true   # Lock via Raft log sequencing
```

## Docker Compose (No NFS)

```yaml
version: '3.8'

services:
  gateway:
    image: apicerberus/apicerberus:v1.0.0
    deploy:
      mode: replicated
      replicas: 3
    ports:
      - target: 8080
        published: 8080
        mode: ingress
      - target: 8443
        published: 8443
        mode: ingress
    environment:
      - APICERBERUS_NODE_ID={{.Task.Slot}}
      - APICERBERUS_RAFT_ENABLED=true
      - APICERBERUS_RAFT_PEERS=gateway:12000
      - APICERBERUS_CERT_SYNC_RAFT=true  # ← Raft sync enabled
    volumes:
      # LOCAL volume - no NFS!
      # Certificates are synchronized via Raft
      - type: volume
        source: acme-local
        target: /data/acme
    networks:
      - gateway-public
      - raft-cluster

volumes:
  # Local volume on each node
  # Synchronized via Raft
  acme-local:
    driver: local

networks:
  gateway-public:
    driver: overlay
  raft-cluster:
    driver: overlay
    encrypted: true
```

## Advantages

| Feature | With NFS | With Raft |
|---------|----------|-----------|
| **External Dependency** | NFS server required | Only nodes |
| **Network Traffic** | NFS protocol | Raft gRPC (already exists) |
| **Consistency** | Eventual (NFS cache) | Strong (Raft consensus) |
| **Failover** | NFS single point | Raft leader election |
| **Locking** | Redis required | Raft log sequencing |
| **Complexity** | NFS + Redis + App | Just App |

## Disadvantages

1. **Disk Usage**: Certificate copy on each node (small file, not a problem)
2. **Raft Log Size**: Certificates stored in Raft log (truncated with snapshot)
3. **Initial Sync**: New node joins and gets certificates from snapshot

## Summary

**Yes, we can synchronize certificates via Raft!**

- Leader node gets certificate from ACME
- Replicates certificate as Raft log entry
- All followers apply log and write to local disk
- No NFS, EFS, or Redis lock needed
- Each node reads from local disk (fast)
- Strong consistency guarantee from Raft
