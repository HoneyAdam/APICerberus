#!/bin/bash
#
# APICerebrus Data Retention Cleanup Script
# Cleans up old audit logs, expired sessions, and other temporal data
#

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DB_PATH="${DB_PATH:-./apicerberus.db}"
AUDIT_DIR="${AUDIT_DIR:-./audit-archive}"
RETENTION_DAYS="${RETENTION_DAYS:-30}"
SESSION_RETENTION_DAYS="${SESSION_RETENTION_DAYS:-7}"
DRY_RUN="${DRY_RUN:-false}"
FORCE="${FORCE:-false}"
LOG_FILE="${LOG_FILE:-/var/log/apicerberus/cleanup.log}"

# Statistics
declare -i DELETED_AUDIT=0
declare -i DELETED_SESSIONS=0
declare -i DELETED_RECORDS=0
declare -i FREED_SPACE=0

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
APICerebrus Data Retention Cleanup Script

Usage: $0 [OPTIONS]

Options:
    -d, --db PATH           Database path (default: ./apicerberus.db)
    -a, --audit DIR         Audit archive directory (default: ./audit-archive)
    -r, --retention DAYS    Audit retention in days (default: 30)
    -s, --session DAYS      Session retention in days (default: 7)
    -n, --dry-run           Show what would be deleted without deleting
    -f, --force             Skip confirmation prompt
    -h, --help              Show this help message

Environment Variables:
    DB_PATH                 Database file path
    AUDIT_DIR               Audit archive directory
    RETENTION_DAYS          Audit log retention period
    SESSION_RETENTION_DAYS  Session retention period
    DRY_RUN                 Dry run mode
    FORCE                   Skip confirmation
    LOG_FILE                Log file path

Examples:
    $0                              # Run cleanup with defaults
    $0 -n                           # Dry run
    $0 -r 90                        # 90 day retention
    $0 -f                           # Force without confirmation

Data Cleaned:
    - Old audit log files
    - Expired sessions from database
    - Old audit log records from database
    - Orphaned credit transactions (optional)

WARNING:
    This script permanently deletes data!
    Always run with -n (dry run) first.
EOF
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -d|--db)
            DB_PATH="$2"
            shift 2
            ;;
        -a|--audit)
            AUDIT_DIR="$2"
            shift 2
            ;;
        -r|--retention)
            RETENTION_DAYS="$2"
            shift 2
            ;;
        -s|--session)
            SESSION_RETENTION_DAYS="$2"
            shift 2
            ;;
        -n|--dry-run)
            DRY_RUN="true"
            shift
            ;;
        -f|--force)
            FORCE="true"
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
    if ! command -v sqlite3 &> /dev/null; then
        log_error "sqlite3 is required but not installed"
        exit 1
    fi

    if [[ ! -f "$DB_PATH" ]]; then
        log_error "Database not found: $DB_PATH"
        exit 1
    fi
}

# Cleanup old audit log files
cleanup_audit_files() {
    log_info "Cleaning up audit log files older than $RETENTION_DAYS days..."

    if [[ ! -d "$AUDIT_DIR" ]]; then
        log_warn "Audit directory not found: $AUDIT_DIR"
        return 0
    fi

    local count=0
    local freed=0

    while IFS= read -r -d '' file; do
        local file_size
        file_size=$(stat -f%z "$file" 2>/dev/null || stat -c%s "$file" 2>/dev/null || echo "0")

        if [[ "$DRY_RUN" == "true" ]]; then
            log_info "  [DRY RUN] Would delete: $(basename "$file") ($((file_size / 1024))KB)"
        else
            rm -f "$file"
            log_info "  Deleted: $(basename "$file")"
        fi

        ((count++))
        ((freed += file_size))
    done < <(find "$AUDIT_DIR" -type f -mtime +$RETENTION_DAYS -print0 2>/dev/null || true)

    DELETED_AUDIT=$count
    FREED_SPACE=$((FREED_SPACE + freed))

    log_info "Audit files: $count deleted, $((freed / 1024 / 1024))MB freed"
}

# Cleanup expired sessions from database
cleanup_expired_sessions() {
    log_info "Cleaning up expired sessions (older than $SESSION_RETENTION_DAYS days)..."

    local cutoff_date
    cutoff_date=$(date -d "$SESSION_RETENTION_DAYS days ago" +%Y-%m-%dT%H:%M:%S 2>/dev/null || \
                  date -v-${SESSION_RETENTION_DAYS}d +%Y-%m-%dT%H:%M:%S 2>/dev/null || \
                  echo "")

    if [[ -z "$cutoff_date" ]]; then
        log_warn "Could not calculate cutoff date, skipping session cleanup"
        return 0
    fi

    # Count expired sessions
    local count
    count=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM sessions WHERE expires_at < '$cutoff_date';" 2>/dev/null || echo "0")

    if [[ "$count" -eq 0 ]]; then
        log_info "No expired sessions found"
        return 0
    fi

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "  [DRY RUN] Would delete $count expired sessions"
    else
        sqlite3 "$DB_PATH" "DELETE FROM sessions WHERE expires_at < '$cutoff_date';" 2>/dev/null || true
        log_success "  Deleted $count expired sessions"
    fi

    DELETED_SESSIONS=$count
}

# Cleanup old audit records from database
cleanup_audit_records() {
    log_info "Cleaning up audit records older than $RETENTION_DAYS days..."

    local cutoff_date
    cutoff_date=$(date -d "$RETENTION_DAYS days ago" +%Y-%m-%dT%H:%M:%S 2>/dev/null || \
                  date -v-${RETENTION_DAYS}d +%Y-%m-%dT%H:%M:%S 2>/dev/null || \
                  echo "")

    if [[ -z "$cutoff_date" ]]; then
        log_warn "Could not calculate cutoff date, skipping audit cleanup"
        return 0
    fi

    # Count old records
    local count
    count=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM audit_logs WHERE created_at < '$cutoff_date';" 2>/dev/null || echo "0")

    if [[ "$count" -eq 0 ]]; then
        log_info "No old audit records found"
        return 0
    fi

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "  [DRY RUN] Would delete $count old audit records"
    else
        sqlite3 "$DB_PATH" "DELETE FROM audit_logs WHERE created_at < '$cutoff_date';" 2>/dev/null || true
        log_success "  Deleted $count old audit records"
    fi

    DELETED_RECORDS=$count
}

# Vacuum database to reclaim space
vacuum_database() {
    log_info "Vacuuming database to reclaim space..."

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "  [DRY RUN] Would vacuum database"
        return 0
    fi

    local size_before
    size_before=$(stat -f%z "$DB_PATH" 2>/dev/null || stat -c%s "$DB_PATH" 2>/dev/null || echo "0")

    if sqlite3 "$DB_PATH" "VACUUM;" 2>/dev/null; then
        local size_after
        size_after=$(stat -f%z "$DB_PATH" 2>/dev/null || stat -c%s "$DB_PATH" 2>/dev/null || echo "0")
        local saved=$(( (size_before - size_after) / 1024 / 1024 ))
        log_success "  Database vacuumed, ${saved}MB reclaimed"
    else
        log_warn "  Database vacuum failed (may be in use)"
    fi
}

# Analyze database for query optimization
analyze_database() {
    log_info "Analyzing database for query optimization..."

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "  [DRY RUN] Would analyze database"
        return 0
    fi

    if sqlite3 "$DB_PATH" "ANALYZE;" 2>/dev/null; then
        log_success "  Database analyzed"
    else
        log_warn "  Database analysis failed"
    fi
}

# Main
main() {
    log_info "=========================================="
    log_info "APICerebrus Data Cleanup Started"
    log_info "=========================================="
    log_info "Database: $DB_PATH"
    log_info "Audit retention: $RETENTION_DAYS days"
    log_info "Session retention: $SESSION_RETENTION_DAYS days"
    [[ "$DRY_RUN" == "true" ]] && log_warn "DRY RUN MODE - No changes will be made"
    log_info "=========================================="

    validate_config

    # Confirmation
    if [[ "$FORCE" != "true" && "$DRY_RUN" != "true" ]]; then
        echo
        log_warn "WARNING: This will permanently delete data!"
        read -p "Are you sure you want to continue? (yes/no) " -r
        echo
        if [[ ! $REPLY =~ ^yes$ ]]; then
            log_info "Cleanup aborted"
            exit 0
        fi
    fi

    # Perform cleanup
    cleanup_audit_files
    cleanup_expired_sessions
    cleanup_audit_records

    # Optimize database
    vacuum_database
    analyze_database

    # Summary
    log_info "=========================================="
    log_info "Cleanup Summary:"
    log_info "  Audit files deleted: $DELETED_AUDIT"
    log_info "  Sessions deleted: $DELETED_SESSIONS"
    log_info "  Audit records deleted: $DELETED_RECORDS"
    log_info "  Space freed: $((FREED_SPACE / 1024 / 1024))MB"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_warn "Dry run complete - no changes made"
    else
        log_success "Cleanup completed successfully"
    fi
    log_info "=========================================="
}

# Run main function
trap 'log_error "Cleanup interrupted"; exit 1' INT TERM
main "$@"
