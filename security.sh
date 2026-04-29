#!/bin/bash
# Security scanning wrapper - runs all available security tools

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

echo "🔒 Security Scanning Suite"
echo "========================="

TOOLS_AVAILABLE=0
TOOLS_FAILED=0

# Color codes
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Helper function
check_tool() {
 local cmd="$1"
 local description="$2"
 if command -v "$cmd" >/dev/null 2>&1; then
 echo -e "${GREEN}✓${NC} $description available"
 return 0
 else
 echo -e "${RED}✗${NC} $description not found"
 return 1
 fi
}

echo ""
echo "📋 Checking available tools..."

# 1. OSV-Scanner
if check_tool osv-scanner "OSV-Scanner"; then
 TOOLS_AVAILABLE=$((TOOLS_AVAILABLE + 1))
 echo "Running OSV-Scanner..."
 if osv-scanner --lockfile=go.mod --no-call-analysis .; then
 echo -e "${GREEN}✓${NC} OSV-Scanner passed"
 else
 echo -e "${RED}✗${NC} OSV-Scanner found issues"
 TOOLS_FAILED=$((TOOLS_FAILED + 1))
 fi
else
 TOOLS_FAILED=$((TOOLS_FAILED + 1))
fi

# 2. Gitleaks
if check_tool gitleaks "Gitleaks"; then
 TOOLS_AVAILABLE=$((TOOLS_AVAILABLE + 1))
 echo "Running Gitleaks..."
 if gitleaks detect --no-banner --exit-code 1 --redact -v .; then
 echo -e "${GREEN}✓${NC} Gitleaks passed"
 else
 echo -e "${RED}✗${NC} Gitleaks found potential secrets"
 TOOLS_FAILED=$((TOOLS_FAILED + 1))
 fi
else
 TOOLS_FAILED=$((TOOLS_FAILED + 1))
fi

# 3. govulncheck
echo ""
if check_tool govulncheck "govulncheck"; then
 TOOLS_AVAILABLE=$((TOOLS_AVAILABLE + 1))
 echo "Running govulncheck..."
 if govulncheck ./...; then
 echo -e "${GREEN}✓${NC} govulncheck passed"
 else
 echo -e "${RED}✗${NC} govulncheck found vulnerabilities"
 TOOLS_FAILED=$((TOOLS_FAILED + 1))
 fi
else
 echo -e "${YELLOW}!${NC} Install govulncheck: go install golang.org/x/vuln/cmd/govulncheck@latest"
 TOOLS_FAILED=$((TOOLS_FAILED + 1))
fi

# Summary
echo ""
echo "========================="
echo "📊 Summary:"
echo "Tools available: $TOOLS_AVAILABLE"
echo "Tools with issues: $TOOLS_FAILED"

if [ $TOOLS_FAILED -eq 0 ]; then
 echo -e "${GREEN}✅ All security checks passed!${NC}"
 exit 0
else
 echo -e "${RED}❌ Some security checks failed${NC}"
 echo "Please review the findings above"
 exit 1
fi
