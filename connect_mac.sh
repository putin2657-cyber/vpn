#!/bin/bash
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$PROJECT_DIR"

SERVER="${1:-localhost:8443}"
TOKEN="${2:-}"

if [ -z "$TOKEN" ]; then
    echo "Usage: sudo ./connect_mac.sh [server:port] <token>"
    echo ""
    echo "Get a token:"
    echo "  ./vpn-admin -action list"
    echo "  ./vpn-admin -action add -user alice -email alice@test.com"
    exit 1
fi

if [ ! -x "./vpn-client" ]; then
    echo "Building vpn-client..."
    go build -o vpn-client ./cmd/client-advanced/
fi

echo "Connecting to $SERVER ..."
exec sudo ./vpn-client -server "$SERVER" -token "$TOKEN"
