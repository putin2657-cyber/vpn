#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")"

echo "=== Build ==="
go build -o vpn-admin ./cmd/admin/
go build -o vpn-api ./cmd/api/

echo "=== Test ==="
go test ./... -count=1

echo
echo "✅ Build and tests complete"
echo
echo "First run:"
echo "  ./vpn-admin -action init -endpoint vpn.example.com:51820 -interface eth0"
echo "  ./vpn-admin -action add -name alice-phone"
echo "  ./vpn-admin -action qr -name alice-phone"
echo
echo "Deploy to the server:"
echo "  ./vpn-admin -action server-config -out awg0.conf"
