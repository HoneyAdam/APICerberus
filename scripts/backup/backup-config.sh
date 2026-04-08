#!/bin/bash
#
# APICerebrus Configuration Backup Script
# Backs up configuration files and environment settings
#

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_DIR="${CONFIG_DIR:-./config}"
BACKUP_DIR="${BACKUP_DIR:-./backups/config}"
RETENTION_DAYS="${RETENTION_DAYS:-90}"
INCLUDE_ENV="${INCLUDE_ENV:-true}"
INCLUDE_SECRETS="${INCLUDE_SECRETS:-false}"
ENCRYPT="${ENCRYPT:-true}"
ENCRYPTION_KEY="${ENCRYPTION_KEY:-}"
LOG_FILE="${LOG_FILE:-/var/log/apicerberus/backup-config.log}"

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
APICerebrus Configuration Backup Script

Usage: $0 [OPTIONS]

Options:
    -c, --config DIR        Config directory (default: ./config)
    -o, --output DIR        Backup directory (default: ./backups/config)
    -r, --retention DAYS    Retention period in days (default: 90)
    --no-env                Don't include environment files
    --include-secrets       Include secret files (use with caution!)
    --no-encrypt            Don't encrypt backup
    -k, --key KEY           Encryption key
    -h, --help              Show this help message

Environment Variables:
    CONFIG_DIR              Configuration directory
    BACKUP_DIR              Backup output directory
    RETENTION_DAYS          Backup retention period
    INCLUDE_ENV             Include .env files (default: true)
    INCLUDE_SECRETS         Include secret files (default: false)
    ENCRYPT                 Encrypt backup (default: true)
    ENCRYPTION_KEY          Encryption key
    LOG_FILE                Log file path

Examples:
    $0                              # Backup with defaults
    $0 -c /etc/apicerberus          # Backup specific config dir
    $0 --include-secrets            # Include secrets (encrypted)
    $0 -k "mysecretkey"             # Use specific encryption key

Backup Contents:
    - YAML configuration files
    - JSON configuration files
    - Environment files (.env)
    - SSL certificates (if present)
    - Custom plugin configs

Security Notes:
    - Backups are encrypted by default
    - Secret inclusion is opt-in for safety
    - Store encryption keys separately from backups
EOF
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -c|--config)
            CONFIG_DIR="$2"
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
        --no-env)
            INCLUDE_ENV="false"
            shift
            ;;
        --include-secrets)
            INCLUDE_SECRETS="true"
            shift
            ;;
        --no-encrypt)
            ENCRYPT="false"
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
    if [[ ! -d "$CONFIG_DIR" ]]; then
        log_warn "Config directory not found: $CONFIG_DIR"
        log_info "Will backup default config files from current directory"
        CONFIG_DIR="."
    fi

    if [[ "$ENCRYPT" == "true" && -z "$ENCRYPTION_KEY" ]]; then
        # Generate a random key if not provided
        ENCRYPTION_KEY=$(openssl rand -base64 32)
        log_warn "No encryption key provided, generated temporary key"
        log_warn "SAVE THIS KEY TO DECRYPT THE BACKUP:"
        echo "================================================"
        echo "$ENCRYPTION_KEY"
        echo "================================================"
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
    echo "apicerberus_config_${timestamp}.tar.gz"
}

# Collect configuration files
collect_configs() {
    local temp_dir="$1"
    local config_dir="$2"

    log_info "Collecting configuration files..."

    # Create temp config directory
    mkdir -p "${temp_dir}/config"

    # Find and copy YAML files
    find "$config_dir" -maxdepth 2 -name "*.yaml" -o -name "*.yml" 2>/dev/null | while read -r file; do
        if [[ -f "$file" ]]; then
            cp "$file" "${temp_dir}/config/"
            log_info "  Added: $file"
        fi
    done || true

    # Find and copy JSON files
    find "$config_dir" -maxdepth 2 -name "*.json" 2>/dev/null | while read -r file; do
        if [[ -f "$file" ]]; then
            cp "$file" "${temp_dir}/config/"
            log_info "  Added: $file"
        fi
    done || true

    # Copy environment files if enabled
    if [[ "$INCLUDE_ENV" == "true" ]]; then
        find "." -maxdepth 1 -name ".env*" -type f 2>/dev/null | while read -r file; do
            cp "$file" "${temp_dir}/config/"
            log_info "  Added: $file"
        done || true
    fi

    # Copy SSL certificates if present
    if [[ -d "${config_dir}/ssl" ]]; then
        mkdir -p "${temp_dir}/config/ssl"
        cp -r "${config_dir}/ssl/"* "${temp_dir}/config/ssl/" 2>/dev/null || true
        log_info "  Added: SSL certificates"
    fi

    # Copy example config from root if exists
    if [[ -f "apicerberus.example.yaml" ]]; then
        cp "apicerberus.example.yaml" "${temp_dir}/config/"
        log_info "  Added: apicerberus.example.yaml"
    fi

    # Include secrets if enabled (with warning)
    if [[ "$INCLUDE_SECRETS" == "true" ]]; then
        log_warn "INCLUDING SECRET FILES IN BACKUP"
        if [[ -d "${config_dir}/secrets" ]]; then
            mkdir -p "${temp_dir}/config/secrets"
            cp -r "${config_dir}/secrets/"* "${temp_dir}/config/secrets/" 2>/dev/null || true
            log_info "  Added: Secrets directory"
        fi
    fi

    # Create metadata file
    cat > "${temp_dir}/backup-metadata.txt" <<EOF
APICerebrus Configuration Backup
================================
Backup Date: $(date -Iseconds)
Hostname: $(hostname)
Config Directory: $config_dir
Include Environment: $INCLUDE_ENV
Include Secrets: $INCLUDE_SECRETS
Encrypted: $ENCRYPT

Files Included:
$(find "${temp_dir}/config" -type f | sed 's|^'"${temp_dir}"'/config/||' | sort)
EOF

    log_info "Configuration collection complete"
}

# Create backup archive
create_backup() {
    local temp_dir="$1"
    local backup_file="$2"

    log_info "Creating backup archive..."

    # Create tar.gz archive
    if tar -czf "$backup_file" -C "$temp_dir" .; then
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
    done < <(find "$backup_dir" -name "apicerberus_config_*.tar.gz*" -type f -mtime +$retention_days -print0 2>/dev/null || true)

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
    log_info "APICerebrus Configuration Backup Started"
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

    # Collect configurations
    collect_configs "$TEMP_DIR" "$CONFIG_DIR"

    # Create backup
    create_backup "$TEMP_DIR" "$backup_file"

    # Update latest symlink
    update_latest_symlink "$BACKUP_DIR" "$backup_file"

    # Cleanup old backups
    cleanup_old_backups "$BACKUP_DIR" "$RETENTION_DAYS"

    # Report
    local backup_size
    backup_size=$(du -h "$backup_file" | cut -f1)

    log_info "=========================================="
    log_success "Configuration Backup Summary:"
    log_success "  Backup size: $backup_size"
    log_success "  Location: $backup_file"
    log_success "  Encrypted: $ENCRYPT"
    if [[ "$ENCRYPT" == "true" ]]; then
        log_warn "  Store encryption key securely!"
    fi
    log_info "=========================================="
}

# Run main function
trap 'log_error "Backup interrupted"; exit 1' INT TERM
main "$@"
