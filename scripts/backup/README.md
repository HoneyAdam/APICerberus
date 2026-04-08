# APICerebrus Backup & Restore Documentation

## Overview

This directory contains comprehensive backup and restore scripts for APICerebrus:

- **backup-sqlite.sh** - Database-only backups
- **backup-config.sh** - Configuration backups
- **backup-full.sh** - Full system backups
- **restore.sh** - Universal restore script
- **backup-scheduler.sh** - Automated backup scheduling

## Quick Start

### Create a Database Backup
```bash
./backup-sqlite.sh
```

### Create a Full System Backup
```bash
./backup-full.sh
```

### Restore from Backup
```bash
./restore.sh -f /backups/apicerberus_20260407_120000.db.gz
```

### Setup Automated Backups
```bash
sudo ./backup-scheduler.sh install
```

## Backup Types

### 1. Database Backup (backup-sqlite.sh)

Creates compressed, timestamped backups of the SQLite database.

**Features:**
- Online backup (no downtime)
- Compression (gzip)
- Optional encryption (AES-256)
- Integrity verification
- Automatic retention cleanup
- Daily/weekly/monthly organization

**Usage:**
```bash
# Basic backup
./backup-sqlite.sh

# Custom database location
./backup-sqlite.sh -d /data/apicerberus.db

# Encrypted backup
./backup-sqlite.sh -e -k "my-secret-key"

# Custom retention (7 days)
./backup-sqlite.sh -r 7

# Dry run (no actual backup)
./backup-sqlite.sh -n
```

**Output:**
```
backups/
├── daily/
│   └── apicerberus_20260407_120000.db.gz
├── weekly/
│   └── apicerberus_20260406_030000.db.gz
├── monthly/
│   └── apicerberus_20260401_040000.db.gz
└── latest -> daily/apicerberus_20260407_120000.db.gz
```

### 2. Configuration Backup (backup-config.sh)

Backs up all configuration files and environment settings.

**Features:**
- YAML/JSON configuration files
- Environment files (.env)
- SSL certificates
- Optional secret inclusion
- Encryption support

**Usage:**
```bash
# Basic config backup
./backup-config.sh

# Include secrets (use with caution!)
./backup-config.sh --include-secrets

# Custom config directory
./backup-config.sh -c /etc/apicerberus
```

### 3. Full System Backup (backup-full.sh)

Comprehensive backup including database, configuration, and audit logs.

**Features:**
- Everything from database + config backups
- Audit log archives
- System metadata
- Complete verification
- Single-file archive

**Usage:**
```bash
# Full backup
./backup-full.sh

# Exclude audit logs
./backup-full.sh --no-audit

# Custom locations
./backup-full.sh -d /data/db.db -c /etc/apicerberus -a /var/audit
```

## Restore Operations

### Restore from Database Backup
```bash
# Restore database
./restore.sh -f backups/daily/apicerberus_20260407_120000.db.gz

# Restore to different location
./restore.sh -f backup.db.gz -d /new/location/apicerberus.db

# Restore encrypted backup
./restore.sh -f backup.db.enc -k "my-secret-key"
```

### Restore Full System
```bash
# Restore everything
./restore.sh -f backups/full/apicerberus_full_20260407_030000.tar.gz

# Dry run first
./restore.sh -f backup.tar.gz -n
```

### Restore Configuration Only
```bash
./restore.sh -f backups/config/apicerberus_config_20260401_040000.tar.gz
```

## Automated Scheduling

### Install Scheduled Backups
```bash
# Install with defaults
sudo ./backup-scheduler.sh install

# Custom user and paths
sudo ./backup-scheduler.sh install -u apicerberus -d /data/db.db -b /backups
```

### Default Schedule
- **Daily (2:00 AM)**: Database backup
- **Weekly Sundays (3:00 AM)**: Full system backup
- **Monthly 1st (4:00 AM)**: Configuration backup
- **Weekly Sundays (5:00 AM)**: Cleanup old backups

### Manage Scheduled Backups
```bash
# List current schedule
./backup-scheduler.sh list

# Remove scheduled backups
sudo ./backup-scheduler.sh remove

# Test scripts
./backup-scheduler.sh test
```

## Encryption

### Encrypted Backups
All backup scripts support AES-256 encryption via OpenSSL.

**Generate Encryption Key:**
```bash
openssl rand -base64 32
```

**Create Encrypted Backup:**
```bash
export ENCRYPTION_KEY="your-generated-key"
./backup-sqlite.sh -e
```

**Restore Encrypted Backup:**
```bash
./restore.sh -f backup.db.enc -k "your-generated-key"
```

**Important:** Store encryption keys securely and separately from backups!

## Retention Policies

### Default Retention
- **Database backups**: 30 days
- **Configuration backups**: 90 days
- **Full backups**: 30 days

### Custom Retention
```bash
# 7 day retention
./backup-sqlite.sh -r 7

# 1 year retention
./backup-full.sh -r 365
```

### Manual Cleanup
```bash
# Remove backups older than 30 days
find /backups -name "apicerberus_*.db*" -mtime +30 -delete
```

## Environment Variables

All scripts support these environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `DB_PATH` | SQLite database path | `./apicerberus.db` |
| `CONFIG_DIR` | Configuration directory | `./config` |
| `AUDIT_DIR` | Audit archive directory | `./audit-archive` |
| `BACKUP_DIR` | Backup output directory | `./backups` |
| `RETENTION_DAYS` | Backup retention period | `30` |
| `COMPRESS` | Enable compression | `true` |
| `ENCRYPT` | Enable encryption | `false` |
| `ENCRYPTION_KEY` | Encryption/decryption key | (none) |
| `VERIFY` | Verify backup integrity | `true` |
| `LOG_FILE` | Log file path | `/var/log/apicerberus/*.log` |

## Best Practices

### 1. Backup Strategy
- **Daily**: Database backups (automated)
- **Weekly**: Full system backups (automated)
- **Before upgrades**: Manual full backup
- **Before migrations**: Manual database backup

### 2. Storage Locations
- Local: Fast recovery
- Network: Redundancy
- Cloud: Disaster recovery
- Offsite: Geographic redundancy

### 3. Security
- Encrypt backups containing sensitive data
- Store encryption keys separately
- Use secure file permissions (600)
- Regularly test restore procedures

### 4. Verification
- Always verify backups after creation
- Test restore procedures monthly
- Monitor backup logs for errors
- Set up alerts for failed backups

### 5. Monitoring
```bash
# Check recent backup logs
tail -f /var/log/apicerberus/backup.log

# List backup files with sizes
ls -lh backups/daily/

# Check backup integrity
./backup-sqlite.sh -d /path/to/backup.db -n
```

## Troubleshooting

### Backup Fails
1. Check disk space: `df -h`
2. Check permissions: `ls -la backups/`
3. Check logs: `tail /var/log/apicerberus/backup.log`
4. Verify database: `sqlite3 apicerberus.db "PRAGMA integrity_check;"`

### Restore Fails
1. Verify backup file exists and is readable
2. Check encryption key (if encrypted)
3. Ensure sufficient disk space
4. Check file permissions

### Database Locked
1. Stop APICerebrus service
2. Check for other processes: `lsof apicerberus.db`
3. Clear WAL files if needed
4. Retry backup/restore

### Cron Jobs Not Running
1. Check cron service: `systemctl status crond`
2. Verify cron file: `cat /etc/cron.d/apicerberus-backup`
3. Check cron logs: `grep CRON /var/log/syslog`
4. Test manually: `sudo -u apicerberus ./backup-sqlite.sh`

## Migration Scenarios

### Moving to New Server
```bash
# On old server
./backup-full.sh
scp backups/full/latest new-server:/tmp/

# On new server
./restore.sh -f /tmp/latest
```

### Database Corruption Recovery
```bash
# Find last good backup
ls -lt backups/daily/

# Restore from backup
./restore.sh -f backups/daily/apicerberus_20260406_120000.db.gz

# Verify integrity
sqlite3 apicerberus.db "PRAGMA integrity_check;"
```

### Point-in-Time Recovery
```bash
# List available backups
ls backups/daily/

# Restore specific point in time
./restore.sh -f backups/daily/apicerberus_20260405_120000.db.gz
```
