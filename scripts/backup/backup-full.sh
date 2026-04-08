#!/bin/bash
#
# APICerebrus Full System Backup Script
# Backs up database, configuration, and audit logs
#

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DB_PATH="${DB_PATH:-./apicerberus.db}"
CONFIG_DIR="${CONFIG_DIR:-./config}"
AUDIT_DIR="${AUDIT_DIR:-./audit-archive}"
BACKUP_DIR="${BACKUP_DIR:-./backups/full}"
RETENTION_DAYS="${RETENTION_DAYS:-30}"
INCLUDE_AUDIT="${INCLUDE_AUDIT:-true}"
COMPRESS="${COMPRESS:-true}"
ENCRYPT="${ENCRYPT:-true}"
ENCRYPTION_KEY="${ENCRYPTION_KEY:-}"
VERIFY="${VERIFY:-true}"
LOG_FILE="${LOG_FILE:-/var/log/apicerberus/backup-full.log}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1" | tee -a "$LOG_FILE"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1" | tee -a "$LOG_FILE"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1" | tee -a "$LOG_FILE"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1" | tee -a "$LOG_FILE"
}

# Create log directory
mkdir -p "$(dirname "$LOG_FILE")" 2>/dev/null || true

# Help message
usage() {
    cat <<EOF
APICerebrus Full System Backup Script

Usage: $0 [OPTIONS]

Options:
    -d, --db PATH           Database path (default: ./apicerberus.db)
    -c, --config DIR        Config directory (default: ./config)
    -a, --audit DIR         Audit archive directory (default: ./audit-archive)
    -o, --output DIR        Backup directory (default: ./backups/full)
    -r, --retention DAYS    Retention period in days (default: 30)
    --no-audit              Don't include audit logs
    --no-compress           Don't compress backup
    --no-encrypt            Don't encrypt backup
    --no-verify             Skip verification
    -k, --key KEY           Encryption key
    -h, --help              Show this help message

Environment Variables:
    DB_PATH                 Database file path
    CONFIG_DIR              Configuration directory
    AUDIT_DIR               Audit archive directory
    BACKUP_DIR              Backup output directory
    RETENTION_DAYS          Backup retention period
    INCLUDE_AUDIT           Include audit logs (default: true)
    COMPRESS                Compress backup (default: true)
    ENCRYPT                 Encrypt backup (default: true)
    ENCRYPTION_KEY          Encryption key
    VERIFY                  Verify backup (default: true)
    LOG_FILE                Log file path

Examples:
    $0                              # Full backup with defaults
    $0 -d /data/apicerberus.db     # Backup specific database
    $0 --no-audit                   # Exclude audit logs
    $0 -k "mysecretkey"             # Encrypted backup

Backup Contents:
    - SQLite database (with WAL files)
    - Configuration files (YAML, JSON, .env)
    - SSL certificates
    - Audit logs (optional)
    - System metadata

Backup Schedule Recommendations:
    - Daily: Database only (backup-sqlite.sh)
    - Weekly: Full system backup (this script)
    - Monthly: Archive to remote storage
EOF
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -d|--db)
            DB_PATH="$2"
            shift 2
            ;;
        -c|--config)
            CONFIG_DIR="$2"
            shift 2
            ;;
        -a|--audit)
            AUDIT_DIR="$2"
            shift 2
            ;;
        -o|--output)
            BACKUP_DIR="$2"
            shift 2
            ;;
        -r|--retention)
            RETENTION_DAYS="$2"
            shift 2
            ;;
        --no-audit)
            INCLUDE_AUDIT="false"
            shift
            ;;
        --no-compress)
            COMPRESS="false"
            shift
            ;;
        --no-encrypt)
            ENCRYPT="false"
            shift
            ;;
        --no-verify)
            VERIFY="false"
            shift
            ;;
        -k|--key)
            ENCRYPTION_KEY="$2"
            shift 2
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            usage
            exit 1
            ;;
    esac
done

# Validate configuration
validate_config() {
    if [[ ! -f "$DB_PATH" ]]; then
        log_error "Database not found: $DB_PATH"
        exit 1
    fi

    if [[ "$ENCRYPT" == "true" && -z "$ENCRYPTION_KEY" ]]; then
        ENCRYPTION_KEY=$(openssl rand -base64 32)
        log_warn "No encryption key provided, generated temporary key"
        log_warn "SAVE THIS KEY TO DECRYPT THE BACKUP:"
        echo "================================================"
        echo "$ENCRYPTION_KEY"
        echo "================================================"
    fi

    # Check for required tools
    if ! command -v sqlite3 &> /dev/null; then
        log_error "sqlite3 is required but not installed"
        exit 1
    fi

    if [[ "$ENCRYPT" == "true" ]] && ! command -v openssl &> /dev/null; then
        log_error "openssl is required for encryption but not installed"
        exit 1
    fi
}

# Create backup directory
create_backup_dir() {
    local backup_path="$1"
    if [[ ! -d "$backup_path" ]]; then
        log_info "Creating backup directory: $backup_path"
        mkdir -p "$backup_path"
    fi
}

# Get backup filename
get_backup_filename() {
    local timestamp
    timestamp=$(date +%Y%m%d_%H%M%S)
    local suffix="tar"

    if [[ "$COMPRESS" == "true" ]]; then
        suffix="tar.gz"
    fi

    echo "apicerberus_full_${timestamp}.${suffix}"
}

# Backup database
backup_database() {
    local temp_dir="$1"
    local db_path="$2"

    log_info "Backing up database..."

    mkdir -p "${temp_dir}/database"

    # Create SQLite backup
    if sqlite3 "$db_path" ".backup '${temp_dir}/database/apicerberus.db'"; then
        log_info "  Database backup created"
    else
        log_error "  Failed to backup database"
        return 1
    fi

    # Copy WAL files if they exist
    if [[ -f "${db_path}-wal" ]]; then
        cp "${db_path}-wal" "${temp_dir}/database/"
        log_info "  WAL file copied"
    fi

    if [[ -f "${db_path}-shm" ]]; then
        cp "${db_path}-shm" "${temp_dir}/database/"
        log_info "  SHM file copied"
    fi

    # Copy schema migrations info
    sqlite3 "$db_path" "SELECT * FROM schema_migrations ORDER BY version;" > "${temp_dir}/database/schema_migrations.txt" 2>/dev/null || true
}

# Backup configuration
backup_configuration() {
    local temp_dir="$1"
    local config_dir="$2"

    log_info "Backing up configuration..."

    mkdir -p "${temp_dir}/config"

    # Copy YAML files
    find "$config_dir" -maxdepth 2 \( -name "*.yaml" -o -name "*.yml" \) -type f 2>/dev/null | while read -r file; do
        cp "$file" "${temp_dir}/config/"
    done || true

    # Copy JSON files
    find "$config_dir" -maxdepth 2 -name "*.json" -type f 2>/dev/null | while read -r file; do
        cp "$file" "${temp_dir}/config/"
    done || true

    # Copy environment files
    find "." -maxdepth 1 -name ".env*" -type f 2>/dev/null | while read -r file; do
        cp "$file" "${temp_dir}/config/"
    done || true

    # Copy SSL certificates
    if [[ -d "${config_dir}/ssl" ]]; then
        cp -r "${config_dir}/ssl" "${temp_dir}/config/"
    fi

    # Copy example config
    if [[ -f "apicerberus.example.yaml" ]]; then
        cp "apicerberus.example.yaml" "${temp_dir}/config/"
    fi

    log_info "  Configuration backup complete"
}

# Backup audit logs
backup_audit_logs() {
    local temp_dir="$1"
    local audit_dir="$2"

    if [[ "$INCLUDE_AUDIT" != "true" ]]; then
        log_info "Skipping audit logs backup (disabled)"
        return 0
    fi

    if [[ ! -d "$audit_dir" ]]; then
        log_warn "Audit directory not found: $audit_dir"
        return 0
    fi

    log_info "Backing up audit logs..."

    mkdir -p "${temp_dir}/audit"

    # Copy recent audit logs (last 30 days)
    find "$audit_dir" -type f -mtime -30 -exec cp {} "${temp_dir}/audit/" \; 2>/dev/null || true

    local count
    count=$(find "${temp_dir}/audit" -type f | wc -l)
    log_info "  Copied $count audit log files"
}

# Create metadata
create_metadata() {
    local temp_dir="$1"

    log_info "Creating backup metadata..."

    cat > "${temp_dir}/backup-info.txt" <<EOF
APICerebrus Full System Backup
==============================
Backup Date: $(date -Iseconds)
Hostname: $(hostname)
User: $(whoami)

Components:
  - Database: $DB_PATH
  - Configuration: $CONFIG_DIR
  - Audit Logs: $AUDIT_DIR (included: $INCLUDE_AUDIT)

Options:
  - Compressed: $COMPRESS
  - Encrypted: $ENCRYPT
  - Verified: $VERIFY

File Manifest:
$(find "$temp_dir" -type f | sort)

Database Info:
$(sqlite3 "$DB_PATH" "PRAGMA integrity_check;" 2>/dev/null || echo "Integrity check not available")
EOF

    # Add system info
    {
        echo ""
        echo "System Information:"
        echo "  OS: $(uname -a)"
        echo "  SQLite: $(sqlite3 --version 2>/dev/null || echo 'unknown')"
        echo "  Disk Usage: $(df -h . | tail -1)"
    } >> "${temp_dir}/backup-info.txt"
}

# Create backup archive
create_backup() {
    local temp_dir="$1"
    local backup_file="$2"

    log_info "Creating backup archive..."

    local tar_opts="-cf"
    [[ "$COMPRESS" == "true" ]] && tar_opts="-czf"

    if tar $tar_opts "$backup_file" -C "$temp_dir" .; then
        log_info "Archive created: $backup_file"
    else
        log_error "Failed to create archive"
        exit 1
    fi

    # Encrypt if enabled
    if [[ "$ENCRYPT" == "true" ]]; then
        log_info "Encrypting backup..."
        if openssl enc -aes-256-cbc -salt -in "$backup_file" -out "${backup_file}.enc" -k "$ENCRYPTION_KEY"; then
            mv "${backup_file}.enc" "$backup_file"
            log_info "Encryption complete"
        else
            log_error "Encryption failed"
            rm -f "$backup_file" "${backup_file}.enc"
            exit 1
        fi
    fi
}

# Verify backup
verify_backup() {
    local backup_file="$1"

    if [[ "$VERIFY" != "true" ]]; then
        return 0
    fi

    log_info "Verifying backup..."

    local temp_verify
    temp_verify=$(mktemp -d)
    trap 'rm -rf "$temp_verify"' RETURN

    # Decrypt if encrypted
    local verify_file="$backup_file"
    if [[ "$ENCRYPT" == "true" ]]; then
        if ! openssl enc -aes-256-cbc -d -in "$backup_file" -out "${temp_verify}/backup.tar.gz" -k "$ENCRYPTION_KEY" 2>/dev/null; then
            log_error "Backup verification failed: cannot decrypt"
            return 1
        fi
        verify_file="${temp_verify}/backup.tar.gz"
    fi

    # Extract and verify
    local tar_opts="-xf"
    [[ "$COMPRESS" == "true" ]] && tar_opts="-xzf"

    if ! tar $tar_opts "$verify_file" -C "$temp_verify" >/dev/null 2>&1; then
        log_error "Backup verification failed: cannot extract"
        return 1
    fi

    # Verify database integrity
    if [[ -f "${temp_verify}/database/apicerberus.db" ]]; then
        if ! sqlite3 "${temp_verify}/database/apicerberus.db" "PRAGMA integrity_check;" | grep -q "ok"; then
            log_error "Backup verification failed: database integrity check failed"
            return 1
        fi
    fi

    log_success "Backup verification passed"
}

# Cleanup old backups
cleanup_old_backups() {
    local backup_dir="$1"
    local retention_days="$2"

    log_info "Cleaning up backups older than $retention_days days"

    local deleted=0
    while IFS= read -r -d '' file; do
        log_info "Removing old backup: $file"
        rm -f "$file"
        ((deleted++))
    done < <(find "$backup_dir" -name "apicerberus_full_*.tar.gz*" -type f -mtime +$retention_days -print0 2>/dev/null || true)

    log_info "Removed $deleted old backup(s)"
}

# Create symlink to latest backup
update_latest_symlink() {
    local backup_dir="$1"
    local backup_file="$2"
    local latest_link="${backup_dir}/latest"

    rm -f "$latest_link"
    ln -s "$(basename "$backup_file")" "$latest_link"
}

# Main
main() {
    log_info "=========================================="
    log_info "APICerebrus Full System Backup Started"
    log_info "=========================================="

    validate_config

    # Create backup directory
    create_backup_dir "$BACKUP_DIR"

    # Generate backup filename
    local backup_filename
    backup_filename=$(get_backup_filename)
    local backup_file="${BACKUP_DIR}/${backup_filename}"

    # Create temp directory
    TEMP_DIR=$(mktemp -d)
    trap 'rm -rf "$TEMP_DIR"' EXIT

    # Perform backups
    backup_database "$TEMP_DIR" "$DB_PATH"
    backup_configuration "$TEMP_DIR" "$CONFIG_DIR"
    backup_audit_logs "$TEMP_DIR" "$AUDIT_DIR"
    create_metadata "$TEMP_DIR"

    # Create archive
    create_backup "$TEMP_DIR" "$backup_file"

    # Verify backup
    verify_backup "$backup_file"

    # Update latest symlink
    update_latest_symlink "$BACKUP_DIR" "$backup_file"

    # Cleanup old backups
    cleanup_old_backups "$BACKUP_DIR" "$RETENTION_DAYS"

    # Report
    local backup_size
    backup_size=$(du -h "$backup_file" | cut -f1)

    log_info "=========================================="
    log_success "Full System Backup Complete:"
    log_success "  Backup size: $backup_size"
    log_success "  Location: $backup_file"
    log_success "  Encrypted: $ENCRYPT"
    log_success "  Verified: $VERIFY"
    if [[ "$ENCRYPT" == "true" ]]; then
        log_warn "  Store encryption key securely!"
    fi
    log_info "=========================================="
}

# Run main function
trap 'log_error "Backup interrupted"; exit 1' INT TERM
main "$@"
