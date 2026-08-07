#!/bin/bash
# Rewrites awg0.conf from vpn.db (enabled peers only) and restarts the tunnel.
set -euo pipefail

ROOT="${VPN_ROOT:-/opt/vpn}"
DB="${BILLING_DB:-$ROOT/vpn.db}"
CONF_DIR=/etc/amnezia/amneziawg
ADMIN="$ROOT/vpn-admin"

if [ ! -x "$ADMIN" ]; then
  echo "missing $ADMIN" >&2
  exit 1
fi

tmp="$(mktemp)"
"$ADMIN" -db "$DB" -action server-config -out "$tmp"
install -m 600 "$tmp" "$CONF_DIR/awg0.conf"
rm -f "$tmp"
systemctl restart awg-quick@awg0
echo "awg0 synced from $DB"
