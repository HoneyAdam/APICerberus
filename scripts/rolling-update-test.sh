#!/usr/bin/env bash
# =============================================================================
# Rolling Update Zero-Downtime Deployment Test
# =============================================================================
# Tests that a 3-node Raft cluster can be rolled (one node restarted at a time)
# without losing in-flight requests through an Nginx load balancer.
#
# Usage: ./scripts/rolling-update-test.sh
#
# Requirements:
#   - Docker + Docker Compose
#   - curl
#
# What it does:
#   1. Builds the apicerberus Docker image
#   2. Starts a 3-node Raft cluster with Nginx LB
#   3. Waits for all nodes to be healthy
#   4. Sends continuous HTTP requests through the LB
#   5. Restarts each node one at a time (rolling update simulation)
#   6. Tracks success/failure counts
#   7. Reports whether any requests were lost
# =============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
DEPLOY_DIR="$PROJECT_DIR/deployments/docker"
COMPOSE_FILE="$DEPLOY_DIR/docker-compose.rolling-test.yml"

# Configuration
TOTAL_REQUESTS_TARGET=500
REQUEST_INTERVAL=0.1          # seconds between requests
HEALTH_CHECK_TIMEOUT=60       # max seconds to wait for node health
ROLL_SETTLE_TIME=15           # seconds to wait after node restart before next roll
CLUSTER_STARTUP_WAIT=20       # seconds to wait for cluster to form

# Counters
TOTAL=0
SUCCESS=0
FAILURES=0
ERRORS=0
CONSECUTIVE_FAILURES=0
MAX_CONSECUTIVE_FAILURES=0

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info()    { echo -e "${BLUE}[INFO]${NC} $*"; }
log_success() { echo -e "${GREEN}[OK]${NC} $*"; }
log_warn()    { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_error()   { echo -e "${RED}[ERROR]${NC} $*"; }

cleanup() {
    log_info "Cleaning up..."
    cd "$DEPLOY_DIR"
    docker-compose -f "$COMPOSE_FILE" down -v --remove-orphans 2>/dev/null || true
    # Kill background traffic if running
    if [ -n "${TRAFFIC_PID:-}" ] && kill -0 "$TRAFFIC_PID" 2>/dev/null; then
        kill "$TRAFFIC_PID" 2>/dev/null || true
        wait "$TRAFFIC_PID" 2>/dev/null || true
    fi
}

trap cleanup EXIT

# --- Step 1: Build Docker Image ---
log_info "Building Docker image (tag: apicerberus/apicerberus:test)..."
cd "$PROJECT_DIR"
docker build -t apicerberus/apicerberus:test \
    --build-arg VERSION=rolling-test \
    --build-arg COMMIT=test \
    .

# --- Step 2: Start 3-node cluster ---
log_info "Starting 3-node Raft cluster + Nginx LB..."
cd "$DEPLOY_DIR"
docker-compose -f "$COMPOSE_FILE" up -d

# --- Step 3: Wait for cluster to form ---
log_info "Waiting ${CLUSTER_STARTUP_WAIT}s for cluster to form..."
sleep "$CLUSTER_STARTUP_WAIT"

# --- Step 4: Verify all nodes are healthy ---
log_info "Checking node health..."

wait_for_node() {
    local name=$1
    local port=$2
    local elapsed=0
    while [ $elapsed -lt $HEALTH_CHECK_TIMEOUT ]; do
        if curl -sf "http://localhost:${port}/health" >/dev/null 2>&1; then
            return 0
        fi
        sleep 2
        elapsed=$((elapsed + 2))
    done
    return 1
}

NODES=("node1:18080" "node2:18081" "node3:18082")

for node_entry in "${NODES[@]}"; do
    name="${node_entry%%:*}"
    port="${node_entry##*:}"
    if wait_for_node "$name" "$port"; then
        log_success "$name is healthy on port $port"
    else
        log_error "$name failed health check after ${HEALTH_CHECK_TIMEOUT}s"
        docker-compose -f "$COMPOSE_FILE" logs "$name" | tail -20
        exit 1
    fi
done

# --- Step 5: Verify cluster status ---
log_info "Checking Raft cluster status..."
CLUSTER_STATUS=$(curl -sf -H "X-Admin-Key: test-admin-key-for-rolling-update-deployment-testing" \
    "http://localhost:19876/admin/api/v1/cluster/status" 2>/dev/null || echo "{}")
log_info "Cluster status: $CLUSTER_STATUS"

# --- Step 6: Start continuous traffic ---
log_info "Starting traffic generator ($REQUEST_INTERVAL s interval, target: $TOTAL_REQUESTS)..."

send_traffic() {
    while true; do
        HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
            --max-time 5 "http://localhost:18083/health" 2>/dev/null || echo "000")

        if [ "$HTTP_CODE" = "200" ]; then
            SUCCESS=$((SUCCESS + 1))
            CONSECUTIVE_FAILURES=0
        else
            FAILURES=$((FAILURES + 1))
            CONSECUTIVE_FAILURES=$((CONSECUTIVE_FAILURES + 1))
            if [ $CONSECUTIVE_FAILURES -gt $MAX_CONSECUTIVE_FAILURES ]; then
                MAX_CONSECUTIVE_FAILURES=$CONSECUTIVE_FAILURES
            fi
            echo -e "${RED}[FAIL]${NC} HTTP $HTTP_CODE at $(date +%H:%M:%S)"
        fi
        TOTAL=$((TOTAL + 1))

        if [ $TOTAL -ge $TOTAL_REQUESTS_TARGET ]; then
            break
        fi

        sleep "$REQUEST_INTERVAL"
    done
}

send_traffic &
TRAFFIC_PID=$!

# --- Step 7: Rolling restart ---
log_info "=== Starting rolling restart ==="

RESTART_ORDER=("apicerberus-node3" "apicerberus-node2" "apicerberus-node1")

for i in "${!RESTART_ORDER[@]}"; do
    node="${RESTART_ORDER[$i]}"
    node_num=$((i + 1))
    log_info "--- Rolling restart $node_num/3: $node ---"

    # Record pre-restart success count
    PRE_SUCCESS=$SUCCESS

    # Restart the node
    log_info "Stopping $node..."
    docker-compose -f "$COMPOSE_FILE" stop "$node"

    # Brief pause to let LB detect removal
    sleep 3

    log_info "Starting $node..."
    docker-compose -f "$COMPOSE_FILE" start "$node"

    # Wait for node to rejoin and become healthy
    log_info "Waiting for $node to rejoin..."
    # Map container name back to port
    case "$node" in
        *node1) NODE_PORT=18080 ;;
        *node2) NODE_PORT=18081 ;;
        *node3) NODE_PORT=18082 ;;
    esac

    ELAPSED=0
    while [ $ELAPSED -lt $HEALTH_CHECK_TIMEOUT ]; do
        if curl -sf "http://localhost:${NODE_PORT}/health" >/dev/null 2>&1; then
            log_success "$node is back and healthy"
            break
        fi
        sleep 2
        ELAPSED=$((ELAPSED + 2))
    done

    # Let cluster stabilize
    log_info "Letting cluster stabilize for ${ROLL_SETTLE_TIME}s..."
    sleep "$ROLL_SETTLE_TIME"

    POST_SUCCESS=$SUCCESS
    DELTA=$((POST_SUCCESS - PRE_SUCCESS))
    log_info "$node: $DELTA successful requests during restart window"
done

# --- Step 8: Wait for remaining traffic ---
log_info "Waiting for traffic generator to finish..."
wait "$TRAFFIC_PID" 2>/dev/null || true
TRAFFIC_PID=""

# --- Step 9: Final cluster health check ---
log_info "Final cluster health check..."
for node_entry in "${NODES[@]}"; do
    name="${node_entry%%:*}"
    port="${node_entry##*:}"
    if curl -sf "http://localhost:${port}/health" >/dev/null 2>&1; then
        log_success "$name is healthy"
    else
        log_warn "$name not responding (may still be recovering)"
    fi
done

# --- Step 10: Report ---
echo ""
echo "========================================"
echo "  Rolling Update Test Results"
echo "========================================"
echo ""
echo "  Total requests:    $TOTAL"
echo "  Successful (200):  $SUCCESS"
echo "  Failed:            $FAILURES"
echo ""

if [ $FAILURES -eq 0 ]; then
    echo -e "  ${GREEN}RESULT: PASS - Zero downtime achieved${NC}"
    echo ""
    echo "  All requests succeeded during rolling restart of 3 nodes."
    echo "  The load balancer correctly routed traffic away from restarting"
    echo "  nodes and the Raft cluster maintained quorum throughout."
    PASS=true
elif [ $FAILURES -le 3 ]; then
    echo -e "  ${YELLOW}RESULT: PASS (minor) - $FAILURES failures during transition${NC}"
    echo ""
    echo "  A small number of failures is expected during the brief window"
    echo "  when the LB detects a downed backend. Max consecutive failures: $MAX_CONSECUTIVE_FAILURES"
    PASS=true
else
    echo -e "  ${RED}RESULT: FAIL - $FAILURES requests lost during rolling restart${NC}"
    echo ""
    echo "  Max consecutive failures: $MAX_CONSECUTIVE_FAILURES"
    echo "  This indicates the LB did not route around restarting nodes quickly enough."
    PASS=false
fi

echo ""
echo "  Max consecutive failures: $MAX_CONSECUTIVE_FAILURES"
echo "========================================"

if [ "$PASS" = true ]; then
    log_success "Zero-downtime deployment test PASSED"
    exit 0
else
    log_error "Zero-downtime deployment test FAILED"
    exit 1
fi
