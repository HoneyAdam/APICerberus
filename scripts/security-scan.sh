#!/bin/bash
set -e

echo "🔐 APICerebrus Security Scan"
echo "=============================="

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo ""
echo "📦 Installing security tools..."

# Install gosec if not present
if ! command -v gosec &> /dev/null; then
    echo "Installing gosec..."
    go install github.com/securego/gosec/v2/cmd/gosec@latest
fi

# Install govulncheck if not present
if ! command -v govulncheck &> /dev/null; then
    echo "Installing govulncheck..."
    go install golang.org/x/vuln/cmd/govulncheck@latest
fi

echo ""
echo "🔍 Running gosec security scan..."
gosec -fmt sarif -out security-report.sarif ./... || true
gosec ./... || true

echo ""
echo "🔍 Running govulncheck vulnerability scan..."
govulncheck ./... || true

echo ""
echo "🔍 Checking for hardcoded secrets..."
# Check for common patterns
grep -r "password.*=" --include="*.go" internal/ | grep -v "_test.go" | grep -v "HashPassword" | head -20 || true

echo ""
echo "🔍 Checking for weak TLS configurations..."
grep -r "TLS.*VersionTLS10\|TLS.*VersionTLS11" --include="*.go" internal/ || echo "${GREEN}✓ No weak TLS versions found${NC}"

echo ""
echo "🔍 Checking for SQL injection risks..."
grep -r "fmt.Sprintf.*SELECT\|fmt.Sprintf.*INSERT\|fmt.Sprintf.*UPDATE\|fmt.Sprintf.*DELETE" --include="*.go" internal/store/ || echo "${GREEN}✓ No SQL injection risks found${NC}"

echo ""
echo "✅ Security scan complete!"
echo ""
echo "Reports:"
echo "  - SARIF: security-report.sarif"
echo ""
