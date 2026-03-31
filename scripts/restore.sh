#!/bin/bash
set -e

# APICerebrus Restore Script
# Usage: ./restore.sh <backup_file> [data_dir]

BACKUP_FILE="$1"
DATA_DIR="${2:-./data}"

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

if [ -z "$BACKUP_FILE" ]; then
    log_error "Usage: $0 <backup_file> [data_dir]"
    exit 1
fi

if [ ! -f "$BACKUP_FILE" ]; then
    log_error "Backup file not found: $BACKUP_FILE"
    exit 1
fi

log_info "Starting APICerebrus restore..."
log_info "Backup file: ${BACKUP_FILE}"
log_info "Target directory: ${DATA_DIR}"

# Confirm restore
read -p "⚠️  This will overwrite existing data. Continue? (y/N): " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    log_info "Restore cancelled"
    exit 0
fi

# Create data directory
mkdir -p "${DATA_DIR}"

# Create staging directory
TMP_DIR=$(mktemp -d)
trap "rm -rf ${TMP_DIR}" EXIT

# Extract backup
log_info "Extracting backup..."
tar -xzf "${BACKUP_FILE}" -C "${TMP_DIR}"

# Verify manifest
if [ ! -f "${TMP_DIR}/manifest.json" ]; then
    log_error "Invalid backup: manifest.json not found"
    exit 1
fi

# Backup current data before restore
if [ -d "${DATA_DIR}" ] && [ "$(ls -A ${DATA_DIR})" ]; then
    BACKUP_CURRENT="${DATA_DIR}_backup_$(date +%Y%m%d_%H%M%S)"
    log_warn "Backing up current data to: ${BACKUP_CURRENT}"
    mv "${DATA_DIR}" "${BACKUP_CURRENT}"
    mkdir -p "${DATA_DIR}"
fi

# Restore database
if [ -f "${TMP_DIR}/apicerberus.db" ]; then
    log_info "Restoring database..."
    cp "${TMP_DIR}/apicerberus.db" "${DATA_DIR}/"
    log_info "Database restored"
fi

# Restore ACME certificates
if [ -d "${TMP_DIR}/acme" ]; then
    log_info "Restoring ACME certificates..."
    cp -r "${TMP_DIR}/acme" "${DATA_DIR}/"
    log_info "ACME certificates restored"
fi

# Restore Raft snapshots
if [ -d "${TMP_DIR}/raft" ]; then
    log_info "Restoring Raft snapshots..."
    cp -r "${TMP_DIR}/raft" "${DATA_DIR}/"
    log_info "Raft snapshots restored"
fi

# Restore configuration
if [ -f "${TMP_DIR}/config.yaml" ]; then
    log_info "Restoring configuration..."
    cp "${TMP_DIR}/config.yaml" "${DATA_DIR}/"
fi

# Set permissions
chmod 600 "${DATA_DIR}/apicerberus.db" 2>/dev/null || true
chmod -R 700 "${DATA_DIR}/acme" 2>/dev/null || true
chmod -R 700 "${DATA_DIR}/raft" 2>/dev/null || true

log_info "Restore complete!"
log_info "Data restored to: ${DATA_DIR}"
log_info "Restart APICerebrus to apply changes"
