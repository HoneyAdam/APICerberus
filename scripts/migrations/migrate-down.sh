#!/bin/bash
#
# APICerebrus Database Migration Script - DOWN
# Rolls back migrations from the database
#

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MIGRATIONS_DIR="${SCRIPT_DIR}"
DB_PATH="${DB_PATH:-./apicerberus.db}"
LOG_FILE="${LOG_FILE:-/var/log/apicerberus/migrate-down.log}"
DRY_RUN="${DRY_RUN:-false}"
FORCE="${FORCE:-false}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1" | tee -a "$LOG_FILE"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1" | tee -a "$LOG_FILE"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1" | tee -a "$LOG_FILE"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1" | tee -a "$LOG_FILE"
}

# Create log directory if needed
mkdir -p "$(dirname "$LOG_FILE")" 2>/dev/null || true

# Help message
usage() {
    cat <<EOF
APICerebrus Database Migration - DOWN

Usage: $0 [OPTIONS]

Options:
    -d, --db PATH       Database path (default: ./apicerberus.db)
    -m, --migrations    Migrations directory (default: script directory)
    -n, --dry-run       Show what would be rolled back without applying
    -s, --steps N       Number of migrations to roll back (default: 1)
    -v, --version N     Roll back to specific version
    -f, --force         Force rollback without confirmation
    -h, --help          Show this help message

Environment Variables:
    DB_PATH             Database file path
    DRY_RUN             Set to 'true' for dry run mode
    FORCE               Set to 'true' to skip confirmation

Examples:
    $0                              # Roll back 1 migration
    $0 -s 3                         # Roll back 3 migrations
    $0 -v 2                         # Roll back to version 2
    $0 -n                           # Dry run - show what would be rolled back
    $0 -f                           # Force rollback without confirmation

WARNING:
    Rolling back migrations may result in DATA LOSS.
    Always backup your database before rolling back.
EOF
}

# Parse arguments
STEPS=1
TARGET_VERSION=""
while [[ $# -gt 0 ]]; do
    case $1 in
        -d|--db)
            DB_PATH="$2"
            shift 2
            ;;
        -m|--migrations)
            MIGRATIONS_DIR="$2"
            shift 2
            ;;
        -n|--dry-run)
            DRY_RUN="true"
            shift
            ;;
        -s|--steps)
            STEPS="$2"
            shift 2
            ;;
        -v|--version)
            TARGET_VERSION="$2"
            shift 2
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

# Validate database path
if [[ ! -f "$DB_PATH" ]]; then
    log_error "Database not found at $DB_PATH"
    exit 1
fi

# Check for sqlite3
if ! command -v sqlite3 &> /dev/null; then
    log_error "sqlite3 is required but not installed"
    exit 1
fi

log_info "Starting migration DOWN"
log_info "Database: $DB_PATH"
log_info "Migrations: $MIGRATIONS_DIR"
[[ "$DRY_RUN" == "true" ]] && log_warn "DRY RUN MODE - No changes will be made"

# Get current version
get_current_version() {
    local version
    version=$(sqlite3 "$DB_PATH" "SELECT COALESCE(MAX(version), 0) FROM schema_migrations;" 2>/dev/null || echo "0")
    echo "$version"
}

# Get migrations to roll back
get_rollback_migrations() {
    local current_version="$1"
    local target="$2"

    sqlite3 "$DB_PATH" "SELECT version, name FROM schema_migrations WHERE version > $target ORDER BY version DESC;" | while IFS='|' read -r version name; do
        local file="${MIGRATIONS_DIR}/$(printf "%03d" "$version")_${name}.down.sql"
        if [[ -f "$file" ]]; then
            echo "$version:$name:$file"
        else
            log_error "Down migration not found: $file"
            exit 1
        fi
    done
}

# Rollback a single migration
rollback_migration() {
    local version="$1"
    local name="$2"
    local file="$3"

    log_info "Rolling back migration $version: $name"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "  [DRY RUN] Would execute: $file"
        return 0
    fi

    # Execute rollback in a transaction
    if sqlite3 "$DB_PATH" < "$file"; then
        # Remove migration record
        sqlite3 "$DB_PATH" "DELETE FROM schema_migrations WHERE version = $version;"
        log_success "  Rolled back migration $version"
    else
        log_error "  Failed to roll back migration $version"
        return 1
    fi
}

# Main rollback logic
main() {
    local current_version
    current_version=$(get_current_version)

    if [[ $current_version -eq 0 ]]; then
        log_warn "No migrations have been applied"
        exit 0
    fi

    log_info "Current database version: $current_version"

    # Determine target version
    local target_version
    if [[ -n "$TARGET_VERSION" ]]; then
        target_version="$TARGET_VERSION"
    else
        target_version=$((current_version - STEPS))
    fi

    if [[ $target_version -lt 0 ]]; then
        target_version=0
    fi

    log_info "Target version: $target_version"

    if [[ $target_version -ge $current_version ]]; then
        log_warn "Nothing to roll back"
        exit 0
    fi

    # Get migrations to roll back
    local to_rollback
    to_rollback=$(get_rollback_migrations "$current_version" "$target_version")

    if [[ -z "$to_rollback" ]]; then
        log_warn "No migrations to roll back"
        exit 0
    fi

    local count
    count=$(echo "$to_rollback" | wc -l)
    log_warn "About to roll back $count migration(s)"

    # Confirmation (unless forced)
    if [[ "$FORCE" != "true" && "$DRY_RUN" != "true" ]]; then
        echo
        log_warn "WARNING: This operation may result in DATA LOSS!"
        read -p "Are you sure you want to continue? (yes/no) " -r
        echo
        if [[ ! $REPLY =~ ^yes$ ]]; then
            log_info "Rollback aborted"
            exit 0
        fi
    fi

    # Perform rollback
    local rolled_back=0
    echo "$to_rollback" | while IFS=: read -r version name file; do
        if rollback_migration "$version" "$name" "$file"; then
            ((rolled_back++))
        else
            log_error "Rollback failed at version $version"
            log_error "Database may be in an inconsistent state"
            log_error "Manual intervention may be required"
            exit 1
        fi
    done

    local final_version
    final_version=$(get_current_version)

    if [[ "$DRY_RUN" == "true" ]]; then
        log_warn "Dry run complete. No changes made."
    else
        log_success "Rollback complete. Database is now at version $final_version"
    fi
}

# Run main function
main "$@"
