#!/bin/bash
# Deploy security tools boilerplate to a repository

set -euo pipefail

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
  # Install Go security tools
  go install golang.org/x/vuln/cmd/govulncheck@latest 2>/dev/null || echo "  ⚠ govulncheck install failed"
  go install github.com/google/osv-scanner/cmd/osv-scanner@latest 2>/dev/null || echo "  ⚠ osv-scanner install failed"
elif [ -f "requirements.txt" ] || [ -f "pyproject.toml" ] || [ -f "setup.py" ]; then
  echo "  🐍 Python project detected"
  # Install Python security tools
  pip install bandit safety 2>/dev/null || echo "  ⚠ Python tools install failed"
  # Create Python security workflow if needed
  if [ ! -f ".github/workflows/python-security.yml" ]; then
    cp ~/victron/inverter-dashboard-go/.github/workflows/python-security.yml .github/workflows/ 2>/dev/null || echo "  ⚠ Python workflow not found"
  fi
fi

go install github.com/zricethezav/gitleaks/v8@latest 2>/dev/null || echo "  ⚠ gitleaks install failed"

echo ""
echo "📋 Security summary for $(basename $PWD):"
echo "  - Workflows: $(ls .github/workflows/*.yml 2>/dev/null | wc -l) files"
echo "  - Local scripts: $(ls commit.sh release.sh security.sh .gitleaksignore 2>/dev/null | wc -l) files"
echo "  - Tools ready: $(which gitleaks govulncheck osv-scanner 2>/dev/null | wc -l) available"
echo ""
echo "✅ Security tools deployed to $(basename $PWD)"
