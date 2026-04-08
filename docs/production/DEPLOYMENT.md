# APICerebrus Production Deployment Guide

This guide covers deploying APICerebrus in production environments.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Installation](#installation)
3. [Configuration](#configuration)
4. [Security Hardening](#security-hardening)
5. [High Availability Setup](#high-availability-setup)
6. [Monitoring Setup](#monitoring-setup)
7. [Backup Configuration](#backup-configuration)
8. [Troubleshooting](#troubleshooting)

## Prerequisites

### System Requirements

| Component | Minimum | Recommended |
|-----------|---------|-------------|
| CPU | 2 cores | 4+ cores |
| RAM | 2 GB | 8+ GB |
| Disk | 20 GB SSD | 100+ GB SSD |
| Network | 100 Mbps | 1 Gbps |

### Software Requirements

- Linux (Ubuntu 22.04 LTS, RHEL 8+, or Debian 11+)
- Go 1.25+ (for building from source)
- SQLite 3.39+
- systemd (for service management)

### Network Requirements

| Port | Service | Direction |
|------|---------|-----------|
| 8080 | Gateway HTTP | Inbound |
| 8443 | Gateway HTTPS | Inbound |
| 9876 | Admin API | Inbound (restricted) |
| 9877 | Portal | Inbound |
| 50051 | gRPC | Inbound (optional) |
| 12000 | Raft | Internal |

## Installation

### Option 1: Binary Installation

```bash
# Download latest release
curl -L -o apicerberus.tar.gz \
  https://github.com/APICerberus/APICerebrus/releases/latest/download/apicerberus-linux-amd64.tar.gz

# Extract
tar -xzf apicerberus.tar.gz
sudo mv apicerberus /usr/local/bin/
sudo chmod +x /usr/local/bin/apicerberus

# Create directories
sudo mkdir -p /etc/apicerberus
sudo mkdir -p /var/lib/apicerberus
sudo mkdir -p /var/log/apicerberus
sudo mkdir -p /var/backups/apicerberus

# Create user
sudo useradd -r -s /bin/false apicerberus
sudo chown -R apicerberus:apicerberus /var/lib/apicerberus
sudo chown -R apicerberus:apicerberus /var/log/apicerberus
```

### Option 2: Docker Installation

```bash
# Pull image
docker pull apicerberus/apicerberus:latest

# Run with docker-compose
curl -O https://raw.githubusercontent.com/APICerberus/APICerebrus/main/deployments/docker/docker-compose.yml
docker-compose up -d
```

### Option 3: Build from Source

```bash
# Clone repository
git clone https://github.com/APICerberus/APICerebrus.git
cd APICerebrus

# Build
make build

# Install
sudo make install
```

## Configuration

### 1. Generate Secrets

```bash
# Generate admin API key
export ADMIN_API_KEY=$(openssl rand -base64 32)

# Generate session secret
export SESSION_SECRET=$(openssl rand -base64 64)

# Save to secure location
echo "ADMIN_API_KEY=$ADMIN_API_KEY" > /etc/apicerberus/.env
chmod 600 /etc/apicerberus/.env
```

### 2. Create Configuration

```bash
# Copy example configuration
sudo cp /usr/share/apicerberus/apicerberus.example.yaml /etc/apicerberus/apicerberus.yaml

# Edit configuration
sudo nano /etc/apicerberus/apicerberus.yaml
```

### 3. Systemd Service

Create `/etc/systemd/system/apicerberus.service`:

```ini
[Unit]
Description=APICerebrus API Gateway
Documentation=https://docs.apicerberus.io
After=network.target

[Service]
Type=simple
User=apicerberus
Group=apicerberus
EnvironmentFile=/etc/apicerberus/.env
ExecStart=/usr/local/bin/apicerberus -c /etc/apicerberus/apicerberus.yaml
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=apicerberus

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/apicerberus /var/log/apicerberus

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable apicerberus
sudo systemctl start apicerberus
sudo systemctl status apicerberus
```

## Security Hardening

### 1. File Permissions

```bash
# Set secure permissions
sudo chmod 750 /etc/apicerberus
sudo chmod 600 /etc/apicerberus/apicerberus.yaml
sudo chmod 600 /etc/apicerberus/.env
sudo chown -R root:apicerberus /etc/apicerberus
```

### 2. Firewall Configuration

```bash
# UFW (Ubuntu)
sudo ufw default deny incoming
sudo ufw allow 22/tcp
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw allow from 10.0.0.0/8 to any port 9876
sudo ufw enable

# firewalld (RHEL)
sudo firewall-cmd --permanent --add-service=http
sudo firewall-cmd --permanent --add-service=https
sudo firewall-cmd --permanent --add-rich-rule='rule family="ipv4" source address="10.0.0.0/8" port port="9876" protocol="tcp" accept'
sudo firewall-cmd --reload
```

### 3. TLS Configuration

```yaml
gateway:
  https_addr: ":8443"
  tls:
    auto: true
    acme_email: "admin@example.com"
    acme_dir: "/var/lib/apicerberus/certs"
```

### 4. Admin API Protection

```bash
# Restrict admin to localhost + VPN
sudo iptables -A INPUT -p tcp --dport 9876 -s 127.0.0.1 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 9876 -s 10.0.0.0/8 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 9876 -j DROP
```

## High Availability Setup

### 1. Raft Cluster Configuration

On each node, edit configuration:

```yaml
raft:
  enabled: true
  node_id: "node1"  # Unique per node
  bind_addr: ":12000"
  data_dir: "/var/lib/apicerberus/raft"
  bootstrap: true  # Only on first node
  peers:
    - "node1:12000"
    - "node2:12000"
    - "node3:12000"
```

### 2. Redis for Distributed Rate Limiting

```yaml
redis:
  enabled: true
  address: "redis-cluster:6379"
  password: "${REDIS_PASSWORD}"
  pool_size: 20
```

### 3. Load Balancer Configuration

**Nginx:**

```nginx
upstream apicerberus {
    least_conn;
    server node1:8080;
    server node2:8080;
    server node3:8080;
    keepalive 32;
}

server {
    listen 80;
    server_name api.example.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name api.example.com;

    ssl_certificate /etc/ssl/certs/api.example.com.crt;
    ssl_certificate_key /etc/ssl/private/api.example.com.key;

    location / {
        proxy_pass http://apicerberus;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## Monitoring Setup

### 1. Install Monitoring Stack

```bash
cd /opt/apicerberus/deployments/monitoring
docker-compose up -d
```

### 2. Configure Alerts

Edit `alertmanager/alertmanager.yml` with your notification channels.

### 3. Import Dashboards

1. Access Grafana at http://localhost:3000
2. Import dashboards from `grafana/dashboards/`

## Backup Configuration

### 1. Automated Backups

```bash
# Install backup scripts
sudo cp scripts/backup/*.sh /opt/apicerberus/backup/
sudo chmod +x /opt/apicerberus/backup/*.sh

# Setup cron
sudo cp scripts/backup/backup-scheduler.sh /etc/cron.daily/apicerberus-backup
```

### 2. Backup Verification

```bash
# Test restore
/opt/apicerberus/backup/restore.sh -f /var/backups/apicerberus/latest -n
```

## Troubleshooting

### Service Won't Start

```bash
# Check logs
sudo journalctl -u apicerberus -f

# Verify configuration
apicerberus -c /etc/apicerberus/apicerberus.yaml --validate

# Check permissions
ls -la /var/lib/apicerberus
ls -la /var/log/apicerberus
```

### Database Issues

```bash
# Check integrity
sqlite3 /var/lib/apicerberus/apicerberus.db "PRAGMA integrity_check;"

# Vacuum database
sqlite3 /var/lib/apicerberus/apicerberus.db "VACUUM;"
```

### Performance Issues

```bash
# Check resource usage
top -p $(pgrep apicerberus)
iostat -x 1

# Profile application
curl http://localhost:9876/debug/pprof/profile > profile.out
go tool pprof profile.out
```

### Common Errors

| Error | Solution |
|-------|----------|
| `bind: address already in use` | Check for conflicting services |
| `permission denied` | Check file permissions |
| `database is locked` | Stop service, check WAL files |
| `TLS handshake error` | Check certificate validity |

## Next Steps

- Review [SCALING.md](SCALING.md) for horizontal scaling
- Review [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for advanced debugging
- Review [SECURITY_HARDENING.md](SECURITY_HARDENING.md) for additional security measures
