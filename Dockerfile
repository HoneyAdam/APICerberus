# =============================================================================
# APICerebrus Dockerfile
# Multi-stage build for optimized production image
# =============================================================================

# -----------------------------------------------------------------------------
# Stage 1: Web Dashboard Builder
# -----------------------------------------------------------------------------
FROM node:20-alpine AS web-builder

WORKDIR /build/web

# Copy package files for better layer caching
COPY web/package*.json ./

# Install dependencies
RUN npm ci --prefer-offline --no-audit

# Copy web source code
COPY web/ ./

# Build the web dashboard
RUN npm run build

# -----------------------------------------------------------------------------
# Stage 2: Go Builder
# -----------------------------------------------------------------------------
FROM golang:1.25-alpine AS go-builder

# Install build dependencies
RUN apk add --no-cache git make ca-certificates tzdata

WORKDIR /build

# Copy go mod files for dependency caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Copy built web assets from web-builder
COPY --from=web-builder /build/web/dist ./web/dist

# Build arguments for version info
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME

# Build the binary with optimizations
# -s: omit symbol table
# -w: omit DWARF symbol table
# -extldflags "-static": static linking
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w \
        -X github.com/APICerberus/APICerebrus/internal/version.Version=${VERSION} \
        -X github.com/APICerberus/APICerebrus/internal/version.Commit=${COMMIT} \
        -X github.com/APICerberus/APICerebrus/internal/version.BuildTime=${BUILD_TIME} \
        -extldflags '-static'" \
    -a -installsuffix cgo \
    -o apicerberus \
    ./cmd/apicerberus

# -----------------------------------------------------------------------------
# Stage 3: Final Runtime Image (Distroless)
# -----------------------------------------------------------------------------
FROM gcr.io/distroless/static:nonroot

LABEL maintainer="APICerebrus Team <maintainers@apicerberus.com>"
LABEL description="APICerebrus - Production-ready API Gateway with Raft clustering"
LABEL org.opencontainers.image.source="https://github.com/APICerberus/APICerebrus"
LABEL org.opencontainers.image.licenses="MIT"

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=go-builder /build/apicerberus /app/apicerberus

# Copy CA certificates from builder
COPY --from=go-builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy timezone data
COPY --from=go-builder /usr/share/zoneinfo /usr/share/zoneinfo

# Use non-root user (distroless provides nonroot:65532)
USER nonroot:nonroot

# Expose ports
# 8080: HTTP Gateway
# 8443: HTTPS Gateway
# 9876: Admin API
# 9877: Portal
# 50051: gRPC
# 12000: Raft clustering
EXPOSE 8080 8443 9876 9877 50051 12000

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/app/apicerberus", "health"] || exit 1

# Set environment variables
ENV APICERBERUS_CONFIG=/config/apicerberus.yaml \
    APICERBERUS_DATA_DIR=/data \
    APICERBERUS_CERTS_DIR=/certs

# Use ENTRYPOINT for the binary, CMD for default arguments
ENTRYPOINT ["/app/apicerberus"]
CMD ["--config", "/config/apicerberus.yaml"]

# -----------------------------------------------------------------------------
# Alternative: Alpine-based image (uncomment to use instead of distroless)
# -----------------------------------------------------------------------------
# FROM alpine:3.21 AS alpine-runtime
#
# RUN apk add --no-cache ca-certificates curl tzdata \
#     && adduser -D -s /bin/sh -u 1000 apicerberus \
#     && mkdir -p /data /config /certs \
#     && chown -R apicerberus:apicerberus /app /data /config /certs
#
# WORKDIR /app
# COPY --from=go-builder /build/apicerberus /app/apicerberus
# RUN chmod +x /app/apicerberus
#
# USER apicerberus
# EXPOSE 8080 8443 9876 9877 50051 12000
#
# HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
#     CMD curl -f http://localhost:8080/health || exit 1
#
# ENV APICERBERUS_CONFIG=/config/apicerberus.yaml \
#     APICERBERUS_DATA_DIR=/data \
#     APICERBERUS_CERTS_DIR=/certs
#
# ENTRYPOINT ["/app/apicerberus"]
# CMD ["--config", "/config/apicerberus.yaml"]
