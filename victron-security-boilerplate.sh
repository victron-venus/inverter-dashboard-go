#!/bin/bash
# Deploy security tools boilerplate to a repository

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

REPO_PATH="${1:?usage: $0 <repository-path>}"
cd "$REPO_PATH"

echo "🔒 Deploying security tools to $(basename $PWD)..."

# Ensure .github/workflows exists
mkdir -p .github/workflows

# Copy workflow files (skip if exists to preserve existing workflows)
for file in ~/victron/inverter-dashboard-go/.github/workflows/*.yml; do
  filename=$(basename "$file")
  if [ ! -f ".github/workflows/$filename" ]; then
    cp "$file" ".github/workflows/"
    echo "  ✓ Added workflow: $filename"
  fi
done

# Copy local security scripts
for script in commit.sh commit.txt release.sh security.sh .gitleaksignore; do
  if [ -f "~/victron/inverter-dashboard-go/$script" ]; then
    cp "~/victron/inverter-dashboard-go/$script" .
    chmod +x "$script" 2>/dev/null || true
    echo "  ✓ Added: $script"
  fi
done

# Check if Go or Python project
if [ -f "go.mod" ]; then
  echo "  📦 Go project detected"
  # Pinned govulncheck v1.7.0 and OSV Scanner v1.9.2; Go verifies module checksums.
  go install golang.org/x/vuln/cmd/govulncheck@617f44b718537dccdea1915395650e0529e3b72e 2>/dev/null || echo "  ⚠ govulncheck install failed"
  go install github.com/google/osv-scanner/cmd/osv-scanner@1e295ee11c5e107886e58bacb04228325082146f 2>/dev/null || echo "  ⚠ osv-scanner install failed"
elif [ -f "requirements.txt" ] || [ -f "pyproject.toml" ] || [ -f "setup.py" ]; then
  echo "  🐍 Python project detected"
  # Install Python security tools
  python3 -m pip install --require-hashes --only-binary=:all: -r "${SCRIPT_DIR}/scripts/requirements-security-tools.txt" 2>/dev/null || echo "  ⚠ Python tools install failed"
  # Create Python security workflow if needed
  if [ ! -f ".github/workflows/python-security.yml" ]; then
    cp ~/victron/inverter-dashboard-go/.github/workflows/python-security.yml .github/workflows/ 2>/dev/null || echo "  ⚠ Python workflow not found"
  fi
fi

go install github.com/zricethezav/gitleaks/v8@8d1f98c7967eb1e79cb44ac6241a124e145d2165 2>/dev/null || echo "  ⚠ gitleaks install failed"

echo ""
echo "📋 Security summary for $(basename $PWD):"
echo "  - Workflows: $(ls .github/workflows/*.yml 2>/dev/null | wc -l) files"
echo "  - Local scripts: $(ls commit.sh release.sh security.sh .gitleaksignore 2>/dev/null | wc -l) files"
echo "  - Tools ready: $(which gitleaks govulncheck osv-scanner 2>/dev/null | wc -l) available"
echo ""
echo "✅ Security tools deployed to $(basename $PWD)"
