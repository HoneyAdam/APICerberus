#!/bin/bash
# =============================================================================
# APICerebrus Docker Build Script
# =============================================================================
# Usage:
#   ./scripts/build-docker.sh [options]
#
# Options:
#   -t, --tag TAG       Docker image tag (default: dev)
#   -p, --push          Push image to registry
#   -r, --registry REG  Registry URL (default: ghcr.io/apicerberus)
#   -l, --latest        Tag as latest
#   -h, --help          Show this help message
#
# Examples:
#   ./scripts/build-docker.sh
#   ./scripts/build-docker.sh -t v1.0.0 -p
#   ./scripts/build-docker.sh --tag v1.0.0 --push --latest
# =============================================================================

set -euo pipefail

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Default values
TAG="dev"
PUSH=false
REGISTRY="ghcr.io/apicerberus"
LATEST=false
PLATFORMS="linux/amd64,linux/arm64"

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
        -t|--tag)
            TAG="$2"
            shift 2
            ;;
        -p|--push)
            PUSH=true
            shift
            ;;
        -r|--registry)
            REGISTRY="$2"
            shift 2
            ;;
        -l|--latest)
            LATEST=true
            shift
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

# Full image name
IMAGE_NAME="${REGISTRY}/apicerberus"
FULL_TAG="${IMAGE_NAME}:${TAG}"

log_info "Building APICerebrus Docker image..."
log_info "  Tag: ${TAG}"
log_info "  Registry: ${REGISTRY}"
log_info "  Push: ${PUSH}"
log_info "  Latest: ${LATEST}"

# Change to project root
cd "${PROJECT_ROOT}"

# Get version info
VERSION="${TAG#v}"
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)

log_info "Version: ${VERSION}"
log_info "Commit: ${COMMIT}"
log_info "Build Time: ${BUILD_TIME}"

# Check if docker buildx is available
if docker buildx version > /dev/null 2>&1; then
    BUILDER="buildx"
    log_info "Using Docker Buildx for multi-platform builds"
else
    BUILDER="docker"
    log_warn "Docker Buildx not available, using standard docker build"
fi

# Build arguments
BUILD_ARGS=(
    "--build-arg" "VERSION=${VERSION}"
    "--build-arg" "COMMIT=${COMMIT}"
    "--build-arg" "BUILD_TIME=${BUILD_TIME}"
)

# Tags
TAGS=()
TAGS+=("-t" "${FULL_TAG}")

if [[ "${LATEST}" == true ]]; then
    TAGS+=("-t" "${IMAGE_NAME}:latest")
fi

# Build command
if [[ "${BUILDER}" == "buildx" ]]; then
    # Multi-platform build
    BUILD_CMD=(
        "docker" "buildx" "build"
        "--platform" "${PLATFORMS}"
        "${BUILD_ARGS[@]}"
        "${TAGS[@]}"
        "--provenance=false"
    )

    if [[ "${PUSH}" == true ]]; then
        BUILD_CMD+=("--push")
    else
        BUILD_CMD+=("--load")
    fi

    BUILD_CMD+=("-f" "Dockerfile" ".")
else
    # Standard build
    BUILD_CMD=(
        "docker" "build"
        "${BUILD_ARGS[@]}"
        "${TAGS[@]}"
        "-f" "Dockerfile"
        "."
    )
fi

log_info "Running: ${BUILD_CMD[*]}"
"${BUILD_CMD[@]}"

if [[ $? -eq 0 ]]; then
    log_success "Docker image built successfully!"
    log_info "Image: ${FULL_TAG}"

    if [[ "${LATEST}" == true ]]; then
        log_info "Image: ${IMAGE_NAME}:latest"
    fi

    # Show image size
    if [[ "${PUSH}" == false ]]; then
        SIZE=$(docker images --format "{{.Size}}" "${FULL_TAG}" 2>/dev/null || echo "unknown")
        log_info "Image size: ${SIZE}"
    fi
else
    log_error "Docker build failed!"
    exit 1
fi

# Push if requested
if [[ "${PUSH}" == true ]]; then
    log_info "Image pushed to registry: ${FULL_TAG}"

    if [[ "${LATEST}" == true ]]; then
        log_info "Image pushed to registry: ${IMAGE_NAME}:latest"
    fi
fi

log_success "Build complete!"
