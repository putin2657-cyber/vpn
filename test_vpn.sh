#!/bin/bash
set -euo pipefail

PROJECT_DIR="/Users/elizavetalistopad/CLionProjects/untitled3"
cd "$PROJECT_DIR"

echo "=== VPN Build ==="
go build -o vpn-server ./cmd/server/
go build -o vpn-admin ./cmd/admin/
go build -o vpn-api ./cmd/api/
go build -o vpn-test-client ./cmd/client/
go build -o vpn-client ./cmd/client-advanced/

echo "✅ Build complete"
echo ""
echo "Mac (requires sudo for TUN):"
echo "  Terminal 1: ./vpn-server -addr :8443 -cert server.crt -key server.key"
echo "  Terminal 2: ./vpn-admin -action add -user alice -email alice@test.com"
echo "  Terminal 3: sudo ./connect_mac.sh localhost:8443 <TOKEN>"
echo ""
echo "Test without TUN:"
echo "  ./vpn-client -server localhost:8443 -token <TOKEN> -no-tun"
