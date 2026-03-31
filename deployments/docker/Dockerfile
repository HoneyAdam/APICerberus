# Build stage - use Go 1.25 rc
FROM golang:1.25-rc-alpine AS builder

ENV GOTOOLCHAIN=auto

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git make

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN go build -ldflags="-w -s" -o apicerberus ./cmd/apicerberus

# Runtime stage
FROM alpine:3.21

LABEL maintainer="APICerebrus Team"
LABEL description="Production-ready API Gateway with Raft clustering"

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates curl && \
    adduser -D -s /bin/sh -u 1000 apicerberus && \
    mkdir -p /data /config /certs && \
    chown -R apicerberus:apicerberus /app /data /config /certs

# Copy binary from builder
COPY --from=builder /build/apicerberus /app/apicerberus

# Copy default configs (optional)
RUN mkdir -p /config

# Set permissions
RUN chmod +x /app/apicerberus

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# Use non-root user
USER apicerberus

# Expose ports
EXPOSE 8080 8443 9090 7946

# Data volumes
VOLUME ["/data", "/config", "/certs"]

# Environment
ENV APICERBERUS_CONFIG=/config/apicerberus.yaml
ENV APICERBERUS_DATA_DIR=/data
ENV APICERBERUS_CERTS_DIR=/certs

ENTRYPOINT ["/app/apicerberus"]
CMD ["--config", "/config/apicerberus.yaml"]
