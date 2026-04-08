#!/bin/bash
#
# APICerebrus Restore Script
# Restores database from backup
#

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKUP_FILE="${BACKUP_FILE:-}"
RESTORE_DIR="${RESTORE_DIR:-./restore}"
DB_PATH="${DB_PATH:-./apicerberus.db}"
ENCRYPTION_KEY="${ENCRYPTION_KEY:-}"
FORCE="${FORCE:-false}"
DRY_RUN="${DRY_RUN:-false}"
LOG_FILE="${LOG_FILE:-/var/log/apicerberus/restore.log}"

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
APICerebrus Restore Script

Usage: $0 [OPTIONS] -f BACKUP_FILE

Options:
    -f, --file FILE         Backup file to restore (required)
    -d, --db PATH           Database path (default: ./apicerberus.db)
    -o, --output DIR        Restore working directory (default: ./restore)
    -k, --key KEY           Encryption key for encrypted backups
    --force                 Skip confirmation prompts
    -n, --dry-run           Show what would be restored without restoring
    -h, --help              Show this help message

Environment Variables:
    BACKUP_FILE             Backup file path
    DB_PATH                 Target database path
    RESTORE_DIR             Restore working directory
    ENCRYPTION_KEY          Decryption key
    FORCE                   Skip confirmations
    DRY_RUN                 Dry run mode
    LOG_FILE                Log file path

Examples:
    $0 -f backup.db.gz                    # Restore from backup
    $0 -f backup.db.gz -d /data/db.db    # Restore to specific location
    $0 -f backup.db.enc -k "secretkey"   # Restore encrypted backup
    $0 -f backup.tar.gz -n               # Dry run

Restore Types:
    - SQLite database (.db, .db.gz, .db.enc)
    - Configuration archive (.tar.gz, .tar.gz.enc)
    - Full system backup (.tar.gz, .tar.gz.enc)

WARNING:
    Restoring will OVERWRITE existing data!
    Always backup current data before restoring.
EOF
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -f|--file)
            BACKUP_FILE="$2"
            shift 2
            ;;
        -d|--db)
            DB_PATH="$2"
            shift 2
            ;;
        -o|--output)
            RESTORE_DIR="$2"
            shift 2
            ;;
        -k|--key)
            ENCRYPTION_KEY="$2"
            shift 2
            ;;
        --force)
            FORCE="true"
            shift
            ;;
        -n|--dry-run)
            DRY_RUN="true"
            shift
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
    if [[ -z "$BACKUP_FILE" ]]; then
        log_error "Backup file is required (use -f)"
        usage
        exit 1
    fi

    if [[ ! -f "$BACKUP_FILE" ]]; then
        log_error "Backup file not found: $BACKUP_FILE"
        exit 1
    fi

    # Check for required tools
    if ! command -v sqlite3 &> /dev/null; then
        log_error "sqlite3 is required but not installed"
        exit 1
    fi

    # Detect backup type
    local filename
    filename=$(basename "$BACKUP_FILE")

    if [[ "$filename" == *.enc ]]; then
        if [[ -z "$ENCRYPTION_KEY" ]]; then
            log_error "Encrypted backup requires encryption key (use -k)"
            exit 1
        fi
        if ! command -v openssl &> /dev/null; then
            log_error "openssl is required for decryption but not installed"
            exit 1
        fi
    fi
}

# Detect backup type
detect_backup_type() {
    local file="$1"
    local filename
    filename=$(basename "$file")

    if [[ "$filename" == apicerberus_full_* ]]; then
        echo "full"
    elif [[ "$filename" == apicerberus_config_* ]]; then
        echo "config"
    elif [[ "$filename" == apicerberus_*.db* ]]; then
        echo "database"
    else
        echo "unknown"
    fi
}

# Decrypt backup if needed
decrypt_backup() {
    local input_file="$1"
    local output_file="$2"

    if [[ "$input_file" == *.enc ]]; then
        log_info "Decrypting backup..."
        if openssl enc -aes-256-cbc -d -in "$input_file" -out "$output_file" -k "$ENCRYPTION_KEY"; then
            log_info "Decryption successful"
            echo "$output_file"
        else
            log_error "Decryption failed - check your encryption key"
            exit 1
        fi
    else
        echo "$input_file"
    fi
}

# Decompress backup if needed
decompress_backup() {
    local input_file="$1"
    local output_dir="$2"

    if [[ "$input_file" == *.gz || "$input_file" == *.tar.gz ]]; then
        log_info "Decompressing backup..."

        if [[ "$input_file" == *.tar.gz ]]; then
            # Extract tar.gz
            if tar -xzf "$input_file" -C "$output_dir"; then
                log_info "Extraction successful"
            else
                log_error "Extraction failed"
                exit 1
            fi
        else
            # Gunzip single file
            if gunzip -c "$input_file" > "${output_file}/backup.db"; then
                log_info "Decompression successful"
            else
                log_error "Decompression failed"
                exit 1
            fi
        fi
    else
        # Copy as-is
        cp "$input_file" "$output_dir/"
    fi
}

# Restore database
restore_database() {
    local source_file="$1"
    local target_db="$2"

    log_info "Restoring database..."

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY RUN] Would restore database to: $target_db"
        return 0
    fi

    # Backup current database if exists
    if [[ -f "$target_db" ]]; then
        local backup_name="${target_db}.backup.$(date +%Y%m%d_%H%M%S)"
        log_warn "Backing up current database to: $backup_name"
        cp "$target_db" "$backup_name"
    fi

    # Verify source is valid SQLite
    if ! sqlite3 "$source_file" "PRAGMA integrity_check;" | grep -q "ok"; then
        log_error "Source file is not a valid SQLite database"
        exit 1
    fi

    # Copy database
    cp "$source_file" "$target_db"

    log_success "Database restored to: $target_db"
}

# Restore configuration
restore_configuration() {
    local source_dir="$1"

    log_info "Restoring configuration..."

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY RUN] Would restore configuration files"
        return 0
    fi

    # Backup current config
    if [[ -d "config" ]]; then
        local backup_name="config.backup.$(date +%Y%m%d_%H%M%S)"
        log_warn "Backing up current config to: $backup_name"
        cp -r "config" "$backup_name"
    fi

    # Restore config files
    if [[ -d "${source_dir}/config" ]]; then
        mkdir -p "config"
        cp -r "${source_dir}/config/"* "config/"
        log_success "Configuration restored"
    else
        log_warn "No configuration found in backup"
    fi
}

# Restore full backup
restore_full() {
    local source_dir="$1"

    log_info "Restoring full system backup..."

    # Restore database
    if [[ -f "${source_dir}/database/apicerberus.db" ]]; then
        restore_database "${source_dir}/database/apicerberus.db" "$DB_PATH"

        # Restore WAL files if present
        if [[ -f "${source_dir}/database/apicerberus.db-wal" ]]; then
            cp "${source_dir}/database/apicerberus.db-wal" "${DB_PATH}-wal"
        fi
        if [[ -f "${source_dir}/database/apicerberus.db-shm" ]]; then
            cp "${source_dir}/database/apicerberus.db-shm" "${DB_PATH}-shm"
        fi
    fi

    # Restore configuration
    restore_configuration "$source_dir"

    # Restore audit logs
    if [[ -d "${source_dir}/audit" ]]; then
        mkdir -p "audit-archive"
        cp -r "${source_dir}/audit/"* "audit-archive/"
        log_success "Audit logs restored"
    fi
}

# Main
main() {
    log_info "=========================================="
    log_info "APICerebrus Restore Started"
    log_info "=========================================="

    validate_config

    local backup_type
    backup_type=$(detect_backup_type "$BACKUP_FILE")
    log_info "Detected backup type: $backup_type"

    # Confirmation
    if [[ "$FORCE" != "true" && "$DRY_RUN" != "true" ]]; then
        echo
        log_warn "WARNING: This will OVERWRITE existing data!"
        log_warn "Backup file: $BACKUP_FILE"
        log_warn "Target database: $DB_PATH"
        echo
        read -p "Are you sure you want to continue? (yes/no) " -r
        echo
        if [[ ! $REPLY =~ ^yes$ ]]; then
            log_info "Restore aborted"
            exit 0
        fi
    fi

    # Create restore directory
    mkdir -p "$RESTORE_DIR"

    # Decrypt if needed
    local decrypted_file="$BACKUP_FILE"
    if [[ "$BACKUP_FILE" == *.enc ]]; then
        decrypted_file="${RESTORE_DIR}/decrypted_backup"
        decrypted_file=$(decrypt_backup "$BACKUP_FILE" "$decrypted_file")
    fi

    # Extract/decompress
    local extract_dir="${RESTORE_DIR}/extracted"
    mkdir -p "$extract_dir"
    decompress_backup "$decrypted_file" "$extract_dir"

    # Perform restore based on type
    case "$backup_type" in
        database)
            local db_file
            db_file=$(find "$extract_dir" -name "*.db" -type f | head -1)
            if [[ -n "$db_file" ]]; then
                restore_database "$db_file" "$DB_PATH"
            else
                log_error "No database file found in backup"
                exit 1
            fi
            ;;
        config)
            restore_configuration "$extract_dir"
            ;;
        full)
            restore_full "$extract_dir"
            ;;
        *)
            # Try to auto-detect
            if [[ -f "${extract_dir}/database/apicerberus.db" ]]; then
                restore_full "$extract_dir"
            elif [[ -d "${extract_dir}/config" ]]; then
                restore_configuration "$extract_dir"
            else
                local db_file
                db_file=$(find "$extract_dir" -name "*.db" -type f | head -1)
                if [[ -n "$db_file" ]]; then
                    restore_database "$db_file" "$DB_PATH"
                else
                    log_error "Could not determine backup type"
                    exit 1
                fi
            fi
            ;;
    esac

    # Cleanup
    if [[ "$DRY_RUN" != "true" ]]; then
        rm -rf "$RESTORE_DIR"
    fi

    log_info "=========================================="
    if [[ "$DRY_RUN" == "true" ]]; then
        log_warn "DRY RUN - No changes made"
    else
        log_success "Restore completed successfully"
    fi
    log_info "=========================================="
}

# Run main function
trap 'log_error "Restore interrupted"; exit 1' INT TERM
main "$@"
