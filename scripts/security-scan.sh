#!/bin/bash
# Security Scan Script for APICerebrus
# Runs comprehensive security checks on the codebase

set -e

echo "========================================"
echo "  APICerebrus Security Scan"
echo "========================================"
echo ""

# Check if Go is installed
if ! command -v go >/dev/null 2>&1; then
    echo "Error: Go is not installed"
    exit 1
fi

GO_VERSION=$(go version | awk '{print $3}')
echo "Go version: ${GO_VERSION}"
echo ""

# Dependency Vulnerability Scan
echo "----------------------------------------"
echo "1. Dependency Vulnerability Scan"
echo "----------------------------------------"

if command -v govulncheck >/dev/null 2>&1; then
    echo "Running govulncheck..."
    govulncheck ./... 2>&1 || echo "govulncheck completed with findings"
else
    echo "govulncheck not installed. Install: go install golang.org/x/vuln/cmd/govulncheck@latest"
    echo "Running go mod verify as fallback..."
    go mod verify
fi
echo ""

# Static Analysis Security Scan
echo "----------------------------------------"
echo "2. Static Analysis Security Scan"
echo "----------------------------------------"

if command -v gosec >/dev/null 2>&1; then
    echo "Running gosec..."
    gosec -exclude-generated ./... 2>&1 || echo "gosec completed with findings"
else
    echo "gosec not installed. Install: go install github.com/securego/gosec/v2/cmd/gosec@latest"
    echo "Running go vet..."
    go vet ./...
fi
echo ""

# Go Vet Analysis
echo "----------------------------------------"
echo "3. Go Vet Analysis"
echo "----------------------------------------"

echo "Running go vet..."
go vet ./...
echo "Go vet passed"
echo ""

# Race Condition Detection
echo "----------------------------------------"
echo "4. Race Condition Detection"
echo "----------------------------------------"

echo "Running tests with race detector (short mode)..."
go test -race -count=1 -short ./... 2>&1 | head -30 || echo "Race detection tests completed"
echo ""

# Summary
echo "========================================"
echo "  Security Scan Complete"
echo "========================================"
echo ""
echo "Recommendations:"
echo "  1. Run this scan before each release"
echo "  2. Integrate security scanning into CI/CD"
echo "  3. Keep dependencies updated"
echo "  4. Review security advisories regularly"
echo ""
