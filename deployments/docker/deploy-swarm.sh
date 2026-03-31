#!/bin/bash
# APICerebrus Docker Swarm Deployment Script
# Usage: ./deploy-swarm.sh [init|deploy|scale|destroy]

set -e

STACK_NAME="apicerberus"
COMPOSE_FILE="docker-compose.swarm.yml"
NFS_SERVER="${NFS_SERVER:-}"
ACME_EMAIL="${ACME_EMAIL:-admin@example.com}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Initialize Docker Swarm
init_swarm() {
    log_info "Initializing Docker Swarm..."

    if docker info --format '{{.Swarm.LocalNodeState}}' | grep -q "active"; then
        log_warn "Swarm is already initialized"
    else
        docker swarm init --advertise-addr $(hostname -i | awk '{print $1}')
        log_info "Swarm initialized successfully"
    fi

    # Create overlay networks if they don't exist
    docker network create --driver overlay --attachable gateway-public 2>/dev/null || true
    docker network create --driver overlay --opt encrypted --internal raft-cluster 2>/dev/null || true
    docker network create --driver overlay --opt encrypted backend 2>/dev/null || true

    log_info "Networks created"
}

# Deploy the stack
deploy_stack() {
    log_info "Deploying APICerebrus stack..."

    # Check if NFS server is set
    if [ -z "$NFS_SERVER" ]; then
        log_warn "NFS_SERVER not set. Using local volumes (not recommended for production)"
    fi

    # Generate secrets if not exist
    if ! docker secret ls | grep -q "jwt_secret"; then
        log_info "Creating JWT secret..."
        openssl rand -base64 32 | docker secret create jwt_secret -
    fi

    if ! docker secret ls | grep -q "admin_password"; then
        log_info "Creating admin password..."
        openssl rand -base64 16 | docker secret create admin_password -
    fi

    if ! docker secret ls | grep -q "db_password"; then
        log_info "Creating database password..."
        openssl rand -base64 16 | docker secret create db_password -
    fi

    # Deploy
    export NFS_SERVER
    export ACME_EMAIL
    export JWT_SECRET=$(openssl rand -base64 32)
    export ADMIN_PASSWORD=$(openssl rand -base64 16)
    export DB_PASSWORD=$(openssl rand -base64 16)

    docker stack deploy -c $COMPOSE_FILE $STACK_NAME

    log_info "Stack deployed. Waiting for services..."
    sleep 10

    # Check service status
    docker stack ps $STACK_NAME --format "table {{.Name}}\t{{.CurrentState}}\t{{.Error}}"
    docker service ls | grep $STACK_NAME
}

# Scale services
scale_service() {
    local service=$1
    local replicas=$2

    if [ -z "$service" ] || [ -z "$replicas" ]; then
        log_error "Usage: $0 scale <service> <replicas>"
        log_info "Example: $0 scale gateway 5"
        exit 1
    fi

    log_info "Scaling ${STACK_NAME}_${service} to ${replicas} replicas..."
    docker service scale ${STACK_NAME}_${service}=$replicas
}

# Show status
status() {
    log_info "Stack Status:"
    docker stack ps $STACK_NAME --format "table {{.Name}}\t{{.CurrentState}}\t{{.Node}}"

    log_info "\nService Status:"
    docker service ls | grep $STACK_NAME

    log_info "\nNetwork Status:"
    docker network ls | grep $STACK_NAME
}

# Destroy stack
destroy() {
    log_warn "Destroying stack ${STACK_NAME}..."
    docker stack rm $STACK_NAME

    log_info "Removing volumes..."
    docker volume rm -f ${STACK_NAME}_acme-certs 2>/dev/null || true
    docker volume rm -f ${STACK_NAME}_postgres-data 2>/dev/null || true
    docker volume rm -f ${STACK_NAME}_redis-data 2>/dev/null || true

    log_info "Stack destroyed"
}

# Show logs
logs() {
    local service=$1
    if [ -z "$service" ]; then
        docker service logs ${STACK_NAME}_gateway --tail 100 -f
    else
        docker service logs ${STACK_NAME}_${service} --tail 100 -f
    fi
}

# Main
case "${1:-}" in
    init)
        init_swarm
        ;;
    deploy)
        deploy_stack
        ;;
    scale)
        scale_service $2 $3
        ;;
    status)
        status
        ;;
    destroy)
        destroy
        ;;
    logs)
        logs $2
        ;;
    *)
        echo "APICerebrus Docker Swarm Deployment Script"
        echo ""
        echo "Usage: $0 <command> [options]"
        echo ""
        echo "Commands:"
        echo "  init              Initialize Docker Swarm"
        echo "  deploy            Deploy the stack"
        echo "  scale <svc> <n>   Scale a service (e.g., scale gateway 5)"
        echo "  status            Show stack status"
        echo "  logs [service]    Show logs (default: gateway)"
        echo "  destroy           Remove the stack"
        echo ""
        echo "Environment Variables:"
        echo "  NFS_SERVER        NFS server for shared ACME storage"
        echo "  ACME_EMAIL        Email for Let's Encrypt"
        echo "  JWT_SECRET        JWT signing secret"
        echo "  ADMIN_PASSWORD    Admin panel password"
        echo "  DB_PASSWORD       PostgreSQL password"
        exit 1
        ;;
esac
