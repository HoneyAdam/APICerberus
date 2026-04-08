#!/bin/bash
#
# APICerebrus Backup Scheduler Setup
# Creates cron jobs for automated backups
#

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CRON_FILE="/etc/cron.d/apicerberus-backup"
BACKUP_USER="${BACKUP_USER:-$(whoami)}"
DB_PATH="${DB_PATH:-/var/lib/apicerberus/apicerberus.db}"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/apicerberus}"
CONFIG_DIR="${CONFIG_DIR:-/etc/apicerberus}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Help message
usage() {
    cat <<EOF
APICerebrus Backup Scheduler Setup

Usage: $0 [COMMAND] [OPTIONS]

Commands:
    install     Install backup cron jobs
    remove      Remove backup cron jobs
    list        List current backup cron jobs
    test        Test backup scripts

Options:
    -u, --user USER         User to run backups as (default: current user)
    -d, --db PATH           Database path
    -b, --backup-dir DIR    Backup directory
    -c, --config-dir DIR    Config directory
    -h, --help              Show this help message

Examples:
    $0 install                          # Install with defaults
    $0 install -u apicerberus           # Run as specific user
    $0 remove                           # Remove all backup cron jobs
    $0 list                             # Show current cron jobs
    $0 test                             # Test backup scripts

Backup Schedule:
    - Daily database backup: 2:00 AM
    - Weekly full backup: Sundays at 3:00 AM
    - Monthly config backup: 1st of month at 4:00 AM

Requirements:
    - Root access to install system cron jobs
    - Backup scripts must be executable
    - Sufficient disk space for backups
EOF
}

# Parse arguments
COMMAND="${1:-}"
shift || true

while [[ $# -gt 0 ]]; do
    case $1 in
        -u|--user)
            BACKUP_USER="$2"
            shift 2
            ;;
        -d|--db)
            DB_PATH="$2"
            shift 2
            ;;
        -b|--backup-dir)
            BACKUP_DIR="$2"
            shift 2
            ;;
        -c|--config-dir)
            CONFIG_DIR="$2"
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

# Check if running as root for install/remove
check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This command must be run as root"
        exit 1
    fi
}

# Install cron jobs
install_cron() {
    check_root

    log_info "Installing backup cron jobs..."

    # Create backup directories
    mkdir -p "$BACKUP_DIR"
    mkdir -p "/var/log/apicerberus"
    chown -R "$BACKUP_USER:$BACKUP_USER" "$BACKUP_DIR" 2>/dev/null || true

    # Create cron file
    cat > "$CRON_FILE" <<EOF
# APICerebrus Automated Backup Schedule
# Generated: $(date -Iseconds)
# Do not edit manually - use backup-scheduler.sh

SHELL=/bin/bash
PATH=/usr/local/bin:/usr/bin:/bin

# Daily database backup at 2:00 AM
0 2 * * * $BACKUP_USER $SCRIPT_DIR/backup-sqlite.sh -d "$DB_PATH" -o "$BACKUP_DIR/database" >> /var/log/apicerberus/backup-cron.log 2>&1

# Weekly full backup on Sundays at 3:00 AM
0 3 * * 0 $BACKUP_USER $SCRIPT_DIR/backup-full.sh -d "$DB_PATH" -c "$CONFIG_DIR" -o "$BACKUP_DIR/full" >> /var/log/apicerberus/backup-cron.log 2>&1

# Monthly config backup on 1st at 4:00 AM
0 4 1 * * $BACKUP_USER $SCRIPT_DIR/backup-config.sh -c "$CONFIG_DIR" -o "$BACKUP_DIR/config" >> /var/log/apicerberus/backup-cron.log 2>&1

# Cleanup old backups weekly (Sundays at 5:00 AM)
0 5 * * 0 $BACKUP_USER find "$BACKUP_DIR" -type f -mtime +30 -delete >> /var/log/apicerberus/backup-cron.log 2>&1
EOF

    chmod 644 "$CRON_FILE"

    log_success "Cron jobs installed to: $CRON_FILE"
    log_info "Backup schedule:"
    log_info "  - Daily database: 2:00 AM"
    log_info "  - Weekly full: Sundays 3:00 AM"
    log_info "  - Monthly config: 1st of month 4:00 AM"
    log_info "  - Cleanup: Sundays 5:00 AM"

    # Reload cron
    if command -v crond &> /dev/null; then
        systemctl restart crond 2>/dev/null || service cron restart 2>/dev/null || true
    fi

    log_success "Cron service reloaded"
}

# Remove cron jobs
remove_cron() {
    check_root

    log_info "Removing backup cron jobs..."

    if [[ -f "$CRON_FILE" ]]; then
        rm -f "$CRON_FILE"
        log_success "Cron jobs removed"

        # Reload cron
        if command -v crond &> /dev/null; then
            systemctl restart crond 2>/dev/null || service cron restart 2>/dev/null || true
        fi
    else
        log_warn "No cron jobs found to remove"
    fi
}

# List cron jobs
list_cron() {
    log_info "Current backup cron jobs:"
    echo

    if [[ -f "$CRON_FILE" ]]; then
        cat "$CRON_FILE"
    else
        log_warn "No APICerebrus cron jobs found"
    fi

    echo
    log_info "System cron entries:"
    crontab -l 2>/dev/null | grep -i apicerberus || log_warn "No user cron jobs found"
}

# Test backup scripts
test_backups() {
    log_info "Testing backup scripts..."
    echo

    # Test database backup
    log_info "Testing database backup..."
    if [[ -f "$SCRIPT_DIR/backup-sqlite.sh" ]]; then
        if bash -n "$SCRIPT_DIR/backup-sqlite.sh"; then
            log_success "  backup-sqlite.sh syntax OK"
        else
            log_error "  backup-sqlite.sh has syntax errors"
        fi
    else
        log_warn "  backup-sqlite.sh not found"
    fi

    # Test config backup
    log_info "Testing config backup..."
    if [[ -f "$SCRIPT_DIR/backup-config.sh" ]]; then
        if bash -n "$SCRIPT_DIR/backup-config.sh"; then
            log_success "  backup-config.sh syntax OK"
        else
            log_error "  backup-config.sh has syntax errors"
        fi
    else
        log_warn "  backup-config.sh not found"
    fi

    # Test full backup
    log_info "Testing full backup..."
    if [[ -f "$SCRIPT_DIR/backup-full.sh" ]]; then
        if bash -n "$SCRIPT_DIR/backup-full.sh"; then
            log_success "  backup-full.sh syntax OK"
        else
            log_error "  backup-full.sh has syntax errors"
        fi
    else
        log_warn "  backup-full.sh not found"
    fi

    echo
    log_info "To perform a test backup, run:"
    log_info "  $SCRIPT_DIR/backup-sqlite.sh -d $DB_PATH -o /tmp/test-backup -n"
}

# Main
main() {
    case "${COMMAND:-}" in
        install)
            install_cron
            ;;
        remove)
            remove_cron
            ;;
        list)
            list_cron
            ;;
        test)
            test_backups
            ;;
        *)
            usage
            exit 1
            ;;
    esac
}

main "$@"
