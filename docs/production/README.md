# APICerebrus Production Documentation

This directory contains comprehensive guides for deploying and operating APICerebrus in production environments.

## Available Guides

### [DEPLOYMENT.md](DEPLOYMENT.md)
Step-by-step deployment guide covering:
- System requirements and prerequisites
- Installation methods (binary, Docker, source)
- Configuration management
- Systemd service setup
- Security hardening basics
- High availability setup
- Monitoring integration
- Backup configuration

**Use this when:** Setting up APICerebrus for the first time or migrating to production.

---

### [SCALING.md](SCALING.md)
Horizontal scaling strategies including:
- Vertical vs horizontal scaling trade-offs
- Raft clustering configuration
- Load balancing strategies (L4/L7)
- Database scaling options
- Caching strategies with Redis
- Rate limiting at scale
- Multi-region deployment patterns
- Performance tuning
- Capacity planning guidelines

**Use this when:** Your traffic is growing and you need to scale beyond a single node.

---

### [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
Comprehensive troubleshooting guide covering:
- Quick diagnostic procedures
- Common installation issues
- Configuration problems
- Database issues (locks, corruption, performance)
- Network connectivity problems
- Security incidents
- Raft cluster issues
- Common error messages and solutions
- Emergency procedures

**Use this when:** Something is not working and you need to diagnose the issue.

---

### [SECURITY_HARDENING.md](SECURITY_HARDENING.md)
Advanced security measures including:
- Network security (firewalls, segmentation, VPN)
- Authentication and authorization (MFA, RBAC)
- Data protection (encryption at rest/transit)
- Audit logging and integrity
- Secret management (Vault, AWS Secrets Manager)
- TLS configuration and certificate management
- Rate limiting and DDoS protection
- Container security (Docker, Kubernetes)
- Compliance (GDPR, SOC 2, PCI DSS)
- Security incident response

**Use this when:** You need to meet strict security requirements or compliance standards.

---

## Quick Reference

### Common Tasks

| Task | Command/Location |
|------|------------------|
| Check health | `curl http://localhost:8080/health` |
| View logs | `journalctl -u apicerberus -f` |
| Restart service | `systemctl restart apicerberus` |
| Run health check | `./scripts/ops/health-check.sh` |
| Backup database | `./scripts/backup/backup-sqlite.sh` |
| Rotate secrets | `./scripts/ops/rotate-secrets.sh` |
| Check config | `apicerberus -c config.yaml --validate` |

### Important File Locations

| File/Directory | Purpose |
|----------------|---------|
| `/etc/apicerberus/` | Configuration files |
| `/var/lib/apicerberus/` | Database and data files |
| `/var/log/apicerberus/` | Application logs |
| `/var/backups/apicerberus/` | Backup files |
| `/opt/apicerberus/scripts/` | Operational scripts |

### Default Ports

| Port | Service | Access |
|------|---------|--------|
| 8080 | Gateway HTTP | Public |
| 8443 | Gateway HTTPS | Public |
| 9876 | Admin API | Restricted |
| 9877 | Portal | Public |
| 50051 | gRPC | Optional |
| 12000 | Raft | Internal |

## Production Checklist

### Before Going Live

- [ ] Review all security settings
- [ ] Enable HTTPS/TLS
- [ ] Configure rate limiting
- [ ] Set up monitoring and alerting
- [ ] Configure automated backups
- [ ] Test disaster recovery
- [ ] Document runbooks
- [ ] Train operations team
- [ ] Set up log aggregation
- [ ] Configure audit logging

### After Going Live

- [ ] Monitor error rates
- [ ] Check response times
- [ ] Verify backup jobs
- [ ] Review access logs
- [ ] Test failover scenarios
- [ ] Update documentation
- [ ] Schedule security reviews

## Support

For additional support:

1. **Documentation**: Review the relevant guide above
2. **Scripts**: Check `/scripts/ops/` for operational tools
3. **Examples**: See `/deployments/examples/` for configuration samples
4. **Monitoring**: Use the monitoring stack in `/deployments/monitoring/`
5. **Community**: [GitHub Issues](https://github.com/APICerberus/APICerebrus/issues)

## Updates

These guides are updated regularly. Check for updates:
- After major APICerebrus releases
- When security advisories are published
- When new best practices emerge

Last updated: 2026-04-07
