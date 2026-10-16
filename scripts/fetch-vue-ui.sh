#!/usr/bin/env bash
# Import one explicit published Vue release, preserving verified provenance.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ $# != 1 ]]; then
  echo "Usage: $0 v2.2.8-rc.1 (an explicit published version is required)" >&2
  exit 2
fi
exec python3 "$SCRIPT_DIR/import_vue_release.py" "$1"
