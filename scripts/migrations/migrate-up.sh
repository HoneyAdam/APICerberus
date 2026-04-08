#!/bin/bash
#
# APICerebrus Database Migration Script - UP
# Applies pending migrations to the database
#

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MIGRATIONS_DIR="${SCRIPT_DIR}"
DB_PATH="${DB_PATH:-./apicerberus.db}"
LOG_FILE="${LOG_FILE:-/var/log/apicerberus/migrate-up.log}"
DRY_RUN="${DRY_RUN:-false}"

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
APICerebrus Database Migration - UP

Usage: $0 [OPTIONS]

Options:
    -d, --db PATH       Database path (default: ./apicerberus.db)
    -m, --migrations    Migrations directory (default: script directory)
    -n, --dry-run       Show what would be applied without applying
    -v, --version N     Migrate to specific version (default: latest)
    -h, --help          Show this help message

Environment Variables:
    DB_PATH             Database file path
    DRY_RUN             Set to 'true' for dry run mode

Examples:
    $0                              # Apply all pending migrations
    $0 -d /data/apicerberus.db      # Use specific database
    $0 -n                           # Dry run - show what would be applied
    $0 -v 3                         # Migrate to version 3
EOF
}

# Parse arguments
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
        -v|--version)
            TARGET_VERSION="$2"
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

# Validate database path
if [[ ! -f "$DB_PATH" ]]; then
    log_warn "Database not found at $DB_PATH"
    read -p "Create new database? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        log_error "Migration aborted"
        exit 1
    fi
fi

# Check for sqlite3
if ! command -v sqlite3 &> /dev/null; then
    log_error "sqlite3 is required but not installed"
    exit 1
fi

log_info "Starting migration UP"
log_info "Database: $DB_PATH"
log_info "Migrations: $MIGRATIONS_DIR"
[[ "$DRY_RUN" == "true" ]] && log_warn "DRY RUN MODE - No changes will be made"

# Initialize schema_migrations table if not exists
init_schema_migrations() {
    sqlite3 "$DB_PATH" < "${MIGRATIONS_DIR}/schema_migrations.sql"
}

# Get current version
get_current_version() {
    local version
    version=$(sqlite3 "$DB_PATH" "SELECT COALESCE(MAX(version), 0) FROM schema_migrations;" 2>/dev/null || echo "0")
    echo "$version"
}

# Get list of pending migrations
get_pending_migrations() {
    local current_version="$1"
    find "$MIGRATIONS_DIR" -name '*.up.sql' -type f | sort | while read -r file; do
        local version
        version=$(basename "$file" | cut -d'_' -f1 | sed 's/^0*//')
        if [[ $version -gt $current_version ]]; then
            echo "$version:$file"
        fi
    done
}

# Apply a single migration
apply_migration() {
    local version="$1"
    local file="$2"
    local name
    name=$(basename "$file" | sed 's/^[0-9]*_//' | sed 's/\.up\.sql$//')

    log_info "Applying migration $version: $name"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "  [DRY RUN] Would execute: $file"
        return 0
    fi

    # Execute migration in a transaction
    if sqlite3 "$DB_PATH" < "$file"; then
        # Record migration
        sqlite3 "$DB_PATH" "INSERT INTO schema_migrations (version, name, applied_at) VALUES ($version, '$name', datetime('now'));"
        log_success "  Applied migration $version"
    else
        log_error "  Failed to apply migration $version"
        return 1
    fi
}

# Main migration logic
main() {
    # Initialize schema_migrations table
    init_schema_migrations

    local current_version
    current_version=$(get_current_version)
    log_info "Current database version: $current_version"

    # Get pending migrations
    local pending
    pending=$(get_pending_migrations "$current_version")

    if [[ -z "$pending" ]]; then
        log_success "Database is up to date (version $current_version)"
        exit 0
    fi

    local count
    count=$(echo "$pending" | wc -l)
    log_info "Found $count pending migration(s)"

    # Apply migrations
    local applied=0
    echo "$pending" | while IFS=: read -r version file; do
        # Check if we should stop at target version
        if [[ -n "$TARGET_VERSION" && $version -gt $TARGET_VERSION ]]; then
            log_info "Stopping at target version $TARGET_VERSION"
            break
        fi

        if apply_migration "$version" "$file"; then
            ((applied++))
        else
            log_error "Migration failed at version $version"
            log_error "Database may be in an inconsistent state"
            exit 1
        fi
    done

    local final_version
    final_version=$(get_current_version)

    if [[ "$DRY_RUN" == "true" ]]; then
        log_warn "Dry run complete. No changes made."
    else
        log_success "Migration complete. Database is now at version $final_version"
    fi
}

# Run main function
main "$@"
