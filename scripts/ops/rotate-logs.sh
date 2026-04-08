#!/bin/bash
#
# APICerebrus Log Rotation Script
# Manages log file rotation and cleanup
#

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOG_DIR="${LOG_DIR:-/var/log/apicerberus}"
RETENTION_DAYS="${RETENTION_DAYS:-30}"
MAX_SIZE_MB="${MAX_SIZE_MB:-100}"
COMPRESS_AFTER_DAYS="${COMPRESS_AFTER_DAYS:-7}"
DRY_RUN="${DRY_RUN:-false}"
LOG_FILE="${LOG_FILE:-/var/log/apicerberus/rotate-logs.log}"

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
APICerebrus Log Rotation Script

Usage: $0 [OPTIONS] [COMMAND]

Commands:
    rotate          Rotate and compress logs (default)
    cleanup         Remove old logs only
    status          Show log directory status

Options:
    -d, --dir DIR           Log directory (default: /var/log/apicerberus)
    -r, --retention DAYS    Retention period in days (default: 30)
    -s, --size SIZE         Max log size in MB before rotation (default: 100)
    -c, --compress DAYS     Compress logs older than N days (default: 7)
    -n, --dry-run           Show what would be done without doing it
    -h, --help              Show this help message

Environment Variables:
    LOG_DIR                 Log directory path
    RETENTION_DAYS          Log retention period
    MAX_SIZE_MB             Maximum log size in MB
    COMPRESS_AFTER_DAYS     Compress logs after N days
    DRY_RUN                 Dry run mode
    LOG_FILE                Script log file

Examples:
    $0                              # Rotate logs with defaults
    $0 cleanup                      # Just cleanup old logs
    $0 status                       # Show log status
    $0 -r 7                         # 7 day retention
    $0 -n                           # Dry run

Log Rotation Strategy:
    1. Rotate logs larger than MAX_SIZE_MB
    2. Compress logs older than COMPRESS_AFTER_DAYS
    3. Delete logs older than RETENTION_DAYS
EOF
}

# Parse arguments
COMMAND="${1:-rotate}"
shift || true

while [[ $# -gt 0 ]]; do
    case $1 in
        -d|--dir)
            LOG_DIR="$2"
            shift 2
            ;;
        -r|--retention)
            RETENTION_DAYS="$2"
            shift 2
            ;;
        -s|--size)
            MAX_SIZE_MB="$2"
            shift 2
            ;;
        -c|--compress)
            COMPRESS_AFTER_DAYS="$2"
            shift 2
            ;;
        -n|--dry-run)
            DRY_RUN="true"
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        rotate|cleanup|status)
            COMMAND="$1"
            shift
            ;;
        *)
            log_error "Unknown option: $1"
            usage
            exit 1
            ;;
    esac
done

# Convert sizes
MAX_SIZE_BYTES=$((MAX_SIZE_MB * 1024 * 1024))

# Rotate a single log file
rotate_log() {
    local log_file="$1"
    local base_name
    base_name=$(basename "$log_file")
    local timestamp
    timestamp=$(date +%Y%m%d_%H%M%S)
    local rotated_name="${log_file}.${timestamp}"

    log_info "Rotating: $base_name"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "  [DRY RUN] Would rotate $base_name"
        return 0
    fi

    # Copy to rotated name and truncate original
    if cp "$log_file" "$rotated_name" && truncate -s 0 "$log_file"; then
        log_success "  Rotated: $base_name -> ${base_name}.${timestamp}"

        # Compress immediately if configured
        if [[ "$COMPRESS_AFTER_DAYS" -le 0 ]]; then
            gzip "$rotated_name"
            log_success "  Compressed: ${base_name}.${timestamp}.gz"
        fi
    else
        log_error "  Failed to rotate: $base_name"
        return 1
    fi
}

# Compress old logs
compress_old_logs() {
    log_info "Compressing logs older than $COMPRESS_AFTER_DAYS days..."

    local compressed=0
    while IFS= read -r -d '' file; do
        if [[ "$file" != *.gz && "$file" != *.bz2 ]]; then
            if [[ "$DRY_RUN" == "true" ]]; then
                log_info "  [DRY RUN] Would compress: $(basename "$file")"
            else
                if gzip "$file"; then
                    log_success "  Compressed: $(basename "$file").gz"
                    ((compressed++))
                else
                    log_error "  Failed to compress: $(basename "$file")"
                fi
            fi
        fi
    done < <(find "$LOG_DIR" -name "*.log.*" -type f -mtime +$COMPRESS_AFTER_DAYS -not -name "*.gz" -not -name "*.bz2" -print0 2>/dev/null || true)

    log_info "Compressed $compressed log file(s)"
}

# Cleanup old logs
cleanup_old_logs() {
    log_info "Removing logs older than $RETENTION_DAYS days..."

    local deleted=0
    while IFS= read -r -d '' file; do
        if [[ "$DRY_RUN" == "true" ]]; then
            log_info "  [DRY RUN] Would delete: $(basename "$file")"
        else
            rm -f "$file"
            log_info "  Deleted: $(basename "$file")"
            ((deleted++))
        fi
    done < <(find "$LOG_DIR" -name "*.log.*" -type f -mtime +$RETENTION_DAYS -print0 2>/dev/null || true)

    log_info "Removed $deleted old log file(s)"
}

# Rotate large logs
rotate_large_logs() {
    log_info "Checking for logs larger than ${MAX_SIZE_MB}MB..."

    local rotated=0
    while IFS= read -r -d '' file; do
        local size
        size=$(stat -f%z "$file" 2>/dev/null || stat -c%s "$file" 2>/dev/null || echo "0")

        if [[ $size -gt $MAX_SIZE_BYTES ]]; then
            local size_mb=$((size / 1024 / 1024))
            log_info "  Found large log: $(basename "$file") (${size_mb}MB)"
            if rotate_log "$file"; then
                ((rotated++))
            fi
        fi
    done < <(find "$LOG_DIR" -name "*.log" -type f -not -name "*.log.*" -print0 2>/dev/null || true)

    log_info "Rotated $ rotated large log file(s)"
}

# Show status
show_status() {
    echo "APICerebrus Log Directory Status"
    echo "================================"
    echo "Log Directory: $LOG_DIR"
    echo ""

    if [[ ! -d "$LOG_DIR" ]]; then
        log_error "Log directory not found: $LOG_DIR"
        exit 1
    fi

    # Directory size
    local total_size
    total_size=$(du -sh "$LOG_DIR" 2>/dev/null | cut -f1 || echo "unknown")
    echo "Total Size: $total_size"
    echo ""

    # Log files summary
    echo "Log Files:"
    printf "%-40s %10s %10s\n" "Filename" "Size" "Age"
    echo "-------------------------------------------------------------"

    find "$LOG_DIR" -name "*.log*" -type f -printf '%T@ %p\n' 2>/dev/null | \
        sort -rn | \
        while read -r mtime filepath; do
            local filename
            filename=$(basename "$filepath")
            local size
            size=$(du -h "$filepath" 2>/dev/null | cut -f1 || echo "?")
            local age
            age=$(( ($(date +%s) - ${mtime%.*}) / 86400 ))
            printf "%-40s %10s %9sd\n" "$filename" "$size" "$age"
        done || true

    echo ""
    echo "Configuration:"
    echo "  Retention: $RETENTION_DAYS days"
    echo "  Max Size: ${MAX_SIZE_MB}MB"
    echo "  Compress after: $COMPRESS_AFTER_DAYS days"
}

# Main
main() {
    log_info "=========================================="
    log_info "APICerebrus Log Rotation Started"
    log_info "Command: $COMMAND"
    [[ "$DRY_RUN" == "true" ]] && log_warn "DRY RUN MODE"
    log_info "=========================================="

    # Check log directory
    if [[ ! -d "$LOG_DIR" ]]; then
        log_warn "Log directory not found: $LOG_DIR"
        log_info "Creating log directory..."
        if [[ "$DRY_RUN" != "true" ]]; then
            mkdir -p "$LOG_DIR"
        fi
    fi

    case "$COMMAND" in
        rotate)
            rotate_large_logs
            compress_old_logs
            cleanup_old_logs
            ;;
        cleanup)
            cleanup_old_logs
            ;;
        status)
            show_status
            ;;
        *)
            log_error "Unknown command: $COMMAND"
            usage
            exit 1
            ;;
    esac

    log_info "=========================================="
    if [[ "$DRY_RUN" == "true" ]]; then
        log_warn "Dry run complete - no changes made"
    else
        log_success "Log rotation complete"
    fi
    log_info "=========================================="
}

main "$@"
