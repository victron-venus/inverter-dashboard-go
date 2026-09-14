#!/usr/bin/env bash
# Build local artifacts without publishing or deploying. Dependencies must be installed.
set -euo pipefail
cd "$(dirname "$0")/.."
VERSION="${1:?Usage: release-build.sh X.Y.Z [nightly|beta|rc]}"
CHANNEL="${2:-rc}"
python3 scripts/check-release-version.py "$VERSION" "$CHANNEL"
PACKAGE_VERSION=$(tr -d '\r\n' < VERSION)
mkdir -p release-output
test -s internal/html/vue-ui/index.html
go build -trimpath -ldflags "-s -w -X main.Version=$PACKAGE_VERSION" -o release-output/inverter-dashboard .
test "$(release-output/inverter-dashboard --version)" = "Inverter Dashboard v$PACKAGE_VERSION"
python3 scripts/write_binary_build_metadata.py release-output/inverter-dashboard release-output/build-info.json
tar -czf "release-output/inverter-dashboard-$(uname -s)-$(uname -m).tar.gz" -C release-output inverter-dashboard build-info.json
