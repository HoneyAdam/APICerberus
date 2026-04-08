#!/bin/bash
#
# APICerebrus Secret Rotation Helper
# Assists with rotating API keys, session secrets, and certificates
#

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DB_PATH="${DB_PATH:-./apicerberus.db}"
CONFIG_FILE="${CONFIG_FILE:-./apicerberus.yaml}"
BACKUP_DIR="${BACKUP_DIR:-./backups/secrets}"
DRY_RUN="${DRY_RUN:-false}"
FORCE="${FORCE:-false}"
LOG_FILE="${LOG_FILE:-/var/log/apicerberus/rotate-secrets.log}"

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
APICerebrus Secret Rotation Helper

Usage: $0 [COMMAND] [OPTIONS]

Commands:
    api-key USER_ID         Rotate API key for a user
    session-secret          Generate new session secret
    admin-key               Generate new admin API key
    ssl-cert DOMAIN         Generate new SSL certificate
    all                     Rotate all secrets (use with caution!)
    list                    List secrets requiring rotation

Options:
    -d, --db PATH           Database path (default: ./apicerberus.db)
    -c, --config FILE       Config file path (default: ./apicerberus.yaml)
    -b, --backup DIR        Backup directory (default: ./backups/secrets)
    -n, --dry-run           Show what would be done without doing it
    -f, --force             Skip confirmation prompts
    -h, --help              Show this help message

Environment Variables:
    DB_PATH                 Database file path
    CONFIG_FILE             Configuration file path
    BACKUP_DIR              Backup directory
    DRY_RUN                 Dry run mode
    FORCE                   Skip confirmations
    LOG_FILE                Log file path

Examples:
    $0 list                             # List secrets needing rotation
    $0 api-key user_123                 # Rotate API key for user
    $0 session-secret                   # Generate new session secret
    $0 ssl-cert api.example.com         # Generate SSL certificate

WARNING:
    Rotating secrets will invalidate existing sessions and API keys!
    Always notify users before rotating production secrets.
EOF
}

# Parse arguments
COMMAND="${1:-}"
ARGUMENT="${2:-}"
shift 2 || true

while [[ $# -gt 0 ]]; do
    case $1 in
        -d|--db)
            DB_PATH="$2"
            shift 2
            ;;
        -c|--config)
            CONFIG_FILE="$2"
            shift 2
            ;;
        -b|--backup)
            BACKUP_DIR="$2"
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

# Generate a secure random key
generate_key() {
    local length="${1:-32}"
    openssl rand -base64 "$length" | tr -d '=+/' | cut -c1-${length}
}

# Generate API key
generate_api_key() {
    local prefix="${1:-ck_live}"
    local key
    key=$(openssl rand -hex 32)
    echo "${prefix}_${key}"
}

# Backup current secrets
backup_secrets() {
    log_info "Backing up current configuration..."

    mkdir -p "$BACKUP_DIR"

    local backup_file="${BACKUP_DIR}/secrets-$(date +%Y%m%d_%H%M%S).yaml"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "  [DRY RUN] Would backup to: $backup_file"
        return 0
    fi

    if [[ -f "$CONFIG_FILE" ]]; then
        cp "$CONFIG_FILE" "$backup_file"
        log_success "  Configuration backed up to: $backup_file"
    else
        log_warn "  Config file not found: $CONFIG_FILE"
    fi
}

# List secrets needing rotation
list_secrets() {
    log_info "Checking secrets requiring rotation..."
    echo

    if ! command -v sqlite3 &> /dev/null; then
        log_error "sqlite3 not installed"
        return 1
    fi

    if [[ ! -f "$DB_PATH" ]]; then
        log_error "Database not found: $DB_PATH"
        return 1
    fi

    # Check for old API keys (older than 90 days)
    log_info "API Keys older than 90 days:"
    local old_keys
    old_keys=$(sqlite3 "$DB_PATH" "SELECT id, user_id, created_at FROM api_keys WHERE created_at < datetime('now', '-90 days') LIMIT 10;" 2>/dev/null || echo "")
    if [[ -n "$old_keys" ]]; then
        echo "$old_keys" | while IFS='|' read -r id user_id created_at; do
            log_warn "  Key $id (user: $user_id) created: $created_at"
        done
    else
        log_success "  No old API keys found"
    fi
    echo

    # Check for old sessions
    log_info "Expired sessions to clean:"
    local expired_sessions
    expired_sessions=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM sessions WHERE expires_at < datetime('now');" 2>/dev/null || echo "0")
    if [[ "$expired_sessions" -gt 0 ]]; then
        log_warn "  $expired_sessions expired sessions"
    else
        log_success "  No expired sessions"
    fi
    echo

    # Check SSL certificate expiration
    if [[ -d "./config/ssl" ]]; then
        log_info "SSL Certificates:"
        for cert in ./config/ssl/*.pem ./config/ssl/*.crt 2>/dev/null; do
            if [[ -f "$cert" ]]; then
                local expiry
                expiry=$(openssl x509 -in "$cert" -noout -enddate 2>/dev/null | cut -d= -f2)
                if [[ -n "$expiry" ]]; then
                    local expiry_epoch
                    expiry_epoch=$(date -d "$expiry" +%s 2>/dev/null || echo "0")
                    local now
                    now=$(date +%s)
                    local days_until=$(( (expiry_epoch - now) / 86400 ))

                    if [[ $days_until -lt 0 ]]; then
                        log_error "  $(basename "$cert"): EXPIRED"
                    elif [[ $days_until -lt 30 ]]; then
                        log_warn "  $(basename "$cert"): expires in $days_until days"
                    else
                        log_success "  $(basename "$cert"): valid for $days_until days"
                    fi
                fi
            fi
        done
    fi
}

# Rotate API key for a user
rotate_api_key() {
    local user_id="$1"

    log_info "Rotating API key for user: $user_id"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "  [DRY RUN] Would rotate API key for user: $user_id"
        return 0
    fi

    # Check if user exists
    local user_exists
    user_exists=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM users WHERE id = '$user_id';" 2>/dev/null || echo "0")

    if [[ "$user_exists" -eq 0 ]]; then
        log_error "  User not found: $user_id"
        return 1
    fi

    # Generate new key
    local new_key
    new_key=$(generate_api_key)
    local key_hash
    key_hash=$(echo -n "$new_key" | sha256sum | cut -d' ' -f1)
    local key_prefix
    key_prefix=$(echo "$new_key" | cut -c1-16)

    # Update database
    local timestamp
    timestamp=$(date -Iseconds)

    sqlite3 "$DB_PATH" <<EOF
UPDATE api_keys SET status = 'revoked', updated_at = '$timestamp' WHERE user_id = '$user_id' AND status = 'active';
INSERT INTO api_keys (id, user_id, key_hash, key_prefix, name, status, created_at, updated_at)
VALUES ('$(openssl rand -hex 16)', '$user_id', '$key_hash', '$key_prefix', 'Rotated Key', 'active', '$timestamp', '$timestamp');
EOF

    log_success "  API key rotated for user: $user_id"
    log_warn "  New API Key: $new_key"
    log_warn "  IMPORTANT: Share this key securely with the user!"
}

# Generate new session secret
rotate_session_secret() {
    log_info "Generating new session secret..."

    local new_secret
    new_secret=$(generate_key 64)

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "  [DRY RUN] Would generate new session secret"
        return 0
    fi

    log_success "  New session secret generated"
    log_warn "  Session Secret: $new_secret"
    echo
    log_info "  To apply this secret:"
    log_info "  1. Update your configuration file:"
    log_info "     portal.session.secret: \"$new_secret\""
    log_info "  2. Restart APICerebrus"
    log_info "  3. All existing sessions will be invalidated"
}

# Generate new admin API key
rotate_admin_key() {
    log_info "Generating new admin API key..."

    local new_key
    new_key=$(generate_key 48)

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "  [DRY RUN] Would generate new admin API key"
        return 0
    fi

    log_success "  New admin API key generated"
    log_warn "  Admin API Key: $new_key"
    echo
    log_info "  To apply this key:"
    log_info "  1. Update your configuration file:"
    log_info "     admin.api_key: \"$new_key\""
    log_info "  2. Restart APICerebrus"
    log_info "  3. Update any scripts using the old admin key"
}

# Generate SSL certificate
rotate_ssl_cert() {
    local domain="${1:-localhost}"

    log_info "Generating new SSL certificate for: $domain"

    if ! command -v openssl &> /dev/null; then
        log_error "openssl not installed"
        return 1
    fi

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "  [DRY RUN] Would generate SSL certificate for: $domain"
        return 0
    fi

    mkdir -p "./config/ssl"

    local key_file="./config/ssl/${domain}.key"
    local cert_file="./config/ssl/${domain}.crt"
    local csr_file="./config/ssl/${domain}.csr"

    # Generate private key
    openssl genrsa -out "$key_file" 2048 2>/dev/null

    # Generate CSR
    openssl req -new -key "$key_file" -out "$csr_file" \
        -subj "/C=US/ST=State/L=City/O=Organization/CN=$domain" 2>/dev/null

    # Generate self-signed certificate
    openssl x509 -req -days 365 -in "$csr_file" -signkey "$key_file" -out "$cert_file" 2>/dev/null

    # Cleanup CSR
    rm -f "$csr_file"

    # Set permissions
    chmod 600 "$key_file"
    chmod 644 "$cert_file"

    log_success "  SSL certificate generated:"
    log_info "    Private key: $key_file"
    log_info "    Certificate: $cert_file"
    echo
    log_info "  To apply this certificate:"
    log_info "  1. Update your configuration file:"
    log_info "     gateway.tls.cert_file: \"$cert_file\""
    log_info "     gateway.tls.key_file: \"$key_file\""
    log_info "  2. Restart APICerebrus"
}

# Rotate all secrets
rotate_all() {
    log_warn "=========================================="
    log_warn "WARNING: This will rotate ALL secrets!"
    log_warn "All existing sessions and API keys will be invalidated!"
    log_warn "=========================================="
    echo

    if [[ "$FORCE" != "true" && "$DRY_RUN" != "true" ]]; then
        read -p "Are you absolutely sure? Type 'rotate all secrets' to continue: " -r
        echo
        if [[ ! $REPLY =~ ^rotate\ all\ secrets$ ]]; then
            log_info "Operation cancelled"
            exit 0
        fi
    fi

    backup_secrets

    log_info "Rotating all secrets..."
    rotate_session_secret
    rotate_admin_key

    log_success "All secrets rotated!"
    log_warn "Remember to update your configuration and restart APICerebrus!"
}

# Main
main() {
    log_info "=========================================="
    log_info "APICerebrus Secret Rotation Helper"
    log_info "=========================================="

    case "$COMMAND" in
        list)
            list_secrets
            ;;
        api-key)
            if [[ -z "$ARGUMENT" ]]; then
                log_error "User ID required for api-key command"
                usage
                exit 1
            fi
            backup_secrets
            rotate_api_key "$ARGUMENT"
            ;;
        session-secret)
            backup_secrets
            rotate_session_secret
            ;;
        admin-key)
            backup_secrets
            rotate_admin_key
            ;;
        ssl-cert)
            rotate_ssl_cert "$ARGUMENT"
            ;;
        all)
            rotate_all
            ;;
        *)
            usage
            exit 1
            ;;
    esac

    log_info "=========================================="
}

main "$@"
