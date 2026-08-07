#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")"

echo "=== Build ==="
go build -o vpn-admin ./cmd/admin/
go build -o vpn-api ./cmd/api/
go build -o vpn-billing ./cmd/billing/

echo "=== Test ==="
go test ./... -count=1

echo
echo "✅ Build and tests complete"
echo
echo "Run from terminal or IDE Run Configurations:"
echo "  ./vpn-admin -action list"
echo "  ./vpn-admin -action config -name my-phone -mode smart"
echo "  ./vpn-billing -action serve   # needs YOOKASSA_* in env"
echo
echo "Deploy to the server:"
echo "  ./vpn-admin -action server-config -out awg0.conf"
