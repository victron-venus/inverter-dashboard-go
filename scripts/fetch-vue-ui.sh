#!/usr/bin/env bash
# =============================================================================
# Fetch Vue UI from inverter-dashboard-vue release
# =============================================================================
# Downloads the Vue SPA build artifacts from inverter-dashboard-vue GitHub
# releases and extracts them to internal/html/vue-ui/ for embedding.
#
# Usage:
#   ./scripts/fetch-vue-ui.sh              # Fetch from latest release
#   ./scripts/fetch-vue-ui.sh v2.1.0        # Fetch from specific tag
# =============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
VUE_UI_DIR="$PROJECT_ROOT/internal/html/vue-ui"
TAG="${1:-latest}"

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info()    { echo -e "${BLUE}[INFO]${NC}   $*"; }
log_success() { echo -e "${GREEN}[OK]${NC}     $*"; }
log_warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
log_error()   { echo -e "${RED}[ERROR]${NC} $*" >&2; }

cd "$PROJECT_ROOT"

# Determine release tag
if [ "$TAG" = "latest" ]; then
  log_info "Fetching latest release tag from victron-venus/inverter-dashboard-vue..."
  TAG=$(gh release view --repo victron-venus/inverter-dashboard-vue --json tagName --jq .tagName 2>/dev/null || echo "")
  if [ -z "$TAG" ]; then
    log_warn "No releases found"
    TAG=""
  fi
fi

# No tag means no release to fetch from
if [ -z "$TAG" ]; then
  log_warn "No Vue release available, creating placeholder..."
  mkdir -p "$VUE_UI_DIR"
  echo "<!DOCTYPE html><html><body>Vue UI not available</body></html>" > "$VUE_UI_DIR/index.html"
  log_success "Placeholder created at: $VUE_UI_DIR"
  exit 0
fi

log_info "Fetching Vue UI from: $TAG"

# Create output directory
mkdir -p "$VUE_UI_DIR"

# Create temp directory for download
TEMP_DIR=$(mktemp -d)
# shellcheck disable=SC2064
trap 'rm -rf "$TEMP_DIR"' EXIT

# Download release assets
log_info "Downloading Vue SPA artifacts from release: $TAG"
if gh release download "$TAG" \
  --repo victron-venus/inverter-dashboard-vue \
  --pattern "*.tar.gz" \
  --dir "$TEMP_DIR" 2>/dev/null; then
  log_info "Extracting artifacts..."
  for f in "$TEMP_DIR"/*.tar.gz; do
    [ -e "$f" ] || continue
    tar -xzf "$f" -C "$VUE_UI_DIR"
  done
fi

# Check if we got artifacts
if [ -d "$VUE_UI_DIR" ] && [ "$(ls -A "$VUE_UI_DIR" 2>/dev/null)" ]; then
  log_success "Vue UI extracted to: $VUE_UI_DIR"
  log_success "Vue UI directory contents:"
  find "$VUE_UI_DIR" -type f | head -10
else
  log_warn "No artifacts found for tag: $TAG"
  log_info "Creating placeholder for build without Vue UI"
  echo "<!DOCTYPE html><html><body>Vue UI not available. Build without Vue.</body></html>" > "$VUE_UI_DIR/index.html"
  log_success "Placeholder created"
fi
