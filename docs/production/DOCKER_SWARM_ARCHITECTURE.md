# APICerebrus Docker Swarm Architecture

This document explains how APICerebrus runs in Docker Swarm mode, distributing load without an external load balancer using the routing mesh, and how services communicate within the cluster.

---

## Table of Contents

1. [Overview](#overview)
2. [Docker Swarm Routing Mesh](#docker-swarm-routing-mesh)
3. [Architecture Diagram](#architecture-diagram)
4. [Service Discovery](#service-discovery)
5. [ACME Certificate Synchronization](#acme-certificate-synchronization)
6. [Rate Limiting Synchronization](#rate-limiting-synchronization)
7. [Raft Consensus Network](#raft-consensus-network)
8. [Overlay Network Architecture](#overlay-network-architecture)
9. [Deployment](#deployment)
10. [Scaling](#scaling)

---

## Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         DOCKER SWARM CLUSTER                                │
│                                                                             │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │                    ROUTING MESH (Ingress)                           │   │
│   │                                                                     │   │
│   │   ┌─────────┐    ┌─────────┐    ┌─────────┐    ┌─────────┐         │   │
│   │   │  Node 1 │◄──►│  Node 2 │◄──►│  Node 3 │◄──►│  Node N │         │   │
│   │   │  :8080  │    │  :8080  │    │  :8080  │    │  :8443  │         │   │
│   │   └────┬────┘    └────┬────┘    └────┬────┘    └────┬────┘         │   │
│   │        │              │              │              │              │   │
│   │        └──────────────┴──────────────┴──────────────┘              │   │
│   │                              │                                      │   │
│   │                    ┌─────────┴─────────┐                           │   │
│   │                    │  Load Balancer    │  ◄── IPVS (kernel)        │   │
│   │                    │  (Kernel Space)   │                           │   │
│   │                    └─────────┬─────────┘                           │   │
│   │                              │                                      │   │
│   └──────────────────────────────┼──────────────────────────────────────┘   │
│                                  │                                          │
│   ┌──────────────────────────────┼──────────────────────────────────────┐   │
│   │                              ▼                                      │   │
│   │                    ┌─────────────────┐                             │   │
│   │                    │ Gateway Tasks   │                             │   │
│   │                    │ (Containers)    │                             │   │
│   │                    └────────┬────────┘                             │   │
│   │                             │                                      │   │
│   │        ┌────────────────────┼────────────────────┐                │   │
│   │        │                    │                    │                │   │
│   │        ▼                    ▼                    ▼                │   │
│   │   ┌─────────┐         ┌─────────┐         ┌─────────┐            │   │
│   │   │Task 1   │◄───────►│Task 2   │◄───────►│Task 3   │            │   │
│   │   │(Node 1) │  Raft   │(Node 2) │  Raft   │(Node 3) │            │   │
│   │   └────┬────┘         └────┬────┘         └────┬────┘            │   │
│   │        │                   │                   │                 │   │
│   │        └───────────────────┴───────────────────┘                 │   │
│   │                        │                                         │   │
│   │                        ▼                                         │   │
│   │               ┌─────────────────┐                                │   │
│   │               │ Shared Storage  │  ◄── ACME Certs               │   │
│   │               │   (NFS/EFS)     │                                │   │
│   │               └─────────────────┘                                │   │
│   │                                                                  │   │
│   └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘
```

---

## Docker Swarm Routing Mesh

### How It Works Without HAProxy

Docker Swarm's **Routing Mesh** feature eliminates the need for an external load balancer (HAProxy, NGINX):

```
Traditional (with HAProxy):
┌─────────────┐      ┌─────────────┐      ┌─────────┐
│   Client    │─────►│   HAProxy   │─────►│ Gateway │
└─────────────┘      └─────────────┘      └─────────┘
                           │
                    (Single point of failure)

Docker Swarm (Routing Mesh):
┌─────────────┐      ┌─────────────────────────────────────┐
│   Client    │─────►│   ANY NODE (8080/8443)              │
└─────────────┘      │   Swarm Load Balancer (IPVS)        │
                     │   Kernel Space                       │
                     └──────────────┬──────────────────────┘
                                    │
                           ┌────────┴────────┐
                           ▼                 ▼
                    ┌────────────┐    ┌────────────┐
                    │  Task 1    │    │  Task 2    │
                    │  (Node 1)  │    │  (Node 2)  │
                    └────────────┘    └────────────┘
```

### Routing Mesh Features

1. **Every Node Listens**: Every node in the cluster listens on published ports
2. **Kernel Space Load Balancing**: Uses IPVS (IP Virtual Server) - very fast
3. **VIP (Virtual IP)**: Creates a virtual IP for each service
4. **Iptables/IPVS Integration**: Automatically routes incoming traffic

```
┌────────────────────────────────────────────────────────────┐
│                        CLIENT                              │
└──────────────────────────┬─────────────────────────────────┘
                           │ DNS: any-node.swarm.local:8080
                           ▼
┌────────────────────────────────────────────────────────────┐
│                    DOCKER SWARM NODE                       │
│  ┌──────────────────────────────────────────────────────┐  │
│  │              Ingress Network (Overlay)               │  │
│  │                                                      │  │
│  │   ┌─────────────────────────────────────────────┐   │  │
│  │   │         IPVS Load Balancer                  │   │  │
│  │   │  ┌─────────┐    ┌─────────┐    ┌─────────┐  │   │  │
│  │   │  │ Backend │    │ Backend │    │ Backend │  │   │  │
│  │   │  │ Task 1  │    │ Task 2  │    │ Task 3  │  │   │  │
│  │   │  │ 10.0.0.5│    │ 10.0.0.6│    │ 10.0.0.7│  │   │  │
│  │   │  └─────────┘    └─────────┘    └─────────┘  │   │  │
│  │   └─────────────────────────────────────────────┘   │  │
│  │                                                      │  │
│  └──────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────┘
```

---

## Architecture Diagram

### Full Stack Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           EXTERNAL NETWORK                                  │
│                                                                             │
│   ┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌────────────┐  │
│   │   Client 1  │    │   Client 2  │    │   Client 3  │    │  Let's     │  │
│   │             │    │             │    │             │    │  Encrypt   │  │
│   └──────┬──────┘    └──────┬──────┘    └──────┬──────┘    └─────┬──────┘  │
│          │                  │                  │                 │         │
│          └──────────────────┴──────────────────┘                 │         │
│                             │                                    │         │
└─────────────────────────────┼────────────────────────────────────┼─────────┘
                              │                                    │
                              ▼                                    │
┌─────────────────────────────────────────────────────────────────────────────┐
│                         DOCKER SWARM CLUSTER                                │
│                                                                             │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │                    INGRESS NETWORK (Routing Mesh)                      │ │
│  │                                                                        │ │
│  │    Port 8080 ──────┐                                                   │ │
│  │    Port 8443 ──────┼──► IPVS Load Balancer ──► Gateway Service Tasks │ │
│  │                    │                                                   │ │
│  └────────────────────┼───────────────────────────────────────────────────┘ │
│                       │                                                     │
│  ┌────────────────────┼───────────────────────────────────────────────────┐ │
│  │                    ▼                                                   │ │
│  │  ┌─────────────────────────────────────────────────────────────────┐   │ │
│  │  │                      GATEWAY SERVICE                            │   │ │
│  │  │                      (3 replicas)                               │   │ │
│  │  │                                                                 │   │ │
│  │  │   ┌───────────────┐    ┌───────────────┐    ┌───────────────┐   │   │ │
│  │  │   │   Gateway     │    │   Gateway     │    │   Gateway     │   │   │ │
│  │  │   │   Replica 1   │◄──►│   Replica 2   │◄──►│   Replica 3   │   │   │ │
│  │  │   │               │    │               │    │               │   │   │ │
│  │  │   │ ┌───────────┐ │    │ ┌───────────┐ │    │ ┌───────────┐ │   │   │ │
│  │  │   │ │ HTTP/8080 │ │    │ │ HTTP/8080 │ │    │ │ HTTP/8080 │ │   │   │ │
│  │  │   │ │HTTPS/8443 │ │    │ │HTTPS/8443 │ │    │ │HTTPS/8443 │ │   │   │ │
│  │  │   │ │  Admin    │ │    │ │  Admin    │ │    │ │  Admin    │ │   │   │ │
│  │  │   │ │  Portal   │ │    │ │  Portal   │ │    │ │  Portal   │ │   │   │ │
│  │  │   │ └─────┬─────┘ │    │ └─────┬─────┘ │    │ └─────┬─────┘ │   │   │ │
│  │  │   │       │       │    │       │       │    │       │       │   │   │ │
│  │  │   │   Raft:12000  │◄──►│   Raft:12000  │◄──►│   Raft:12000  │   │   │ │
│  │  │   │       │       │    │       │       │    │       │       │   │   │ │
│  │  │   └───────┼───────┘    └───────┼───────┘    └───────┼───────┘   │   │ │
│  │  │           │                    │                    │           │   │ │
│  │  └───────────┼────────────────────┼────────────────────┼───────────┘   │ │
│  │              │                    │                    │               │ │
│  │              └────────────────────┼────────────────────┘               │ │
│  │                                   │                                    │ │
│  │                    ┌──────────────┴──────────────┐                     │ │
│  │                    │      RAFT CONSENSUS         │                     │ │
│  │                    │      (Encrypted Overlay)    │                     │ │
│  │                    └─────────────────────────────┘                     │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │                         BACKEND NETWORK                                │ │
│  │                                                                        │ │
│  │   ┌───────────────┐    ┌───────────────┐    ┌───────────────┐         │ │
│  │   │   PostgreSQL  │    │    Redis      │    │   Prometheus  │         │ │
│  │   │   (Shared DB) │    │   (Cache)     │    │   (Metrics)   │         │ │
│  │   │               │    │               │    │               │         │ │
│  │   │ • Users       │    │ • Sessions    │    │ • Gateway     │         │ │
│  │   │ • API Keys    │    │ • Rate Limits │    │   metrics     │         │ │
│  │   │ • Routes      │    │ • ACME locks  │    │ • Node stats  │         │ │
│  │   │ • Configs     │    │               │    │               │         │ │
│  │   └───────────────┘    └───────────────┘    └───────────────┘         │ │
│  │                                                                        │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │                       SHARED STORAGE (NFS/EFS)                         │ │
│  │                                                                        │ │
│  │   ┌─────────────────────────────────────────────────────────────────┐  │ │
│  │   │                    ACME CERTIFICATES                            │  │ │
│  │   │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐             │  │ │
│  │   │  │cert1.pem    │  │cert2.pem    │  │private.key  │             │  │ │
│  │   │  │fullchain.pem│  │fullchain.pem│  │             │             │  │ │
│  │   │  │             │  │             │  │             │             │  │ │
│  │   │  │ domain1.com │  │ domain2.com │  │  account.key│             │  │ │
│  │   │  └─────────────┘  └─────────────┘  └─────────────┘             │  │ │
│  │   └─────────────────────────────────────────────────────────────────┘  │ │
│  │                                                                        │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Service Discovery

Docker Swarm uses DNS-based service discovery. Each service is accessible by its name.

### DNS-Based Service Discovery

```
┌─────────────────────────────────────────────────────────────────┐
│                     DOCKER SWARM DNS                            │
│                                                                 │
│   Service Name ──────► IP Addresses (Round Robin)               │
│                                                                 │
│   "gateway"  ───────► 10.0.1.5, 10.0.1.6, 10.0.1.7              │
│   "postgres" ───────► 10.0.2.3                                  │
│   "redis"    ───────► 10.0.2.4                                  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Task (Replica) Specific DNS

```
┌─────────────────────────────────────────────────────────────────┐
│                 TASK-SPECIFIC DNS                               │
│                                                                 │
│   Task Name                    ──────► IP Address               │
│   ───────────────────────────────────────────────────────────── │
│   gateway.1.asd123.apicerberus ──────► 10.0.1.5                 │
│   gateway.2.qwe456.apicerberus ──────► 10.0.1.6                 │
│   gateway.3.zxc789.apicerberus ──────► 10.0.1.7                 │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Raft Peer Discovery

```go
// Raft peers are automatically discovered in Docker Swarm
peers := []string{
    "gateway:12000",        // Load balances to all replicas
    "gateway.1:12000",      // Only replica 1
    "gateway.2:12000",      // Only replica 2
    "gateway.3:12000",      // Only replica 3
}
```

---

## ACME Certificate Synchronization

### The Problem

If each node requests its own ACME certificate:
- Rate limit exceeded (Let's Encrypt limits)
- Certificate mismatches
- Different nodes may serve different certificates

### Solution: Shared Storage

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         ACME CERTIFICATE FLOW                               │
│                                                                             │
│   ┌─────────────┐                                                           │
│   │ Let's Encrypt│◄─────────────────────────────────────────────────────┐   │
│   └──────┬──────┘                                                      │   │
│          │ 1. Certificate Request                                       │   │
│          │                                                             │   │
│          ▼                                                             │   │
│   ┌─────────────────────────────────────────────────────────────────┐   │   │
│   │                    NFS/EFS SHARED STORAGE                        │   │   │
│   │  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐         │   │   │
│   │  │/data/acme/  │    │/data/acme/  │    │/data/acme/  │         │   │   │
│   │  │domain1.com/ │    │domain2.com/ │    │accounts/    │         │   │   │
│   │  │  cert.pem   │    │  cert.pem   │    │  account.key│         │   │   │
│   │  │  key.pem    │    │  key.pem    │    │             │         │   │   │
│   │  └─────────────┘    └─────────────┘    └─────────────┘         │   │   │
│   └──────┬────────────────┬────────────────┬───────────────────────┘   │   │
│          │                │                │                            │   │
│          │ 2. Mount       │ 2. Mount       │ 2. Mount                   │   │
│          ▼                ▼                ▼                            │   │
│   ┌─────────────┐   ┌─────────────┐   ┌─────────────┐                   │   │
│   │  Gateway    │   │  Gateway    │   │  Gateway    │  3. All serve     │   │
│   │  Replica 1  │   │  Replica 2  │   │  Replica 3  │     same cert     │   │
│   │             │   │             │   │             │                   │   │
│   │  Leader:    │   │  Follower:  │   │  Follower:  │                   │   │
│   │  Requests   │   │  Reads certs│   │  Reads certs│                   │   │
│   │  & Renews   │   │             │   │             │                   │   │
│   └─────────────┘   └─────────────┘   └─────────────┘                   │   │
│                                                                             │
│   Flow:                                                                     │
│   1. Leader replica requests certificate from Let's Encrypt               │
│   2. Certificate stored in shared NFS/EFS volume                          │
│   3. All replicas read from same shared volume                            │
│   4. All serve identical certificate                                      │
└─────────────────────────────────────────────────────────────────────────────┘
```

### ACME Renewal Locking

```
┌─────────────────────────────────────────────────────────────────┐
│                 ACME RENEWAL WITH REDIS LOCK                     │
│                                                                 │
│   ┌─────────────┐                                               │
│   │   Redis     │◄───────────────────────────────────────┐      │
│   │   (Lock)    │                                        │      │
│   │             │  SET acme:renew:lock EX 300 NX         │      │
│   └──────┬──────┘                                        │      │
│          │                                               │      │
│          │                                               │      │
│   ┌──────┴───────────────────────────────────────────┐   │      │
│   │              GATEWAY REPLICAS                     │   │      │
│   │                                                   │   │      │
│   │   ┌─────────┐    ┌─────────┐    ┌─────────┐      │   │      │
│   │   │Try Lock │    │Try Lock │    │Try Lock │      │   │      │
│   │   │ ✓ Got it│    │ ✗ Failed│    │ ✗ Failed│      │   │      │
│   │   │         │    │         │    │         │      │   │      │
│   │   │Renews   │    │Waits    │    │Waits    │      │   │      │
│   │   │Cert     │    │         │    │         │      │   │      │
│   │   │         │    │         │    │         │      │   │      │
│   │   │Releases │    │Checks   │    │Checks   │      │   │      │
│   │   │Lock     │    │Cert Age │    │Cert Age │      │   │      │
│   │   └─────────┘    └─────────┘    └─────────┘      │   │      │
│   │                                                   │   │      │
│   └───────────────────────────────────────────────────┘   │      │
│                                                           │      │
│   Only one replica renews, others use existing cert       │      │
└───────────────────────────────────────────────────────────┴──────┘
```

---

## Rate Limiting Synchronization

### Strategies

| Strategy | Advantage | Disadvantage | Use Case |
|----------|-----------|--------------|----------|
| **Local** | Fast, simple | Per-node limits | Default |
| **Redis** | Global, accurate | Network latency | Recommended |
| **Gossip** | Decentralized | Eventual consistency | Optional |

### Redis-Based Rate Limiting

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     GLOBAL RATE LIMITING WITH REDIS                         │
│                                                                             │
│   Client Request                                                            │
│        │                                                                    │
│        ▼                                                                    │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │                    DOCKER SWARM CLUSTER                              │   │
│   │                                                                      │   │
│   │   ┌───────────────┐    ┌───────────────┐    ┌───────────────┐       │   │
│   │   │   Gateway 1   │    │   Gateway 2   │    │   Gateway 3   │       │   │
│   │   │               │    │               │    │               │       │   │
│   │   │ 1. Check Rate │    │ 1. Check Rate │    │ 1. Check Rate │       │   │
│   │   │    Limit      │    │    Limit      │    │    Limit      │       │   │
│   │   │       │       │    │       │       │    │       │       │       │   │
│   │   │       ▼       │    │       ▼       │    │       ▼       │       │   │
│   │   │  INCR user:123│    │  INCR user:123│    │  INCR user:123│       │   │
│   │   │  EXPIRE 60    │    │  EXPIRE 60    │    │  EXPIRE 60    │       │   │
│   │   │       │       │    │       │       │    │       │       │       │   │
│   │   └───────┼───────┘    └───────┼───────┘    └───────┼───────┘       │   │
│   │           │                    │                    │               │   │
│   │           └────────────────────┼────────────────────┘               │   │
│   │                                │                                    │   │
│   │                                ▼                                    │   │
│   │                    ┌─────────────────────┐                          │   │
│   │                    │       Redis         │                          │   │
│   │                    │   ┌─────────────┐   │                          │   │
│   │                    │   │ user:123    │   │                          │   │
│   │                    │   │ count: 45   │   │                          │   │
│   │                    │   │ TTL: 32s    │   │                          │   │
│   │                    │   └─────────────┘   │                          │   │
│   │                    │                     │                          │   │
│   │                    │   Rate: 100 req/min │                          │   │
│   │                    │   Current: 45       │                          │   │
│   │                    │   Remaining: 55     │                          │   │
│   │                    └─────────────────────┘                          │   │
│   │                                                                      │   │
│   └──────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│   Result: Global rate limit across all nodes                                │
│   - User 123 has made 45 requests in current window                         │
│   - All gateways see the same count                                         │
│   - Consistent enforcement regardless of which node handles request         │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Raft Consensus Network

### Raft Overlay Network

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      RAFT CONSENSUS NETWORK                                 │
│                                                                             │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │                  raft-cluster (Overlay Network)                      │   │
│   │                    Encrypted: true                                   │   │
│   │                    Internal: true (no external access)               │   │
│   │                                                                      │   │
│   │   ┌─────────────────────────────────────────────────────────────┐   │   │
│   │   │                     RAFT CLUSTER                             │   │   │
│   │   │                                                              │   │   │
│   │   │    ┌─────────────┐                                          │   │   │
│   │   │    │   Leader    │◄────────────────────────────────────┐    │   │   │
│   │   │    │  (Gateway 1)│                                     │    │   │   │
│   │   │    │             │    AppendEntries (heartbeat)        │    │   │   │
│   │   │    │ ┌─────────┐ │◄────────────────────────────────────┘    │   │   │
│   │   │    │ │Log: [1] │ │                                          │   │   │
│   │   │    │ │    [2]  │ │    ┌─────────────┐    ┌─────────────┐   │   │   │
│   │   │    │ │    [3]  │◄───►│  Follower   │◄──►│  Follower   │   │   │   │
│   │   │    │ │    [4]  │◄───►│  (Gateway 2)│◄───►│  (Gateway 3)│   │   │   │
│   │   │    │ │    [5]  │ │    │             │    │             │   │   │   │
│   │   │    │ └─────────┘ │    │ ┌─────────┐ │    │ ┌─────────┐ │   │   │   │
│   │   │    │             │    │ │Log: [1] │ │    │ │Log: [1] │ │   │   │   │
│   │   │    │  Commit: 3  │    │ │    [2]  │ │    │ │    [2]  │ │   │   │   │
│   │   │    │  Applied: 3 │    │ │    [3]  │ │    │ │    [3]  │ │   │   │   │
│   │   │    └─────────────┘    │ └─────────┘ │    │ └─────────┘ │   │   │   │
│   │   │                       └─────────────┘    └─────────────┘   │   │   │
│   │   │                                                              │   │   │
│   │   │   Log Entries:                                               │   │   │
│   │   │   [1] ConfigUpdate: rate_limits updated                      │   │   │
│   │   │   [2] RouteAdded: /api/v1/users → user-service               │   │   │
│   │   │   [3] CertificateRenewed: api.example.com                    │   │   │
│   │   │   [4] APIKeyRevoked: key_abc123                              │   │   │
│   │   │   [5] (Uncommitted)                                          │   │   │
│   │   │                                                              │   │   │
│   │   └──────────────────────────────────────────────────────────────┘   │   │
│   │                                                                      │   │
│   └──────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│   Raft Network: Isolated from public network                                │
│   - Only cluster nodes can communicate                                      │
│   - IPSec encrypted overlay                                                 │
│   - No external access allowed (internal: true)                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Overlay Network Architecture

### Network Stack

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         NETWORK ARCHITECTURE                                │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────────┐│
│  │                        EXTERNAL TRAFFIC                                ││
│  │                                                                         ││
│  │   HTTP  ──────┐                                                         ││
│  │   HTTPS ──────┼──►  INGRESS NETWORK (gateway-public)                   ││
│  │               │       - Routing Mesh enabled                           ││
│  │               │       - Published ports: 8080, 8443                    ││
│  │               │       - Load balanced by IPVS                          ││
│  │               │                                                         ││
│  └───────────────┼─────────────────────────────────────────────────────────┘│
│                  │                                                          │
│  ┌───────────────┼─────────────────────────────────────────────────────────┐│
│  │               ▼                                                         ││
│  │  ┌─────────────────────────────────────────────────────────────────┐   ││
│  │  │                    GATEWAY SERVICE                               │   ││
│  │  │  Connected to: gateway-public, raft-cluster, backend             │   ││
│  │  └─────────────────────────────────────────────────────────────────┘   ││
│  │                                                                         ││
│  └─────────────────────────────────────────────────────────────────────────┘│
│                  │                                                          │
│                  │ Raft Communication                                        │
│                  ▼                                                          │
│  ┌─────────────────────────────────────────────────────────────────────────┐│
│  │                                                                         ││
│  │              RAFT CLUSTER NETWORK (raft-cluster)                       ││
│  │              - Encrypted: true                                         ││
│  │              - Internal: true (isolated)                               ││
│  │              - Port: 12000                                             ││
│  │                                                                         ││
│  └─────────────────────────────────────────────────────────────────────────┘│
│                  │                                                          │
│                  │ Database/Cache Queries                                   │
│                  ▼                                                          │
│  ┌─────────────────────────────────────────────────────────────────────────┐│
│  │                                                                         ││
│  │              BACKEND NETWORK (backend)                                 ││
│  │              - Services: PostgreSQL, Redis                              ││
│  │              - Encrypted: true                                          ││
│  │              - Internal services only                                   ││
│  │                                                                         ││
│  └─────────────────────────────────────────────────────────────────────────┘│
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Encrypted Overlay

```yaml
networks:
  raft-cluster:
    driver: overlay
    encrypted: true  # IPSec encryption
    internal: true   # No external access
```

---

## Deployment

### 1. Initialize Swarm

```bash
# Initialize Docker Swarm (on manager node)
docker swarm init --advertise-addr <MANAGER-IP>

# Get join token for workers
docker swarm join-token worker

# On worker nodes:
docker swarm join --token <TOKEN> <MANAGER-IP>:2377
```

### 2. Create Secrets (Optional)

```bash
# Create secrets for sensitive data
echo "my-jwt-secret" | docker secret create jwt_secret -
echo "my-admin-password" | docker secret create admin_password -
echo "my-db-password" | docker secret create db_password -
```

### 3. Deploy Stack

```bash
# Deploy the stack
docker stack deploy -c docker-compose.swarm.yml apicerberus

# Check status
docker stack ps apicerberus
docker service ls
```

---

## Scaling

### Horizontal Scaling

```bash
# Scale gateway to 5 replicas
docker service scale apicerberus_gateway=5

# Or update service
docker service update --replicas 5 apicerberus_gateway
```

### Zero-Downtime Deployment

```yaml
deploy:
  update_config:
    parallelism: 1      # Update 1 replica at a time
    delay: 30s          # Wait 30s between updates
    failure_action: rollback
    order: start-first  # Start new before stopping old
```

### Scaling Behavior

```
Before Scaling (3 replicas):
┌─────────────────────────────────────────────┐
│              INGRESS NETWORK                │
│                                             │
│   ┌─────────┐  ┌─────────┐  ┌─────────┐    │
│   │ Node 1  │  │ Node 2  │  │ Node 3  │    │
│   │  :8080  │  │  :8080  │  │  :8080  │    │
│   └────┬────┘  └────┬────┘  └────┬────┘    │
│        │            │            │         │
│        └────────────┼────────────┘         │
│                     │                      │
│              ┌──────┴──────┐               │
│              │   IPVS LB   │               │
│              └──────┬──────┘               │
│                     │                      │
│        ┌────────────┼────────────┐         │
│        ▼            ▼            ▼         │
│   ┌─────────┐  ┌─────────┐  ┌─────────┐    │
│   │ Task 1  │  │ Task 2  │  │ Task 3  │    │
│   └─────────┘  └─────────┘  └─────────┘    │
└─────────────────────────────────────────────┘

After Scaling (5 replicas):
┌─────────────────────────────────────────────────────────┐
│                    INGRESS NETWORK                      │
│                                                         │
│   ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐   │
│   │ Node 1  │  │ Node 2  │  │ Node 3  │  │ Node 4  │   │
│   │  :8080  │  │  :8080  │  │  :8080  │  │  :8080  │   │
│   └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘   │
│        │            │            │            │        │
│        └─────────────┴────────────┴────────────┘        │
│                      │                                  │
│               ┌──────┴──────┐                          │
│               │   IPVS LB   │                          │
│               └──────┬──────┘                          │
│                      │                                  │
│   ┌────────┬────────┼────────┬────────┐                 │
│   ▼        ▼        ▼        ▼        ▼                 │
│ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐          │
│ │Task 1│ │Task 2│ │Task 3│ │Task 4│ │Task 5│          │
│ └──────┘ └──────┘ └──────┘ └──────┘ └──────┘          │
└─────────────────────────────────────────────────────────┘
```

---

## Summary

| Feature | Docker Swarm Solution |
|---------|---------------------|
| **Load Balancer** | Routing Mesh (IPVS) - No external LB needed |
| **Service Discovery** | DNS-based (service name = IP) |
| **ACME Sync** | Shared NFS/EFS volume |
| **Rate Limiting** | Redis (global) or Local (per-node) |
| **Raft Network** | Encrypted overlay network |
| **Scaling** | `docker service scale` |
| **High Availability** | Multi-node + task replicas |

### Advantages

1. **Simplicity**: Not as complex as Kubernetes
2. **Built-in LB**: No HAProxy/NGINX needed
3. **Rolling Updates**: Zero-downtime deployment
4. **Secret Management**: Secure with Docker secrets
5. **Overlay Networks**: Easy network isolation

### Disadvantages

1. **Shared Storage Required**: NFS/EFS needed for ACME
2. **Limited Orchestration**: Not as feature-rich as Kubernetes
3. **Monitoring**: Must be added separately (Prometheus/Grafana)

### Alternative: Raft-Based Certificate Sync

For environments without shared storage, certificates can be synchronized via **Raft consensus**:
- Leader node obtains certificate from ACME
- Certificate is replicated as a Raft log entry
- All followers write certificate to local disk
- No NFS, EFS, or external storage needed!

See [ACME_RAFT_SYNC.md](docs/ACME_RAFT_SYNC.md) for details.
