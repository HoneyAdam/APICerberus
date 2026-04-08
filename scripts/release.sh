#!/bin/bash
# =============================================================================
# APICerebrus Release Script
# =============================================================================
# Usage:
#   ./scripts/release.sh [options] VERSION
#
# Arguments:
#   VERSION             Version to release (e.g., v1.0.0)
#
# Options:
#   -d, --dry-run       Dry run (don't actually create release)
#   -s, --skip-tests    Skip running tests
#   -b, --skip-build    Skip building binaries
#   -h, --help          Show this help message
#
# Examples:
#   ./scripts/release.sh v1.0.0
#   ./scripts/release.sh --dry-run v1.0.0
# =============================================================================

set -euo pipefail

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Default values
DRY_RUN=false
SKIP_TESTS=false
SKIP_BUILD=false
VERSION=""

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
        -d|--dry-run)
            DRY_RUN=true
            shift
            ;;
        -s|--skip-tests)
            SKIP_TESTS=true
            shift
            ;;
        -b|--skip-build)
            SKIP_BUILD=true
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
            VERSION="$1"
            shift
            ;;
    esac
done

# Validate version
if [[ -z "${VERSION}" ]]; then
    log_error "Version is required"
    show_help
    exit 1
fi

# Validate version format
if [[ ! "${VERSION}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9]+)?$ ]]; then
    log_error "Invalid version format. Expected: vX.Y.Z or vX.Y.Z-prerelease"
    exit 1
fi

log_info "Preparing release: ${VERSION}"
log_info "  Dry run: ${DRY_RUN}"
log_info "  Skip tests: ${SKIP_TESTS}"
log_info "  Skip build: ${SKIP_BUILD}"

# Change to project root
cd "${PROJECT_ROOT}"

# Check if we're in a git repository
if ! git rev-parse --git-dir > /dev/null 2>&1; then
    log_error "Not a git repository"
    exit 1
fi

# Check for uncommitted changes
if [[ -n "$(git status --porcelain)" ]]; then
    log_error "There are uncommitted changes. Please commit or stash them first."
    git status --short
    exit 1
fi

# Check if tag already exists
if git rev-parse "${VERSION}" >/dev/null 2>&1; then
    log_error "Tag ${VERSION} already exists"
    exit 1
fi

# Run tests
if [[ "${SKIP_TESTS}" == false ]]; then
    log_info "Running tests..."

    log_info "Running go vet..."
    go vet ./...

    log_info "Running unit tests..."
    go test -race ./...

    log_info "Running web build..."
    if [[ -d "web" ]]; then
        cd web
        npm ci
        npm run build
        cd "${PROJECT_ROOT}"
    fi

    log_success "All tests passed!"
else
    log_warn "Skipping tests"
fi

# Build binaries
if [[ "${SKIP_BUILD}" == false ]]; then
    log_info "Building binaries..."

    mkdir -p dist

    # Build for multiple platforms
    PLATFORMS=(
        "linux/amd64"
        "linux/arm64"
        "darwin/amd64"
        "darwin/arm64"
        "windows/amd64"
    )

    for platform in "${PLATFORMS[@]}"; do
        GOOS=${platform%/*}
        GOARCH=${platform#*/}

        OUTPUT="dist/apicerberus-${VERSION}-${GOOS}-${GOARCH}"
        if [[ "${GOOS}" == "windows" ]]; then
            OUTPUT="${OUTPUT}.exe"
        fi

        log_info "Building for ${GOOS}/${GOARCH}..."

        COMMIT=$(git rev-parse --short HEAD)
        BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)

        if [[ "${DRY_RUN}" == false ]]; then
            GOOS=${GOOS} GOARCH=${GOARCH} CGO_ENABLED=0 go build \
                -ldflags="-s -w \
                    -X github.com/APICerberus/APICerebrus/internal/version.Version=${VERSION} \
                    -X github.com/APICerberus/APICerebrus/internal/version.Commit=${COMMIT} \
                    -X github.com/APICerberus/APICerebrus/internal/version.BuildTime=${BUILD_TIME}" \
                -o "${OUTPUT}" \
                ./cmd/apicerberus

            # Create archive
            if [[ "${GOOS}" == "windows" ]]; then
                (cd dist && zip "$(basename ${OUTPUT}).zip" "$(basename ${OUTPUT})")
            else
                (cd dist && tar -czf "$(basename ${OUTPUT}).tar.gz" "$(basename ${OUTPUT})")
            fi
        else
            log_info "[DRY RUN] Would build: ${OUTPUT}"
        fi
    done

    log_success "Binaries built successfully!"
else
    log_warn "Skipping build"
fi

# Build Docker image
log_info "Building Docker image..."
if [[ "${DRY_RUN}" == false ]]; then
    ./scripts/build-docker.sh --tag "${VERSION}" --latest
else
    log_info "[DRY RUN] Would build Docker image: ${VERSION}"
fi

# Update version in files
log_info "Updating version in files..."

# Update version in version.go if it exists
if [[ -f "internal/version/version.go" ]]; then
    if [[ "${DRY_RUN}" == false ]]; then
        sed -i.bak "s/Version = \"[^\"]*\"/Version = \"${VERSION}\"/" internal/version/version.go
        rm internal/version/version.go.bak
        log_info "Updated internal/version/version.go"
    else
        log_info "[DRY RUN] Would update internal/version/version.go"
    fi
fi

# Update Chart.yaml if it exists
if [[ -f "deployments/helm/apicerberus/Chart.yaml" ]]; then
    if [[ "${DRY_RUN}" == false ]]; then
        sed -i.bak "s/^version: .*/version: ${VERSION#v}/" deployments/helm/apicerberus/Chart.yaml
        sed -i.bak "s/^appVersion: .*/appVersion: \"${VERSION}\"/" deployments/helm/apicerberus/Chart.yaml
        rm deployments/helm/apicerberus/Chart.yaml.bak
        log_info "Updated deployments/helm/apicerberus/Chart.yaml"
    else
        log_info "[DRY RUN] Would update deployments/helm/apicerberus/Chart.yaml"
    fi
fi

# Generate changelog
log_info "Generating changelog..."
PREV_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "")
if [[ -n "${PREV_TAG}" ]]; then
    CHANGELOG=$(git log --pretty=format:"- %s (%h)" "${PREV_TAG}..HEAD")
else
    CHANGELOG=$(git log --pretty=format:"- %s (%h)" -20)
fi

echo ""
echo "=== Changelog ==="
echo "${CHANGELOG}"
echo "================="
echo ""

# Create git tag
log_info "Creating git tag..."
if [[ "${DRY_RUN}" == false ]]; then
    # Commit version changes
    git add -A
    git commit -m "chore(release): prepare for ${VERSION}"

    # Create annotated tag
    git tag -a "${VERSION}" -m "Release ${VERSION}"

    log_success "Created tag: ${VERSION}"
    log_info "To push the release, run:"
    log_info "  git push origin main"
    log_info "  git push origin ${VERSION}"
else
    log_info "[DRY RUN] Would create tag: ${VERSION}"
fi

# Summary
echo ""
log_success "Release ${VERSION} prepared successfully!"
echo ""
echo "Next steps:"
if [[ "${DRY_RUN}" == false ]]; then
    echo "  1. Review the changes: git show ${VERSION}"
    echo "  2. Push the tag: git push origin ${VERSION}"
    echo "  3. Push commits: git push origin main"
    echo "  4. GitHub Actions will create the release automatically"
else
    echo "  1. Run without --dry-run to create the actual release"
fi
echo ""
