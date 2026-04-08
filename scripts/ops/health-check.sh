#!/bin/bash
#
# APICerebrus Comprehensive Health Check Script
# Checks all system components and reports status
#

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_FILE="${CONFIG_FILE:-./apicerberus.yaml}"
DB_PATH="${DB_PATH:-./apicerberus.db}"
LOG_FILE="${LOG_FILE:-/var/log/apicerberus/health-check.log}"
TIMEOUT="${TIMEOUT:-5}"
VERBOSE="${VERBOSE:-false}"
OUTPUT_FORMAT="${OUTPUT_FORMAT:-text}"  # text, json

# Health check results
declare -A CHECK_RESULTS
declare -A CHECK_DETAILS
OVERALL_STATUS="healthy"

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
    echo -e "${GREEN}[PASS]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1" | tee -a "$LOG_FILE"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1" | tee -a "$LOG_FILE"
}

log_error() {
    echo -e "${RED}[FAIL]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1" | tee -a "$LOG_FILE"
}

# Create log directory
mkdir -p "$(dirname "$LOG_FILE")" 2>/dev/null || true

# Help message
usage() {
    cat <<EOF
APICerebrus Health Check Script

Usage: $0 [OPTIONS]

Options:
    -c, --config FILE       Config file path (default: ./apicerberus.yaml)
    -d, --db PATH           Database path (default: ./apicerberus.db)
    -t, --timeout SECONDS   Connection timeout (default: 5)
    -f, --format FORMAT     Output format: text, json (default: text)
    -v, --verbose           Verbose output
    -q, --quiet             Quiet mode (only output results)
    -h, --help              Show this help message

Environment Variables:
    CONFIG_FILE             Configuration file path
    DB_PATH                 Database file path
    TIMEOUT                 Connection timeout
    VERBOSE                 Verbose output
    OUTPUT_FORMAT           Output format

Examples:
    $0                              # Run all health checks
    $0 -v                           # Verbose output
    $0 -f json                      # JSON output
    $0 -c /etc/apicerberus.yaml     # Custom config

Exit Codes:
    0   All checks passed
    1   One or more checks failed
    2   Critical system failure
EOF
}

# Parse arguments
QUIET="false"
while [[ $# -gt 0 ]]; do
    case $1 in
        -c|--config)
            CONFIG_FILE="$2"
            shift 2
            ;;
        -d|--db)
            DB_PATH="$2"
            shift 2
            ;;
        -t|--timeout)
            TIMEOUT="$2"
            shift 2
            ;;
        -f|--format)
            OUTPUT_FORMAT="$2"
            shift 2
            ;;
        -v|--verbose)
            VERBOSE="true"
            shift
            ;;
        -q|--quiet)
            QUIET="true"
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "Unknown option: $1" >&2
            usage
            exit 1
            ;;
    esac
done

# Record check result
record_check() {
    local name="$1"
    local status="$2"
    local details="${3:-}"

    CHECK_RESULTS["$name"]="$status"
    CHECK_DETAILS["$name"]="$details"

    if [[ "$status" == "critical" ]]; then
        OVERALL_STATUS="critical"
    elif [[ "$status" == "failed" && "$OVERALL_STATUS" != "critical" ]]; then
        OVERALL_STATUS="unhealthy"
    elif [[ "$status" == "warning" && "$OVERALL_STATUS" == "healthy" ]]; then
        OVERALL_STATUS="degraded"
    fi
}

# Check 1: Process running
check_process() {
    local name="process"
    local details=""

    if pgrep -x "apicerberus" > /dev/null 2>&1; then
        local pid
        pid=$(pgrep -x "apicerberus")
        details="PID: $pid"
        record_check "$name" "passed" "$details"
        [[ "$QUIET" == "false" ]] && log_success "Process is running ($details)"
    else
        details="apicerberus process not found"
        record_check "$name" "failed" "$details"
        [[ "$QUIET" == "false" ]] && log_error "Process check failed: $details"
    fi
}

# Check 2: Gateway HTTP endpoint
check_gateway() {
    local name="gateway"
    local gateway_url="http://localhost:8080/health"

    # Try to get gateway address from config
    if [[ -f "$CONFIG_FILE" ]]; then
        local config_port
        config_port=$(grep -E "^\s*http_addr:" "$CONFIG_FILE" | head -1 | sed 's/.*:\s*"*//' | sed 's/"*$//' | sed 's/.*://')
        if [[ -n "$config_port" ]]; then
            gateway_url="http://localhost:${config_port}/health"
        fi
    fi

    if curl -sf --max-time "$TIMEOUT" "$gateway_url" > /dev/null 2>&1; then
        record_check "$name" "passed" "Gateway responding at $gateway_url"
        [[ "$QUIET" == "false" ]] && log_success "Gateway is healthy"
    else
        record_check "$name" "failed" "Gateway not responding at $gateway_url"
        [[ "$QUIET" == "false" ]] && log_error "Gateway check failed"
    fi
}

# Check 3: Admin API endpoint
check_admin() {
    local name="admin"
    local admin_url="http://localhost:9876/health"

    # Try to get admin address from config
    if [[ -f "$CONFIG_FILE" ]]; then
        local config_port
        config_port=$(grep -A5 "^admin:" "$CONFIG_FILE" | grep "addr:" | head -1 | sed 's/.*:\s*"*//' | sed 's/"*$//' | sed 's/.*://')
        if [[ -n "$config_port" ]]; then
            admin_url="http://localhost:${config_port}/health"
        fi
    fi

    if curl -sf --max-time "$TIMEOUT" "$admin_url" > /dev/null 2>&1; then
        record_check "$name" "passed" "Admin API responding at $admin_url"
        [[ "$QUIET" == "false" ]] && log_success "Admin API is healthy"
    else
        record_check "$name" "failed" "Admin API not responding at $admin_url"
        [[ "$QUIET" == "false" ]] && log_error "Admin API check failed"
    fi
}

# Check 4: Database connectivity
check_database() {
    local name="database"

    if ! command -v sqlite3 &> /dev/null; then
        record_check "$name" "failed" "sqlite3 not installed"
        [[ "$QUIET" == "false" ]] && log_error "Database check failed: sqlite3 not installed"
        return
    fi

    if [[ ! -f "$DB_PATH" ]]; then
        record_check "$name" "failed" "Database file not found: $DB_PATH"
        [[ "$QUIET" == "false" ]] && log_error "Database check failed: file not found"
        return
    fi

    # Check integrity
    local integrity
    integrity=$(sqlite3 "$DB_PATH" "PRAGMA integrity_check;" 2>/dev/null || echo "failed")

    if [[ "$integrity" == "ok" ]]; then
        # Get database stats
        local tables
        tables=$(sqlite3 "$DB_PATH" "SELECT count(*) FROM sqlite_master WHERE type='table';" 2>/dev/null || echo "0")
        local size
        size=$(du -h "$DB_PATH" 2>/dev/null | cut -f1 || echo "unknown")

        record_check "$name" "passed" "Integrity OK, $tables tables, size: $size"
        [[ "$QUIET" == "false" ]] && log_success "Database is healthy ($tables tables, $size)"
    else
        record_check "$name" "critical" "Integrity check failed: $integrity"
        [[ "$QUIET" == "false" ]] && log_error "Database integrity check failed"
    fi
}

# Check 5: Disk space
check_disk() {
    local name="disk"
    local threshold=90

    local usage
    usage=$(df . | tail -1 | awk '{print $5}' | sed 's/%//')

    if [[ $usage -lt $threshold ]]; then
        record_check "$name" "passed" "Disk usage: ${usage}%"
        [[ "$QUIET" == "false" ]] && log_success "Disk space OK (${usage}%)"
    elif [[ $usage -lt 95 ]]; then
        record_check "$name" "warning" "Disk usage high: ${usage}%"
        [[ "$QUIET" == "false" ]] && log_warn "Disk space warning (${usage}%)"
    else
        record_check "$name" "critical" "Disk usage critical: ${usage}%"
        [[ "$QUIET" == "false" ]] && log_error "Disk space critical (${usage}%)"
    fi
}

# Check 6: Memory usage
check_memory() {
    local name="memory"

    if [[ -f /proc/meminfo ]]; then
        local total
        total=$(grep MemTotal /proc/meminfo | awk '{print $2}')
        local available
        available=$(grep MemAvailable /proc/meminfo | awk '{print $2}')
        local usage=$((100 - (available * 100 / total)))

        if [[ $usage -lt 80 ]]; then
            record_check "$name" "passed" "Memory usage: ${usage}%"
            [[ "$QUIET" == "false" ]] && log_success "Memory OK (${usage}%)"
        elif [[ $usage -lt 90 ]]; then
            record_check "$name" "warning" "Memory usage high: ${usage}%"
            [[ "$QUIET" == "false" ]] && log_warn "Memory warning (${usage}%)"
        else
            record_check "$name" "critical" "Memory usage critical: ${usage}%"
            [[ "$QUIET" == "false" ]] && log_error "Memory critical (${usage}%)"
        fi
    else
        record_check "$name" "warning" "Cannot check memory on this system"
        [[ "$QUIET" == "false" ]] && log_warn "Memory check not available"
    fi
}

# Check 7: Log files
check_logs() {
    local name="logs"
    local log_dir="/var/log/apicerberus"

    if [[ ! -d "$log_dir" ]]; then
        record_check "$name" "warning" "Log directory not found: $log_dir"
        [[ "$QUIET" == "false" ]] && log_warn "Log directory not found"
        return
    fi

    # Check for recent errors
    local errors
    errors=$(find "$log_dir" -name "*.log" -type f -mtime -1 -exec grep -l "ERROR\|CRITICAL" {} \; 2>/dev/null | wc -l)

    if [[ $errors -eq 0 ]]; then
        record_check "$name" "passed" "No recent errors in logs"
        [[ "$QUIET" == "false" ]] && log_success "No recent errors in logs"
    else
        record_check "$name" "warning" "Found errors in $errors log files"
        [[ "$QUIET" == "false" ]] && log_warn "Recent errors found in logs"
    fi
}

# Check 8: Backup status
check_backups() {
    local name="backups"
    local backup_dir="./backups"

    if [[ ! -d "$backup_dir" ]]; then
        record_check "$name" "warning" "Backup directory not found"
        [[ "$QUIET" == "false" ]] && log_warn "Backup directory not found"
        return
    fi

    # Check for recent backup (within 25 hours)
    local recent_backup
    recent_backup=$(find "$backup_dir" -name "apicerberus_*.db*" -type f -mtime -1 2>/dev/null | head -1)

    if [[ -n "$recent_backup" ]]; then
        local backup_time
        backup_time=$(stat -c %y "$recent_backup" 2>/dev/null | cut -d'.' -f1 || echo "unknown")
        record_check "$name" "passed" "Recent backup: $backup_time"
        [[ "$QUIET" == "false" ]] && log_success "Recent backup found"
    else
        record_check "$name" "warning" "No recent backup found (last 24h)"
        [[ "$QUIET" == "false" ]] && log_warn "No recent backup found"
    fi
}

# Check 9: Network connectivity
check_network() {
    local name="network"

    # Check if we can resolve DNS
    if nslookup google.com > /dev/null 2>&1; then
        record_check "$name" "passed" "DNS resolution working"
        [[ "$QUIET" == "false" ]] && log_success "Network connectivity OK"
    else
        record_check "$name" "warning" "DNS resolution failed"
        [[ "$QUIET" == "false" ]] && log_warn "Network check failed"
    fi
}

# Check 10: Certificate expiration
check_certificates() {
    local name="certificates"
    local ssl_dir="./config/ssl"

    if [[ ! -d "$ssl_dir" ]]; then
        record_check "$name" "passed" "No SSL directory configured"
        return
    fi

    local expired=0
    local expiring_soon=0

    while IFS= read -r -d '' cert; do
        if [[ "$cert" == *.pem || "$cert" == *.crt ]]; then
            local expiry
            expiry=$(openssl x509 -in "$cert" -noout -enddate 2>/dev/null | cut -d= -f2)
            if [[ -n "$expiry" ]]; then
                local expiry_epoch
                expiry_epoch=$(date -d "$expiry" +%s 2>/dev/null || echo "0")
                local now
                now=$(date +%s)
                local days_until=$(( (expiry_epoch - now) / 86400 ))

                if [[ $days_until -lt 0 ]]; then
                    ((expired++))
                elif [[ $days_until -lt 30 ]]; then
                    ((expiring_soon++))
                fi
            fi
        fi
    done < <(find "$ssl_dir" -type f -print0 2>/dev/null)

    if [[ $expired -gt 0 ]]; then
        record_check "$name" "critical" "$expired certificate(s) expired"
        [[ "$QUIET" == "false" ]] && log_error "Expired certificates found"
    elif [[ $expiring_soon -gt 0 ]]; then
        record_check "$name" "warning" "$expiring_soon certificate(s) expiring soon"
        [[ "$QUIET" == "false" ]] && log_warn "Certificates expiring soon"
    else
        record_check "$name" "passed" "All certificates valid"
        [[ "$QUIET" == "false" ]] && log_success "Certificates OK"
    fi
}

# Output results in text format
output_text() {
    echo
    echo "========================================"
    echo "APICerebrus Health Check Results"
    echo "========================================"
    echo "Timestamp: $(date -Iseconds)"
    echo "Overall Status: $OVERALL_STATUS"
    echo "----------------------------------------"
    echo

    for check in "${!CHECK_RESULTS[@]}"; do
        local status="${CHECK_RESULTS[$check]}"
        local details="${CHECK_DETAILS[$check]}"
        local symbol="✓"
        local color="$GREEN"

        case "$status" in
            passed)
                symbol="✓"
                color="$GREEN"
                ;;
            warning)
                symbol="⚠"
                color="$YELLOW"
                ;;
            failed)
                symbol="✗"
                color="$RED"
                ;;
            critical)
                symbol="✗"
                color="$RED"
                ;;
        esac

        printf "${color}%s${NC} %-15s [%s]\n" "$symbol" "$check" "$status"
        if [[ "$VERBOSE" == "true" && -n "$details" ]]; then
            printf "    %s\n" "$details"
        fi
    done

    echo
    echo "========================================"
}

# Output results in JSON format
output_json() {
    local json="{"
    json+="\"timestamp\":\"$(date -Iseconds)\","
    json+="\"status\":\"$OVERALL_STATUS\","
    json+="\"checks\":{"

    local first=true
    for check in "${!CHECK_RESULTS[@]}"; do
        [[ "$first" == "true" ]] || json+=","
        first=false
        local status="${CHECK_RESULTS[$check]}"
        local details="${CHECK_DETAILS[$check]}"
        json+="\"$check\":{\"status\":\"$status\",\"details\":\"$details\"}"
    done

    json+="}}"
    echo "$json"
}

# Main
main() {
    [[ "$QUIET" == "false" ]] && log_info "Starting health check..."

    # Run all checks
    check_process
    check_gateway
    check_admin
    check_database
    check_disk
    check_memory
    check_logs
    check_backups
    check_network
    check_certificates

    # Output results
    case "$OUTPUT_FORMAT" in
        json)
            output_json
            ;;
        *)
            output_text
            ;;
    esac

    # Exit with appropriate code
    case "$OVERALL_STATUS" in
        healthy)
            exit 0
            ;;
        degraded)
            exit 0
            ;;
        unhealthy)
            exit 1
            ;;
        critical)
            exit 2
            ;;
        *)
            exit 1
            ;;
    esac
}

main "$@"
