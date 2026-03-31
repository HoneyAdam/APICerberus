# APICerebrus Docker Swarm Deployment

Docker Swarm cluster deployment for APICerebrus - without external load balancers (HAProxy/NGINX) using routing mesh!

## Features

- **Routing Mesh**: Docker Swarm's built-in load balancer - no external LB needed
- **Overlay Networks**: Raft, Backend, and Public network isolation
- **Shared Storage**: ACME certificate synchronization via NFS/EFS
- **Redis**: Global rate limiting support
- **Zero-Downtime**: Rolling updates for continuous deployment

## Quick Start

### 1. Requirements

- Docker Engine 20.10+
- Docker Swarm mode (at least 1 manager)
- NFS Server (for multi-node, optional)

### 2. Initialize Swarm

```bash
cd deployments/docker

# Initialize Swarm (on manager node)
docker swarm init --advertise-addr <MANAGER-IP>

# Add worker nodes (optional)
docker swarm join-token worker
# Run the output command on worker nodes
```

### 3. Configuration

```bash
# Copy environment file
cp .env.example .env

# Edit .env file
nano .env
```

`.env` content:
```env
# NFS Server (required for multi-node)
NFS_SERVER=nfs.example.com

# Let's Encrypt email
ACME_EMAIL=admin@example.com

# Passwords (auto-generated if left empty)
JWT_SECRET=
ADMIN_PASSWORD=
DB_PASSWORD=
```

### 4. Deploy

```bash
# Run deployment script
chmod +x deploy-swarm.sh
./deploy-swarm.sh deploy
```

Or manually:
```bash
# Deploy the stack
docker stack deploy -c docker-compose.swarm.yml apicerberus

# Check status
docker stack ps apicerberus
docker service ls
```

### 5. Scale

```bash
# Scale gateway to 5 replicas
./deploy-swarm.sh scale gateway 5

# Or
docker service scale apicerberus_gateway=5
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     DOCKER SWARM CLUSTER                        │
│                                                                 │
│  ┌───────────────────────────────────────────────────────────┐ │
│  │                  ROUTING MESH                              │ │
│  │  • Every node listens on 8080/8443                        │ │
│  │  • IPVS kernel load balancer                              │ │
│  │  • Automatic failover                                     │ │
│  └────────────────────┬──────────────────────────────────────┘ │
│                       │                                         │
│  ┌────────────────────┴──────────────────────────────────────┐ │
│  │              GATEWAY SERVICE (3+ replicas)                 │ │
│  │                                                            │ │
│  │   ┌─────────┐    ┌─────────┐    ┌─────────┐               │ │
│  │   │ Replica │◄──►│ Replica │◄──►│ Replica │  Raft         │ │
│  │   │   1     │    │   2     │    │   N     │  Consensus    │ │
│  │   └────┬────┘    └────┬────┘    └────┬────┘               │ │
│  │        │              │              │                     │ │
│  │        └──────────────┼──────────────┘                     │ │
│  │                       │                                     │ │
│  │              ┌────────┴────────┐                           │ │
│  │              │  Shared NFS/EFS │  ACME Certs               │ │
│  │              └─────────────────┘                           │ │
│  └───────────────────────────────────────────────────────────┘ │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## Commands

```bash
# Check status
./deploy-swarm.sh status

# View logs
./deploy-swarm.sh logs gateway

# Scale service
./deploy-swarm.sh scale gateway 5

# Remove stack
./deploy-swarm.sh destroy
```

## Network Architecture

| Network | Type | Purpose | Encryption |
|---------|------|---------|------------|
| `gateway-public` | Overlay | External HTTP/HTTPS traffic | No |
| `raft-cluster` | Overlay | Raft consensus (internal) | Yes (IPSec) |
| `backend` | Overlay | PostgreSQL, Redis | Yes (IPSec) |
| `monitoring` | Overlay | Prometheus, Grafana | No |

## ACME Certificate Synchronization

In a multi-node cluster, all nodes must serve the same certificate:

```
┌──────────────────────────────────────────────────────────────┐
│                    NFS SHARED STORAGE                        │
│  ┌─────────────┐                                            │
│  │/data/acme/  │                                            │
│  │  cert.pem   │                                            │
│  │  key.pem    │                                            │
│  └──────┬──────┘                                            │
│         │ Mount                                              │
│    ┌────┴────┬────────┬────────┐                             │
│    ▼         ▼        ▼        ▼                             │
│ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐                         │
│ │Node 1│ │Node 2│ │Node 3│ │Node N│  Same certificate        │
│ └──────┘ └──────┘ └──────┘ └──────┘                         │
└──────────────────────────────────────────────────────────────┘
```

## Troubleshooting

### Services not starting

```bash
# Check logs
docker service logs apicerberus_gateway

# Check tasks
docker stack ps apicerberus --no-trunc
```

### Network connectivity issues

```bash
# Check networks
docker network ls
docker network inspect apicerberus_gateway-public

# Test container network
docker run --rm --network apicerberus_backend alpine ping -c 3 postgres
```

### ACME certificate issues

```bash
# Check shared volume
docker run --rm -v apicerberus_acme-certs:/data/alpine ls -la /data

# Check NFS mount
showmount -e $NFS_SERVER
```

## Resources

- [DOCKER_SWARM_ARCHITECTURE.md](../../DOCKER_SWARM_ARCHITECTURE.md) - Detailed architecture documentation
- [ARCHITECTURE.md](../../ARCHITECTURE.md) - General project architecture
- [ACME_RAFT_SYNC.md](../../docs/ACME_RAFT_SYNC.md) - Raft-based certificate sync (no NFS!)
