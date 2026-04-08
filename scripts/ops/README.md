# APICerebrus Operational Scripts

This directory contains operational scripts for managing APICerebrus in production.

## Available Scripts

### health-check.sh
Comprehensive health check for all APICerebrus components.

```bash
# Run all health checks
./health-check.sh

# Verbose output
./health-check.sh -v

# JSON output for monitoring systems
./health-check.sh -f json

# Check specific components only
./health-check.sh --only database,gateway
```

**Checks performed:**
- Process status (is APICerebrus running?)
- Gateway HTTP endpoint
- Admin API endpoint
- Database connectivity and integrity
- Disk space usage
- Memory usage
- Log file errors
- Backup status
- Network connectivity
- SSL certificate expiration

**Exit Codes:**
- 0: All checks passed
- 1: One or more checks failed
- 2: Critical system failure

### rotate-logs.sh
Manages log file rotation and cleanup.

```bash
# Rotate logs larger than 100MB
./rotate-logs.sh rotate

# Just cleanup old logs
./rotate-logs.sh cleanup

# Show log status
./rotate-logs.sh status

# Dry run
./rotate-logs.sh -n

# Custom retention
./rotate-logs.sh -r 7
```

**Features:**
- Automatic rotation of large log files
- Compression of old logs
- Configurable retention policies
- Disk space monitoring

### cleanup-old-data.sh
Cleans up old audit logs, expired sessions, and temporal data.

```bash
# Run cleanup with defaults (30 day retention)
./cleanup-old-data.sh

# Dry run first
./cleanup-old-data.sh -n

# Custom retention periods
./cleanup-old-data.sh -r 90 -s 14

# Force without confirmation
./cleanup-old-data.sh -f
```

**Cleans:**
- Old audit log files (older than retention period)
- Expired sessions from database
- Old audit records from database
- Optimizes database (VACUUM, ANALYZE)

### rotate-secrets.sh
Assists with rotating API keys, session secrets, and certificates.

```bash
# List secrets needing rotation
./rotate-secrets.sh list

# Rotate API key for a user
./rotate-secrets.sh api-key user_123

# Generate new session secret
./rotate-secrets.sh session-secret

# Generate new admin API key
./rotate-secrets.sh admin-key

# Generate SSL certificate
./rotate-secrets.sh ssl-cert api.example.com

# Rotate all secrets (use with caution!)
./rotate-secrets.sh all
```

**Security Features:**
- Automatic backup before rotation
- Confirmation prompts for destructive operations
- Secure key generation
- Audit logging

## Common Workflows

### Daily Operations

```bash
# Morning health check
./health-check.sh -f json | jq '.status'

# Check log status
./rotate-logs.sh status

# Review secrets needing rotation
./rotate-secrets.sh list
```

### Weekly Maintenance

```bash
# Rotate large logs
./rotate-logs.sh rotate

# Cleanup old data
./cleanup-old-data.sh

# Full health check with verbose output
./health-check.sh -v
```

### Monthly Maintenance

```bash
# Rotate all API keys older than 90 days
./rotate-secrets.sh list
./rotate-secrets.sh api-key <user_id>

# Check SSL certificate expiration
./rotate-secrets.sh list

# Generate new certificates if needed
./rotate-secrets.sh ssl-cert api.example.com
```

### Incident Response

```bash
# Quick health check
./health-check.sh

# Check for recent errors
./rotate-logs.sh status
grep ERROR /var/log/apicerberus/*.log

# Check database integrity
sqlite3 apicerberus.db "PRAGMA integrity_check;"
```

## Integration with Monitoring

### Prometheus Node Exporter Textfile

```bash
# Run health check and output for node exporter
./health-check.sh -f json > /var/lib/node_exporter/textfile/apicerberus.prom
```

### Nagios/Icinga Check

```bash
# Return appropriate exit codes for monitoring systems
./health-check.sh -q
exit $?
```

### Cron Jobs

Add to `/etc/cron.d/apicerberus-ops`:

```bash
# Daily health check at 8 AM
0 8 * * * root /opt/apicerberus/scripts/ops/health-check.sh -q || echo "APICerebrus health check failed" | mail -s "APICerebrus Alert" admin@example.com

# Weekly cleanup on Sundays at 2 AM
0 2 * * 0 root /opt/apicerberus/scripts/ops/cleanup-old-data.sh -f

# Daily log rotation at midnight
0 0 * * * root /opt/apicerberus/scripts/ops/rotate-logs.sh rotate
```

## Environment Variables

All scripts support these environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `DB_PATH` | SQLite database path | `./apicerberus.db` |
| `CONFIG_FILE` | Configuration file path | `./apicerberus.yaml` |
| `LOG_DIR` | Log directory path | `/var/log/apicerberus` |
| `AUDIT_DIR` | Audit archive directory | `./audit-archive` |
| `BACKUP_DIR` | Backup directory | `./backups` |
| `DRY_RUN` | Dry run mode | `false` |
| `FORCE` | Skip confirmations | `false` |
| `VERBOSE` | Verbose output | `false` |
| `LOG_FILE` | Script log file | `/var/log/apicerberus/*.log` |

## Troubleshooting

### Permission Denied
```bash
# Make scripts executable
chmod +x /opt/apicerberus/scripts/ops/*.sh

# Run as correct user
sudo -u apicerberus ./health-check.sh
```

### Database Locked
```bash
# Check for running processes
lsof apicerberus.db

# Stop APICerebrus before maintenance
systemctl stop apicerberus

# Run maintenance
./cleanup-old-data.sh

# Restart
systemctl start apicerberus
```

### Log Directory Not Found
```bash
# Create log directory
mkdir -p /var/log/apicerberus
chown apicerberus:apicerberus /var/log/apicerberus
```

## Security Considerations

1. **File Permissions**: Scripts should be owned by root and readable only by admin users
2. **Log Files**: Log files may contain sensitive data - secure appropriately
3. **Secret Rotation**: Always backup before rotating secrets
4. **Database Access**: Scripts need read access to database; some need write access
5. **Audit Trail**: All operations are logged to `/var/log/apicerberus/`

## Best Practices

1. **Test in Staging**: Always test scripts in a staging environment first
2. **Regular Backups**: Ensure backups are working before running cleanup scripts
3. **Monitor Disk Space**: Log rotation prevents disk full conditions
4. **Review Regularly**: Check `rotate-secrets.sh list` output monthly
5. **Document Changes**: Keep a log of manual operations performed
