#!/usr/bin/env bash
set -euo pipefail
PROV="${1:-}"
if [[ -z "$PROV" || ! -f "$PROV" ]]; then
  echo "Usage: $0 path/to/profile.mobileprovision"
  exit 1
fi
echo "=== Entitlements / AppID ==="
security cms -D -i "$PROV" 2>/dev/null | plutil -p - | sed -n '/Entitlements/,/}/p;/Name/p;/TeamIdentifier/p;/application-identifier/p;/networkextension/p;/application-groups/p'
