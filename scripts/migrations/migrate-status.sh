#!/bin/bash
#
# APICerebrus Database Migration Status
# Shows current migration status
#

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MIGRATIONS_DIR="${SCRIPT_DIR}"
DB_PATH="${DB_PATH:-./apicerberus.db}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
GRAY='\033[0;37m'
NC='\033[0m' # No Color

# Help message
usage() {
    cat <<EOF
APICerebrus Database Migration Status

Usage: $0 [OPTIONS]

Options:
    -d, --db PATH       Database path (default: ./apicerberus.db)
    -m, --migrations    Migrations directory (default: script directory)
    -h, --help          Show this help message

Examples:
    $0                              # Show migration status
    $0 -d /data/apicerberus.db      # Check specific database
EOF
}

# Parse arguments
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
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo -e "${RED}[ERROR]${NC} Unknown option: $1"
            usage
            exit 1
            ;;
    esac
done

# Check for sqlite3
if ! command -v sqlite3 &> /dev/null; then
    echo -e "${RED}[ERROR]${NC} sqlite3 is required but not installed"
    exit 1
fi

# Get current version
get_current_version() {
    if [[ ! -f "$DB_PATH" ]]; then
        echo "0"
        return
    fi
    local version
    version=$(sqlite3 "$DB_PATH" "SELECT COALESCE(MAX(version), 0) FROM schema_migrations;" 2>/dev/null || echo "0")
    echo "$version"
}

# Get all available migrations
get_available_migrations() {
    find "$MIGRATIONS_DIR" -name '*.up.sql' -type f | sort | while read -r file; do
        local version name
        version=$(basename "$file" | cut -d'_' -f1)
        name=$(basename "$file" | sed 's/^[0-9]*_//' | sed 's/\.up\.sql$//')
        echo "$version:$name"
    done
}

# Get applied migrations
get_applied_migrations() {
    if [[ ! -f "$DB_PATH" ]]; then
        return
    fi
    sqlite3 "$DB_PATH" "SELECT version, name, applied_at FROM schema_migrations ORDER BY version;" 2>/dev/null || true
}

# Main status display
main() {
    echo -e "${BLUE}APICerebrus Database Migration Status${NC}"
    echo "========================================"
    echo

    echo -e "${BLUE}Database:${NC} $DB_PATH"

    if [[ ! -f "$DB_PATH" ]]; then
        echo -e "${YELLOW}Status:${NC} Database does not exist"
        exit 0
    fi

    local current_version
    current_version=$(get_current_version)
    echo -e "${BLUE}Current Version:${NC} $current_version"
    echo

    # Get all migrations
    local available
    available=$(get_available_migrations)

    if [[ -z "$available" ]]; then
        echo -e "${YELLOW}No migrations found in $MIGRATIONS_DIR${NC}"
        exit 0
    fi

    # Display migration list
    echo -e "${BLUE}Migrations:${NC}"
    printf "%-8s %-30s %-20s %s\n" "Version" "Name" "Applied At" "Status"
    echo "--------------------------------------------------------------------------------"

    echo "$available" | while IFS=: read -r version name; do
        local version_num
        version_num=$(echo "$version" | sed 's/^0*//')

        if [[ $version_num -le $current_version ]]; then
            local applied_at
            applied_at=$(sqlite3 "$DB_PATH" "SELECT applied_at FROM schema_migrations WHERE version = $version_num;" 2>/dev/null || echo "")
            if [[ -n "$applied_at" ]]; then
                printf "%-8s %-30s %-20s ${GREEN}%s${NC}\n" "$version" "$name" "$applied_at" "APPLIED"
            else
                printf "%-8s %-30s %-20s ${YELLOW}%s${NC}\n" "$version" "$name" "" "PARTIAL"
            fi
        else
            printf "%-8s %-30s %-20s ${GRAY}%s${NC}\n" "$version" "$name" "" "PENDING"
        fi
    done

    echo

    # Summary
    local total available_count pending
    total=$(echo "$available" | wc -l)
    pending=$((total - current_version))

    if [[ $pending -eq 0 ]]; then
        echo -e "${GREEN}Database is up to date!${NC}"
    else
        echo -e "${YELLOW}$pending migration(s) pending${NC}"
        echo "Run 'migrate-up.sh' to apply pending migrations"
    fi
}

main "$@"
