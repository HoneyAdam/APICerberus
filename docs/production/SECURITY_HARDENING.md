# APICerebrus Security Hardening Guide

This guide provides advanced security measures for production deployments of APICerebrus.

## Table of Contents

1. [Security Overview](#security-overview)
2. [Network Security](#network-security)
3. [Authentication & Authorization](#authentication--authorization)
4. [Data Protection](#data-protection)
5. [Audit Logging](#audit-logging)
6. [Secret Management](#secret-management)
7. [TLS Configuration](#tls-configuration)
8. [Rate Limiting & DDoS Protection](#rate-limiting--ddos-protection)
9. [Container Security](#container-security)
10. [Compliance](#compliance)

## Security Overview

APICerebrus implements defense in depth with multiple security layers:

```
┌─────────────────────────────────────────────────────────────┐
│                    WAF / DDoS Protection                    │
├─────────────────────────────────────────────────────────────┤
│                      Load Balancer                          │
│                    (TLS Termination)                        │
├─────────────────────────────────────────────────────────────┤
│                     APICerebrus Cluster                     │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐         │
│  │   Node 1    │  │   Node 2    │  │   Node 3    │         │
│  │  API Auth   │  │  Rate Limit │  │   Audit     │         │
│  │   WAF       │  │   Cache     │  │   Log       │         │
│  └─────────────┘  └─────────────┘  └─────────────┘         │
├─────────────────────────────────────────────────────────────┤
│                      Data Storage                           │
│              (Encrypted SQLite + Redis)                     │
└─────────────────────────────────────────────────────────────┘
```

## Network Security

### Firewall Configuration

**iptables (Linux):**

```bash
#!/bin/bash
# /etc/iptables/apicerberus.rules

# Flush existing rules
iptables -F
iptables -X

# Default policy
iptables -P INPUT DROP
iptables -P FORWARD DROP
iptables -P OUTPUT ACCEPT

# Allow loopback
iptables -A INPUT -i lo -j ACCEPT

# Allow established connections
iptables -A INPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT

# Allow SSH (restrict to specific IPs)
iptables -A INPUT -p tcp --dport 22 -s 10.0.0.0/8 -j ACCEPT

# Allow HTTP/HTTPS
iptables -A INPUT -p tcp --dport 80 -j ACCEPT
iptables -A INPUT -p tcp --dport 443 -j ACCEPT
iptables -A INPUT -p tcp --dport 8080 -j ACCEPT
iptables -A INPUT -p tcp --dport 8443 -j ACCEPT

# Allow Admin API only from internal network
iptables -A INPUT -p tcp --dport 9876 -s 10.0.0.0/8 -j ACCEPT

# Allow Raft communication between nodes
iptables -A INPUT -p tcp --dport 12000 -s 10.0.0.0/8 -j ACCEPT

# Allow monitoring
iptables -A INPUT -p tcp --dport 9090 -s 10.0.0.0/8 -j ACCEPT

# Log dropped packets
iptables -A INPUT -j LOG --log-prefix "IPTABLES DROP: "

# Save rules
iptables-save > /etc/iptables/rules.v4
```

**nftables (Modern Linux):**

```bash
#!/usr/sbin/nft -f

flush ruleset

table inet filter {
    chain input {
        type filter hook input priority 0; policy drop;
        
        # Allow loopback
        iif lo accept
        
        # Allow established
        ct state established,related accept
        
        # Drop invalid
        ct state invalid drop
        
        # Allow ICMP
        ip protocol icmp accept
        ip6 nexthdr icmpv6 accept
        
        # Allow SSH from internal
        ip saddr 10.0.0.0/8 tcp dport 22 accept
        
        # Allow HTTP/HTTPS
        tcp dport { 80, 443, 8080, 8443 } accept
        
        # Admin API internal only
        ip saddr 10.0.0.0/8 tcp dport 9876 accept
        
        # Raft cluster
        ip saddr 10.0.0.0/8 tcp dport 12000 accept
        
        # Log and drop
        log prefix "nftables dropped: " limit rate 5/second
        drop
    }
    
    chain forward {
        type filter hook forward priority 0; policy drop;
    }
    
    chain output {
        type filter hook output priority 0; policy accept;
    }
}
```

### Network Segmentation

**VLAN Configuration:**

```
VLAN 10: Public (Internet-facing)
  - Load Balancers
  - Reverse Proxies

VLAN 20: Application (Internal)
  - APICerebrus Nodes
  - Redis Cluster

VLAN 30: Management (Restricted)
  - Admin API
  - Monitoring
  - Bastion Hosts

VLAN 40: Data (Isolated)
  - Database Backups
  - Audit Archives
```

### VPN Access

**WireGuard Configuration:**

```ini
# /etc/wireguard/wg0.conf
[Interface]
PrivateKey = <server-private-key>
Address = 10.200.200.1/24
ListenPort = 51820
PostUp = iptables -A FORWARD -i wg0 -j ACCEPT; iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
PostDown = iptables -D FORWARD -i wg0 -j ACCEPT; iptables -t nat -D POSTROUTING -o eth0 -j MASQUERADE

[Peer]
# Admin 1
PublicKey = <admin1-public-key>
AllowedIPs = 10.200.200.2/32

[Peer]
# Admin 2
PublicKey = <admin2-public-key>
AllowedIPs = 10.200.200.3/32
```

## Authentication & Authorization

### API Key Security

**Key Generation:**

```bash
# Generate cryptographically secure API key
openssl rand -base64 32 | tr -d '=+/' | cut -c1-32

# Or use APICerebrus CLI
apicerberus keys generate --prefix "ck_live"
```

**Key Rotation Policy:**

```bash
#!/bin/bash
# /usr/local/bin/rotate-api-keys.sh

# Rotate keys older than 90 days
CUTOFF=$(date -d '90 days ago' +%Y-%m-%d)

sqlite3 /var/lib/apicerberus/apicerberus.db <<EOF
SELECT id, user_id FROM api_keys 
WHERE created_at < '$CUTOFF' AND status = 'active';
EOF

# Notify users
# Generate new keys
# Revoke old keys after grace period
```

### Multi-Factor Authentication

**TOTP Configuration:**

```yaml
auth:
  mfa:
    enabled: true
    issuer: "APICerebrus"
    algorithm: "SHA256"
    digits: 6
    period: 30
    skew: 1
```

**Enforcement:**

```yaml
acl:
  groups:
    - name: "admin"
      mfa_required: true
      permissions:
        - "*"
    
    - name: "developer"
      mfa_required: false
      permissions:
        - "routes:read"
        - "services:read"
```

### Role-Based Access Control (RBAC)

```yaml
rbac:
  enabled: true
  
  roles:
    - name: "super-admin"
      permissions:
        - "*"
    
    - name: "admin"
      permissions:
        - "routes:*"
        - "services:*"
        - "consumers:*"
        - "plugins:*"
        - "analytics:read"
      excluded:
        - "system:config"
        - "system:restart"
    
    - name: "operator"
      permissions:
        - "routes:read"
        - "services:read"
        - "analytics:read"
        - "logs:read"
    
    - name: "viewer"
      permissions:
        - "routes:read"
        - "services:read"
        - "analytics:read"

  users:
    - id: "admin@example.com"
      roles:
        - "super-admin"
      mfa_enabled: true
```

## Data Protection

### Encryption at Rest

**SQLite Encryption:**

```bash
# Using SQLCipher (SQLite encryption extension)
sqlcipher /var/lib/apicerberus/apicerberus.db <<EOF
PRAGMA key = '${DB_ENCRYPTION_KEY}';
PRAGMA cipher_page_size = 4096;
PRAGMA kdf_iter = 256000;
EOF
```

**Configuration:**

```yaml
store:
  path: "/var/lib/apicerberus/apicerberus.db"
  encryption:
    enabled: true
    provider: "sqlcipher"
    key_file: "/etc/apicerberus/.db-key"
```

### Field-Level Encryption

```yaml
encryption:
  enabled: true
  provider: "aes-gcm"
  key_rotation_days: 90
  
  fields:
    - table: "api_keys"
      column: "key_hash"
    - table: "users"
      column: "password_hash"
    - table: "sessions"
      column: "token_hash"
```

### Backup Encryption

```bash
#!/bin/bash
# Encrypted backup script

BACKUP_FILE="/var/backups/apicerberus-$(date +%Y%m%d).sql.gz"
ENCRYPTION_KEY="${BACKUP_ENCRYPTION_KEY}"

# Create encrypted backup
sqlite3 /var/lib/apicerberus/apicerberus.db ".dump" | \
  gzip | \
  openssl enc -aes-256-cbc -salt -k "$ENCRYPTION_KEY" > "$BACKUP_FILE.enc"

# Verify
openssl enc -aes-256-cbc -d -k "$ENCRYPTION_KEY" -in "$BACKUP_FILE.enc" | \
  gunzip | \
  head -5
```

## Audit Logging

### Comprehensive Audit Configuration

```yaml
audit:
  enabled: true
  level: "detailed"  # none, basic, detailed, debug
  
  events:
    - authentication
    - authorization
    - configuration_change
    - data_access
    - admin_action
  
  mask:
    headers:
      - "Authorization"
      - "X-API-Key"
      - "Cookie"
      - "X-Auth-Token"
    body_fields:
      - "password"
      - "token"
      - "secret"
      - "credit_card"
      - "ssn"
      - "api_key"
    mask_replacement: "***REDACTED***"
  
  retention:
    days: 365
    compliance_mode: true
    immutable: true
  
  output:
    - type: "file"
      path: "/var/log/apicerberus/audit.log"
      format: "json"
    - type: "syslog"
      address: "syslog.example.com:514"
      protocol: "tcp"
      tls: true
    - type: "s3"
      bucket: "apicerberus-audit"
      region: "us-east-1"
      encryption: "aws:kms"
```

### Audit Log Integrity

```bash
#!/bin/bash
# Verify audit log integrity

AUDIT_LOG="/var/log/apicerberus/audit.log"
HASH_FILE="/var/log/apicerberus/audit.log.hash"

# Generate hash chain
generate_hash() {
    local line="$1"
    local prev_hash="$2"
    echo -n "${prev_hash}${line}" | sha256sum | cut -d' ' -f1
}

# Verify chain
verify_chain() {
    local prev_hash="0" * 64
    
    while IFS= read -r line; do
        local current_hash
        current_hash=$(echo "$line" | jq -r '._hash')
        local computed_hash
        computed_hash=$(generate_hash "$line" "$prev_hash")
        
        if [[ "$current_hash" != "$computed_hash" ]]; then
            echo "INTEGRITY VIOLATION at line: $line"
            return 1
        fi
        
        prev_hash="$current_hash"
    done < "$AUDIT_LOG"
    
    echo "Audit log integrity verified"
}
```

## Secret Management

### HashiCorp Vault Integration

```yaml
secrets:
  provider: "vault"
  vault:
    address: "https://vault.example.com:8200"
    auth:
      method: "kubernetes"
      role: "apicerberus"
    paths:
      api_keys: "secret/apicerberus/api-keys"
      session_secret: "secret/apicerberus/session"
      db_encryption: "secret/apicerberus/database"
```

### AWS Secrets Manager

```yaml
secrets:
  provider: "aws"
  aws:
    region: "us-east-1"
    secrets:
      admin_key: "arn:aws:secretsmanager:us-east-1:123456789:secret:apicerberus/admin-key"
      session_secret: "arn:aws:secretsmanager:us-east-1:123456789:secret:apicerberus/session"
```

### Secret Rotation

```bash
#!/bin/bash
# Automated secret rotation

rotate_secret() {
    local secret_name="$1"
    local secret_path="$2"
    
    # Generate new secret
    local new_secret
    new_secret=$(openssl rand -base64 32)
    
    # Store new version
    vault kv put "$secret_path" value="$new_secret"
    
    # Graceful rotation
    # 1. Add new secret as valid
    # 2. Update applications
    # 3. Remove old secret after TTL
    
    echo "Rotated: $secret_name"
}

# Rotate all secrets
rotate_secret "admin-api-key" "secret/apicerberus/admin-key"
rotate_secret "session-secret" "secret/apicerberus/session"
```

## TLS Configuration

### Strong TLS Settings

```yaml
gateway:
  https_addr: ":8443"
  tls:
    cert_file: "/etc/apicerberus/ssl/cert.pem"
    key_file: "/etc/apicerberus/ssl/key.pem"
    
    # Modern TLS configuration
    min_version: "1.3"
    max_version: "1.3"
    
    # Cipher suites (TLS 1.2 fallback)
    cipher_suites:
      - "TLS_AES_256_GCM_SHA384"
      - "TLS_CHACHA20_POLY1305_SHA256"
      - "TLS_AES_128_GCM_SHA256"
    
    # Certificate settings
    prefer_server_cipher_suites: true
    session_tickets_disabled: true
    
    # Client certificates (mTLS)
    client_auth: "verify-if-given"
    client_ca_file: "/etc/apicerberus/ssl/ca.pem"
    
    # OCSP stapling
    ocsp_stapling: true
    
    # Certificate rotation
    auto_reload: true
    reload_interval: "1h"
```

### Certificate Management

**Let's Encrypt with Certbot:**

```bash
#!/bin/bash
# /etc/letsencrypt/renewal-hooks/deploy/apicerberus.sh

# Reload certificates
systemctl reload apicerberus

# Verify
openssl s_client -connect localhost:8443 -servername api.example.com < /dev/null
```

**Certificate Pinning:**

```yaml
tls:
  pinning:
    enabled: true
    hashes:
      - "sha256/AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
      - "sha256/BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB="
    report_uri: "https://report.example.com/pin"
```

## Rate Limiting & DDoS Protection

### Advanced Rate Limiting

```yaml
routes:
  - name: "api-route"
    plugins:
      - name: "rate-limit"
        config:
          # Multiple tiers
          tiers:
            - name: "free"
              limit: 100
              window: "1h"
              burst: 10
            - name: "pro"
              limit: 10000
              window: "1h"
              burst: 100
            - name: "enterprise"
              limit: 100000
              window: "1h"
              burst: 1000
          
          # Per-endpoint limits
          endpoints:
            "/api/v1/users":
              limit: 1000
              window: "1m"
            "/api/v1/search":
              limit: 100
              window: "1m"
          
          # Response headers
          headers:
            enabled: true
            limit_header: "X-RateLimit-Limit"
            remaining_header: "X-RateLimit-Remaining"
            reset_header: "X-RateLimit-Reset"
```

### DDoS Protection

```yaml
security:
  ddos_protection:
    enabled: true
    
    # Connection limits
    max_connections_per_ip: 100
    max_requests_per_second: 50
    max_burst: 100
    
    # Challenge settings
    challenge:
      type: "javascript"
      difficulty: 4
      timeout: "30s"
    
    # Block rules
    block:
      - condition: "user_agent matches 'bot|crawler|spider'"
        action: "challenge"
      - condition: "request_rate > 1000"
        action: "block"
        duration: "1h"
      - condition: "geo_country in ['CN', 'RU']"
        action: "challenge"
    
    # Whitelist
    whitelist:
      ips:
        - "10.0.0.0/8"
        - "172.16.0.0/12"
      user_agents:
        - "InternalMonitoring/1.0"
```

## Container Security

### Docker Security

**Secure Dockerfile:**

```dockerfile
# Multi-stage build
FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o apicerberus

# Final image
FROM scratch

# Copy certificates
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Create non-root user
COPY --from=builder /etc/passwd /etc/passwd
USER 65534:65534

# Copy binary
COPY --from=builder /build/apicerberus /apicerberus

# No shell, minimal attack surface
ENTRYPOINT ["/apicerberus"]
```

**Security Options:**

```yaml
# docker-compose.yml
services:
  apicerberus:
    image: apicerberus/apicerberus:latest
    read_only: true
    user: "65534:65534"
    security_opt:
      - no-new-privileges:true
    cap_drop:
      - ALL
    cap_add:
      - NET_BIND_SERVICE
    tmpfs:
      - /tmp:noexec,nosuid,size=100m
    volumes:
      - ./config:/etc/apicerberus:ro
      - data:/var/lib/apicerberus
      - /var/log/apicerberus:/var/log/apicerberus
```

### Kubernetes Security

**Pod Security Context:**

```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 65534
        runAsGroup: 65534
        fsGroup: 65534
        seccompProfile:
          type: RuntimeDefault
      containers:
        - name: apicerberus
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities:
              drop:
                - ALL
          resources:
            limits:
              memory: "1Gi"
              cpu: "1000m"
            requests:
              memory: "256Mi"
              cpu: "250m"
```

**Network Policy:**

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: apicerberus-deny-all
spec:
  podSelector:
    matchLabels:
      app: apicerberus
  policyTypes:
    - Ingress
    - Egress
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: apicerberus-allow-specific
spec:
  podSelector:
    matchLabels:
      app: apicerberus
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              name: ingress-nginx
      ports:
        - protocol: TCP
          port: 8080
  egress:
    - to:
        - namespaceSelector:
            matchLabels:
              name: redis
      ports:
        - protocol: TCP
          port: 6379
```

## Compliance

### GDPR Compliance

```yaml
compliance:
  gdpr:
    enabled: true
    
    # Data retention
    retention:
      user_data: "2y"
      audit_logs: "7y"
      access_logs: "1y"
    
    # Right to be forgotten
    data_deletion:
      enabled: true
      grace_period: "30d"
      verification_required: true
    
    # Data portability
    data_export:
      enabled: true
      formats:
        - "json"
        - "csv"
    
    # Consent management
    consent:
      required: true
      granular: true
      withdrawable: true
```

### SOC 2 Compliance

```yaml
compliance:
  soc2:
    enabled: true
    
    # Access controls
    access_control:
      principle_of_least_privilege: true
      regular_access_reviews: "quarterly"
      privileged_access_monitoring: true
    
    # Change management
    change_management:
      approval_required: true
      testing_required: true
      rollback_plan_required: true
    
    # Monitoring
    monitoring:
      anomaly_detection: true
      alerting:
        - "unauthorized_access"
        - "privilege_escalation"
        - "data_exfiltration"
```

### PCI DSS Compliance

```yaml
compliance:
  pci_dss:
    enabled: true
    level: 1
    
    # Network segmentation
    network_segmentation:
      enabled: true
      isolated: true
    
    # Encryption
    encryption:
      data_at_rest: "AES-256"
      data_in_transit: "TLS 1.3"
      key_management: "HSM"
    
    # Logging
    logging:
      comprehensive: true
      tamper_proof: true
      retention: "1y"
```

## Security Checklist

### Pre-Deployment

- [ ] Change all default passwords
- [ ] Generate strong API keys
- [ ] Configure TLS 1.3
- [ ] Enable audit logging
- [ ] Set up firewall rules
- [ ] Configure rate limiting
- [ ] Enable DDoS protection
- [ ] Set up monitoring/alerting
- [ ] Configure backups
- [ ] Document security procedures

### Post-Deployment

- [ ] Run security scan
- [ ] Test failover scenarios
- [ ] Verify backup restoration
- [ ] Test incident response
- [ ] Review access logs
- [ ] Check certificate expiration
- [ ] Verify encryption at rest
- [ ] Test secret rotation
- [ ] Review firewall rules
- [ ] Conduct penetration test

### Ongoing

- [ ] Daily: Review security alerts
- [ ] Weekly: Check access logs
- [ ] Monthly: Rotate secrets
- [ ] Quarterly: Security audit
- [ ] Annually: Penetration test
- [ ] As needed: Update dependencies

## Security Incident Response

### Incident Classification

| Level | Description | Response Time |
|-------|-------------|---------------|
| P1 | Active breach/data loss | 15 minutes |
| P2 | Potential vulnerability | 1 hour |
| P3 | Security alert | 4 hours |
| P4 | Policy violation | 24 hours |

### Response Playbook

```bash
#!/bin/bash
# security-incident-response.sh

INCIDENT_TYPE="$1"

case "$INCIDENT_TYPE" in
  "unauthorized-access")
    # 1. Isolate affected systems
    iptables -A INPUT -s "$ATTACKER_IP" -j DROP
    
    # 2. Revoke compromised credentials
    ./scripts/ops/rotate-secrets.sh all --force
    
    # 3. Preserve evidence
    tar -czf "/incidents/evidence-$(date +%s).tar.gz" /var/log/apicerberus
    
    # 4. Notify stakeholders
    echo "Security incident detected" | mail -s "URGENT: Security Incident" security@example.com
    ;;
    
  "data-exfiltration")
    # 1. Block outbound traffic
    iptables -A OUTPUT -p tcp --dport 443 -m limit --limit 10/min -j ACCEPT
    iptables -A OUTPUT -p tcp --dport 443 -j DROP
    
    # 2. Enable enhanced logging
    # Update config to debug level
    
    # 3. Capture network traffic
    tcpdump -i eth0 -w "/incidents/traffic-$(date +%s).pcap" &
    ;;
esac
```

## Additional Resources

- [OWASP API Security Top 10](https://owasp.org/www-project-api-security/)
- [NIST Cybersecurity Framework](https://www.nist.gov/cyberframework)
- [CIS Benchmarks](https://www.cisecurity.org/cis-benchmarks)
- [SANS Security Resources](https://www.sans.org/security-resources/)
