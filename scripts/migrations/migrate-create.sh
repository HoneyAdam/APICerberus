#!/bin/bash
#
# APICerebrus Migration Creator
# Creates a new migration file with proper naming
#

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MIGRATIONS_DIR="${SCRIPT_DIR}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Help message
usage() {
    cat <<EOF
APICerebrus Migration Creator

Usage: $0 [OPTIONS] <name>

Arguments:
    name                Migration name (e.g., "add_user_preferences")

Options:
    -m, --migrations    Migrations directory (default: script directory)
    -h, --help          Show this help message

Examples:
    $0 add_user_preferences              # Create migration for user preferences
    $0 create_audit_table                # Create migration for audit table

Migration names should:
    - Use snake_case
    - Be descriptive of the change
    - Not include version numbers
EOF
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -m|--migrations)
            MIGRATIONS_DIR="$2"
            shift 2
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        -*)
            echo -e "${RED}[ERROR]${NC} Unknown option: $1"
            usage
            exit 1
            ;;
        *)
            break
            ;;
    esac
done

# Get migration name
MIGRATION_NAME="${1:-}"
if [[ -z "$MIGRATION_NAME" ]]; then
    echo -e "${RED}[ERROR]${NC} Migration name is required"
    usage
    exit 1
fi

# Validate migration name
if [[ ! "$MIGRATION_NAME" =~ ^[a-z][a-z0-9_]*$ ]]; then
    echo -e "${RED}[ERROR]${NC} Migration name must be snake_case and start with a letter"
    exit 1
fi

# Get next version number
get_next_version() {
    local max_version=0
    for file in "$MIGRATIONS_DIR"/*.up.sql; do
        if [[ -f "$file" ]]; then
            local version
            version=$(basename "$file" | cut -d'_' -f1 | sed 's/^0*//')
            if [[ $version -gt $max_version ]]; then
                max_version=$version
            fi
        fi
    done
    echo $((max_version + 1))
}

# Create migration files
create_migration() {
    local version="$1"
    local name="$2"
    local version_padded
    version_padded=$(printf "%03d" "$version")

    local up_file="${MIGRATIONS_DIR}/${version_padded}_${name}.up.sql"
    local down_file="${MIGRATIONS_DIR}/${version_padded}_${name}.down.sql"

    # Check if files already exist
    if [[ -f "$up_file" || -f "$down_file" ]]; then
        echo -e "${RED}[ERROR]${NC} Migration files already exist for version $version"
        exit 1
    fi

    local date_str
    date_str=$(date +%Y-%m-%d)

    # Create up migration
    cat > "$up_file" <<EOF
-- Migration: ${version_padded}_${name}
-- Description: ${name//_/ }
-- Created: ${date_str}

-- Add your migration SQL here

EOF

    # Create down migration
    cat > "$down_file" <<EOF
-- Migration: ${version_padded}_${name} (rollback)
-- Description: Rollback ${name//_/ }

-- Add your rollback SQL here

EOF

    echo -e "${GREEN}[SUCCESS]${NC} Created migration files:"
    echo "  - $up_file"
    echo "  - $down_file"
}

# Main
main() {
    echo -e "${BLUE}APICerebrus Migration Creator${NC}"
    echo

    local next_version
    next_version=$(get_next_version)

    echo -e "${BLUE}Next version:${NC} $next_version"
    echo -e "${BLUE}Name:${NC} $MIGRATION_NAME"
    echo

    create_migration "$next_version" "$MIGRATION_NAME"

    echo
    echo -e "${YELLOW}Next steps:${NC}"
    echo "  1. Edit the .up.sql file to add your migration"
    echo "  2. Edit the .down.sql file to add your rollback"
    echo "  3. Test with: ./migrate-up.sh -n"
    echo "  4. Apply with: ./migrate-up.sh"
}

main "$@"
