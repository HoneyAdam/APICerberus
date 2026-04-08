#!/bin/bash
#
# APICerebrus SQLite Backup Script
# Creates compressed, timestamped backups of the SQLite database
#

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DB_PATH="${DB_PATH:-./apicerberus.db}"
BACKUP_DIR="${BACKUP_DIR:-./backups}"
RETENTION_DAYS="${RETENTION_DAYS:-30}"
COMPRESS="${COMPRESS:-true}"
VERIFY="${VERIFY:-true}"
ENCRYPT="${ENCRYPT:-false}"
ENCRYPTION_KEY="${ENCRYPTION_KEY:-}"
LOG_FILE="${LOG_FILE:-/var/log/apicerberus/backup.log}"

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
APICerebrus SQLite Backup Script

Usage: $0 [OPTIONS]

Options:
    -d, --db PATH           Database path (default: ./apicerberus.db)
    -o, --output DIR        Backup directory (default: ./backups)
    -r, --retention DAYS    Retention period in days (default: 30)
    -n, --no-compress       Don't compress backup
    -v, --no-verify         Skip backup verification
    -e, --encrypt           Encrypt backup (requires ENCRYPTION_KEY)
    -k, --key KEY           Encryption key
    -h, --help              Show this help message

Environment Variables:
    DB_PATH                 Database file path
    BACKUP_DIR              Backup directory
    RETENTION_DAYS          Backup retention period
    COMPRESS                Set to 'false' to disable compression
    VERIFY                  Set to 'false' to skip verification
    ENCRYPT                 Set to 'true' to enable encryption
    ENCRYPTION_KEY          Encryption key (or use -k)
    LOG_FILE                Log file path

Examples:
    $0                                  # Create backup with defaults
    $0 -d /data/apicerberus.db         # Backup specific database
    $0 -o /backups/apicerberus         # Custom backup directory
    $0 -r 7                            # 7 day retention
    $0 -e -k "mysecretkey"             # Encrypted backup

Backup Naming:
    apicerberus_YYYYMMDD_HHMMSS.db[.gz][.enc]
EOF
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -d|--db)
            DB_PATH="$2"
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
        -n|--no-compress)
            COMPRESS="false"
            shift
            ;;
        -v|--no-verify)
            VERIFY="false"
            shift
            ;;
        -e|--encrypt)
            ENCRYPT="true"
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
        log_error "Encryption enabled but no key provided"
        log_error "Use -k option or set ENCRYPTION_KEY environment variable"
        exit 1
    fi

    # Check for required tools
    if ! command -v sqlite3 &> /dev/null; then
        log_error "sqlite3 is required but not installed"
        exit 1
    fi

    if [[ "$COMPRESS" == "true" ]] && ! command -v gzip &> /dev/null; then
        log_warn "gzip not found, compression disabled"
        COMPRESS="false"
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

    # Create subdirectories
    mkdir -p "${backup_path}/daily"
    mkdir -p "${backup_path}/weekly"
    mkdir -p "${backup_path}/monthly"
}

# Get backup filename
get_backup_filename() {
    local timestamp
    timestamp=$(date +%Y%m%d_%H%M%S)
    local filename="apicerberus_${timestamp}.db"

    if [[ "$COMPRESS" == "true" ]]; then
        filename="${filename}.gz"
    fi

    if [[ "$ENCRYPT" == "true" ]]; then
        filename="${filename}.enc"
    fi

    echo "$filename"
}

# Get backup type (daily/weekly/monthly)
get_backup_type() {
    local day_of_month
    day_of_month=$(date +%d)
    local day_of_week
    day_of_week=$(date +%u)

    if [[ "$day_of_month" == "01" ]]; then
        echo "monthly"
    elif [[ "$day_of_week" == "7" ]]; then
        echo "weekly"
    else
        echo "daily"
    fi
}

# Create backup
create_backup() {
    local db_path="$1"
    local backup_file="$2"
    local temp_file="${backup_file}.tmp"

    log_info "Starting database backup"
    log_info "Source: $db_path"
    log_info "Destination: $backup_file"

    # Create backup using SQLite backup API (online backup)
    if sqlite3 "$db_path" ".backup '${temp_file}'"; then
        log_info "Database backup created successfully"
    else
        log_error "Failed to create database backup"
        rm -f "$temp_file"
        exit 1
    fi

    # Compress if enabled
    if [[ "$COMPRESS" == "true" ]]; then
        log_info "Compressing backup..."
        if gzip -c "$temp_file" > "${temp_file}.gz"; then
            mv "${temp_file}.gz" "$temp_file"
            log_info "Compression complete"
        else
            log_error "Compression failed"
            rm -f "$temp_file" "${temp_file}.gz"
            exit 1
        fi
    fi

    # Encrypt if enabled
    if [[ "$ENCRYPT" == "true" ]]; then
        log_info "Encrypting backup..."
        if openssl enc -aes-256-cbc -salt -in "$temp_file" -out "${temp_file}.enc" -k "$ENCRYPTION_KEY"; then
            mv "${temp_file}.enc" "$temp_file"
            log_info "Encryption complete"
        else
            log_error "Encryption failed"
            rm -f "$temp_file" "${temp_file}.enc"
            exit 1
        fi
    fi

    # Move to final location
    mv "$temp_file" "$backup_file"

    # Verify backup
    if [[ "$VERIFY" == "true" ]]; then
        verify_backup "$backup_file"
    fi

    log_success "Backup completed: $backup_file"
}

# Verify backup
verify_backup() {
    local backup_file="$1"
    local temp_verify="${backup_file}.verify"

    log_info "Verifying backup..."

    # Decrypt if encrypted
    local verify_file="$backup_file"
    if [[ "$ENCRYPT" == "true" ]]; then
        if ! openssl enc -aes-256-cbc -d -in "$backup_file" -out "$temp_verify" -k "$ENCRYPTION_KEY" 2>/dev/null; then
            log_error "Backup verification failed: cannot decrypt"
            rm -f "$temp_verify"
            exit 1
        fi
        verify_file="$temp_verify"
    fi

    # Decompress if compressed
    if [[ "$COMPRESS" == "true" ]]; then
        local decomp_file="${temp_verify}.db"
        if [[ "$ENCRYPT" == "true" ]]; then
            if ! gunzip -c "$temp_verify" > "$decomp_file" 2>/dev/null; then
                log_error "Backup verification failed: cannot decompress"
                rm -f "$temp_verify" "$decomp_file"
                exit 1
            fi
        else
            if ! gunzip -c "$backup_file" > "$decomp_file" 2>/dev/null; then
                log_error "Backup verification failed: cannot decompress"
                rm -f "$decomp_file"
                exit 1
            fi
        fi
        verify_file="$decomp_file"
    fi

    # Verify SQLite integrity
    if ! sqlite3 "$verify_file" "PRAGMA integrity_check;" | grep -q "ok"; then
        log_error "Backup verification failed: integrity check failed"
        rm -f "$temp_verify" "${temp_verify}.db" "$decomp_file"
        exit 1
    fi

    # Cleanup temp files
    rm -f "$temp_verify" "${temp_verify}.db"

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
    done < <(find "$backup_dir" -name "apicerberus_*.db*" -type f -mtime +$retention_days -print0 2>/dev/null || true)

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
    log_info "APICerebrus Database Backup Started"
    log_info "=========================================="

    validate_config

    # Create backup directory structure
    create_backup_dir "$BACKUP_DIR"

    # Determine backup type and location
    local backup_type
    backup_type=$(get_backup_type)
    local backup_subdir="${BACKUP_DIR}/${backup_type}"

    # Generate backup filename
    local backup_filename
    backup_filename=$(get_backup_filename)
    local backup_file="${backup_subdir}/${backup_filename}"

    # Create backup
    create_backup "$DB_PATH" "$backup_file"

    # Update latest symlink
    update_latest_symlink "$backup_subdir" "$backup_file"

    # Cleanup old backups
    cleanup_old_backups "$BACKUP_DIR" "$RETENTION_DAYS"

    # Report
    local backup_size
    backup_size=$(du -h "$backup_file" | cut -f1)
    local db_size
    db_size=$(du -h "$DB_PATH" | cut -f1)

    log_info "=========================================="
    log_success "Backup Summary:"
    log_success "  Database size: $db_size"
    log_success "  Backup size: $backup_size"
    log_success "  Backup type: $backup_type"
    log_success "  Location: $backup_file"
    log_info "=========================================="
}

# Run main function
trap 'log_error "Backup interrupted"; exit 1' INT TERM
main "$@"
