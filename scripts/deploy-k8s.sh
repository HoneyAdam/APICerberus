#!/bin/bash
# =============================================================================
# APICerebrus Kubernetes Deployment Script
# =============================================================================
# Usage:
#   ./scripts/deploy-k8s.sh [options] ENVIRONMENT
#
# Arguments:
#   ENVIRONMENT         Target environment (development, staging, production)
#
# Options:
#   -v, --version VER   Version to deploy (default: latest)
#   -n, --namespace NS  Kubernetes namespace (default: apicerberus-<env>)
#   -i, --image IMG     Docker image to deploy
#   --dry-run           Show what would be deployed without applying
#   --rollback          Rollback to previous version
#   --status            Check deployment status
#   --logs              Show logs after deployment
#   -h, --help          Show this help message
#
# Examples:
#   ./scripts/deploy-k8s.sh development
#   ./scripts/deploy-k8s.sh -v v1.0.0 production
#   ./scripts/deploy-k8s.sh --dry-run staging
# =============================================================================

set -euo pipefail

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Default values
ENVIRONMENT=""
VERSION="latest"
NAMESPACE=""
IMAGE=""
DRY_RUN=false
ROLLBACK=false
STATUS=false
LOGS=false
WAIT_TIMEOUT="300s"

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

# Show help
show_help() {
    sed -n '/^# Usage:/,/^# ===/p' "$0" | sed 's/^# //'
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -v|--version)
            VERSION="$2"
            shift 2
            ;;
        -n|--namespace)
            NAMESPACE="$2"
            shift 2
            ;;
        -i|--image)
            IMAGE="$2"
            shift 2
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --rollback)
            ROLLBACK=true
            shift
            ;;
        --status)
            STATUS=true
            shift
            ;;
        --logs)
            LOGS=true
            shift
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        -*)
            log_error "Unknown option: $1"
            show_help
            exit 1
            ;;
        *)
            ENVIRONMENT="$1"
            shift
            ;;
    esac
done

# Validate environment
if [[ -z "${ENVIRONMENT}" ]]; then
    log_error "Environment is required (development, staging, production)"
    show_help
    exit 1
fi

if [[ ! "${ENVIRONMENT}" =~ ^(development|staging|production)$ ]]; then
    log_error "Invalid environment. Must be: development, staging, or production"
    exit 1
fi

# Set default namespace
if [[ -z "${NAMESPACE}" ]]; then
    NAMESPACE="apicerberus-${ENVIRONMENT}"
fi

# Set default image
if [[ -z "${IMAGE}" ]]; then
    IMAGE="ghcr.io/apicerberus/apicerberus:${VERSION}"
fi

log_info "Deploying APICerebrus to Kubernetes"
log_info "  Environment: ${ENVIRONMENT}"
log_info "  Namespace: ${NAMESPACE}"
log_info "  Version: ${VERSION}"
log_info "  Image: ${IMAGE}"
log_info "  Dry run: ${DRY_RUN}"

# Check prerequisites
if ! command -v kubectl &> /dev/null; then
    log_error "kubectl is not installed"
    exit 1
fi

if ! command -v kustomize &> /dev/null; then
    log_warn "kustomize not found, trying kubectl built-in kustomize"
    KUSTOMIZE="kubectl apply -k"
else
    KUSTOMIZE="kustomize build"
fi

# Check cluster connection
if ! kubectl cluster-info &> /dev/null; then
    log_error "Cannot connect to Kubernetes cluster"
    exit 1
fi

log_success "Connected to Kubernetes cluster"

# Handle rollback
if [[ "${ROLLBACK}" == true ]]; then
    log_info "Rolling back deployment..."

    if [[ "${DRY_RUN}" == false ]]; then
        kubectl rollout undo deployment/apicerberus -n "${NAMESPACE}"
        kubectl rollout status deployment/apicerberus -n "${NAMESPACE}" --timeout="${WAIT_TIMEOUT}"
        log_success "Rollback completed!"
    else
        log_info "[DRY RUN] Would rollback deployment in namespace: ${NAMESPACE}"
    fi

    exit 0
fi

# Handle status check
if [[ "${STATUS}" == true ]]; then
    log_info "Checking deployment status..."

    echo ""
    echo "=== Pods ==="
    kubectl get pods -n "${NAMESPACE}" -l app.kubernetes.io/name=apicerberus

    echo ""
    echo "=== Services ==="
    kubectl get svc -n "${NAMESPACE}"

    echo ""
    echo "=== Ingress ==="
    kubectl get ingress -n "${NAMESPACE}" 2>/dev/null || echo "No ingress found"

    echo ""
    echo "=== Deployment Status ==="
    kubectl get deployment -n "${NAMESPACE}" -l app.kubernetes.io/name=apicerberus

    echo ""
    echo "=== Events ==="
    kubectl get events -n "${NAMESPACE}" --field-selector reason=Failed --sort-by='.lastTimestamp' | tail -10 || true

    exit 0
fi

# Change to project root
cd "${PROJECT_ROOT}"

# Set overlay path
OVERLAY_PATH="deployments/kubernetes/overlays/${ENVIRONMENT}"

if [[ ! -d "${OVERLAY_PATH}" ]]; then
    log_error "Overlay not found: ${OVERLAY_PATH}"
    exit 1
fi

# Create namespace if it doesn't exist
if [[ "${DRY_RUN}" == false ]]; then
    kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
    log_success "Namespace ready: ${NAMESPACE}"
else
    log_info "[DRY RUN] Would create namespace: ${NAMESPACE}"
fi

# Update image in kustomization if version is specified
if [[ "${VERSION}" != "latest" ]]; then
    log_info "Updating image tag to ${VERSION}..."

    # Create temporary kustomization with updated image
    cat > "${OVERLAY_PATH}/kustomization.yaml.tmp" << EOF
images:
  - name: ghcr.io/apicerberus/apicerberus
    newTag: ${VERSION}
EOF

    # Merge with existing kustomization
    if [[ -f "${OVERLAY_PATH}/kustomization.yaml" ]]; then
        cat "${OVERLAY_PATH}/kustomization.yaml" >> "${OVERLAY_PATH}/kustomization.yaml.tmp"
    fi

    mv "${OVERLAY_PATH}/kustomization.yaml.tmp" "${OVERLAY_PATH}/kustomization.yaml"
fi

# Build manifests
log_info "Building Kubernetes manifests..."

if [[ "${DRY_RUN}" == false ]]; then
    if command -v kustomize &> /dev/null; then
        kustomize build "${OVERLAY_PATH}" | kubectl apply -f -
    else
        kubectl apply -k "${OVERLAY_PATH}"
    fi
else
    if command -v kustomize &> /dev/null; then
        log_info "[DRY RUN] Manifests that would be applied:"
        kustomize build "${OVERLAY_PATH}"
    else
        log_info "[DRY RUN] Would apply manifests from: ${OVERLAY_PATH}"
    fi
fi

if [[ "${DRY_RUN}" == false ]]; then
    log_success "Manifests applied!"

    # Wait for rollout
    log_info "Waiting for deployment to complete..."
    if kubectl rollout status deployment/apicerberus -n "${NAMESPACE}" --timeout="${WAIT_TIMEOUT}"; then
        log_success "Deployment completed successfully!"
    else
        log_error "Deployment failed or timed out"

        # Show pod status
        echo ""
        echo "=== Pod Status ==="
        kubectl get pods -n "${NAMESPACE}" -l app.kubernetes.io/name=apicerberus

        # Show recent events
        echo ""
        echo "=== Recent Events ==="
        kubectl get events -n "${NAMESPACE}" --sort-by='.lastTimestamp' | tail -20

        exit 1
    fi

    # Show logs if requested
    if [[ "${LOGS}" == true ]]; then
        echo ""
        log_info "Showing logs..."
        kubectl logs -n "${NAMESPACE}" -l app.kubernetes.io/name=apicerberus --tail=50 --follow
    fi
else
    log_info "[DRY RUN] Would wait for rollout to complete"
fi

# Summary
echo ""
log_success "Deployment to ${ENVIRONMENT} completed!"
echo ""
echo "Useful commands:"
echo "  kubectl get pods -n ${NAMESPACE}"
echo "  kubectl logs -n ${NAMESPACE} -l app.kubernetes.io/name=apicerberus"
echo "  kubectl port-forward -n ${NAMESPACE} svc/apicerberus 8080:8080"
echo ""
