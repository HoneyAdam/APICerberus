#!/bin/bash
set -e

# APICerebrus Backup Script
# Usage: ./backup.sh [backup_dir]

BACKUP_DIR="${1:-./backups}"
DATA_DIR="${APICERBERUS_DATA_DIR:-./data}"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="apicerberus_backup_${TIMESTAMP}.tar.gz"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Create backup directory
mkdir -p "${BACKUP_DIR}"

# Temporary backup staging
TMP_DIR=$(mktemp -d)
trap "rm -rf ${TMP_DIR}" EXIT

log_info "Starting APICerebrus backup..."
log_info "Data directory: ${DATA_DIR}"
log_info "Backup directory: ${BACKUP_DIR}"
log_info "Backup file: ${BACKUP_FILE}"

# Backup SQLite database using the SQLite backup API with BUSY timeout
if [ -f "${DATA_DIR}/apicerberus.db" ]; then
    log_info "Backing up SQLite database..."
    sqlite3 "${DATA_DIR}/apicerberus.db" <<SQL
.timeout 5000
.backup '${TMP_DIR}/apicerberus.db'
SQL
    log_info "Database backup complete"
elif [ -f "${DATA_DIR}/apicerberus.db-wal" ] || [ -f "${DATA_DIR}/apicerberus.db-shm" ]; then
    # Database may be busy or in WAL mode — use VACUUM INTO as fallback
    log_info "Database file busy, using VACUUM INTO fallback..."
    sqlite3 "${DATA_DIR}/apicerberus.db" <<SQL
.timeout 10000
VACUUM INTO '${TMP_DIR}/apicerberus.db';
SQL
    log_info "Database backup complete (VACUUM INTO)"
else
    log_warn "Database file not found at ${DATA_DIR}/apicerberus.db"
fi

# Backup ACME certificates
if [ -d "${DATA_DIR}/acme" ]; then
    log_info "Backing up ACME certificates..."
    cp -r "${DATA_DIR}/acme" "${TMP_DIR}/"
    log_info "ACME certificates backup complete"
fi

# Backup Raft snapshots
if [ -d "${DATA_DIR}/raft" ]; then
    log_info "Backing up Raft snapshots..."
    cp -r "${DATA_DIR}/raft" "${TMP_DIR}/"
    log_info "Raft snapshots backup complete"
fi

# Backup configuration
if [ -f "${DATA_DIR}/config.yaml" ]; then
    log_info "Backing up configuration..."
    cp "${DATA_DIR}/config.yaml" "${TMP_DIR}/"
fi

# Create manifest
cat > "${TMP_DIR}/manifest.json" <<EOF
{
  "version": "1.0.0",
  "timestamp": "${TIMESTAMP}",
  "hostname": "$(hostname)",
  "files": [
    "apicerberus.db",
    "acme/",
    "raft/",
    "config.yaml"
  ]
}
EOF

# Create compressed archive
log_info "Creating compressed archive..."
tar -czf "${BACKUP_DIR}/${BACKUP_FILE}" -C "${TMP_DIR}" .

# Verify backup
if [ -f "${BACKUP_DIR}/${BACKUP_FILE}" ]; then
    SIZE=$(du -h "${BACKUP_DIR}/${BACKUP_FILE}" | cut -f1)
    log_info "Backup created successfully: ${BACKUP_FILE} (${SIZE})"

    # Verify database integrity from backup
    if [ -f "${TMP_DIR}/apicerberus.db" ]; then
        log_info "Verifying database integrity..."
        INTEGRITY=$(sqlite3 "${TMP_DIR}/apicerberus.db" "PRAGMA integrity_check;" 2>&1)
        if [ "${INTEGRITY}" = "ok" ]; then
            log_info "Database integrity check passed"
        else
            log_error "Database integrity check failed: ${INTEGRITY}"
            exit 1
        fi
    fi
else
    log_error "Backup creation failed!"
    exit 1
fi

# Cleanup old backups (keep last 7 days)
log_info "Cleaning up old backups (keeping 7 days)..."
find "${BACKUP_DIR}" -name "apicerberus_backup_*.tar.gz" -mtime +7 -delete

log_info "Backup complete!"
log_info "Backup location: ${BACKUP_DIR}/${BACKUP_FILE}"
