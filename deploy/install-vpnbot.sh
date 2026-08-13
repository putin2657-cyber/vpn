#!/bin/bash
# Installs the Telegram payment bot on the VPN VPS.
#
#   GOOS=linux GOARCH=amd64 go build -o vpn-vpnbot ./cmd/vpnbot
#   scp vpn-vpnbot deploy/vpn-vpnbot.service deploy/vpnbot.env.example \
#       deploy/install-vpnbot.sh root@VPS:/tmp/
#   ssh root@VPS 'bash /tmp/install-vpnbot.sh https://YOUR_HOST'
set -euo pipefail

ROOT=/opt/vpn
PUBLIC_BASE="${1:-}"

if [ "$(id -u)" -ne 0 ]; then
  echo "Run as root." >&2
  exit 1
fi

if [ -z "$PUBLIC_BASE" ]; then
  PUBLIC_BASE="https://138.249.141.164.sslip.io"
fi
PUBLIC_BASE="${PUBLIC_BASE%/}"
WEBHOOK_PATH="${CRYPTO_PAY_WEBHOOK_PATH:-/webhook/cryptopay}"

echo "==> Installing vpnbot binary"
install -d -m 755 "$ROOT"
if [ -f /tmp/vpn-vpnbot ]; then install -m 755 /tmp/vpn-vpnbot "$ROOT/vpn-vpnbot"; fi

if [ ! -f "$ROOT/vpnbot.env" ]; then
  if [ -f /tmp/vpnbot.env.example ]; then
    install -m 600 /tmp/vpnbot.env.example "$ROOT/vpnbot.env"
  else
    cat > "$ROOT/vpnbot.env" <<EOF
TELEGRAM_BOT_TOKEN=replace_me
CRYPTO_PAY_TOKEN=replace_me
VPBOT_DB=${ROOT}/vpn.db
VPBOT_ADDR=127.0.0.1:8091
VPBOT_SYNC_CMD=${ROOT}/sync-awg.sh
CRYPTO_PAY_WEBHOOK_PATH=${WEBHOOK_PATH}
EOF
    chmod 600 "$ROOT/vpnbot.env"
  fi
fi

echo "==> Caddy webhook route (append if missing)"
CADDY_SNIPPET="
	@cryptopay path ${WEBHOOK_PATH}*
	handle @cryptopay {
		reverse_proxy 127.0.0.1:8090
	}
"
if [ -f /etc/caddy/Caddyfile ] && ! grep -q 'path /webhook/cryptopay' /etc/caddy/Caddyfile; then
  HOST=$(echo "$PUBLIC_BASE" | sed -E 's#https?://##')
  awk -v host="$HOST" -v snippet="$CADDY_SNIPPET" '
    $0 ~ "^"host" \\{" { print; print snippet; next }
    { print }
  ' /etc/caddy/Caddyfile > /etc/caddy/Caddyfile.new
  mv /etc/caddy/Caddyfile.new /etc/caddy/Caddyfile
  systemctl reload caddy || systemctl restart caddy
fi

if [ -f /tmp/vpn-vpnbot.service ]; then
  install -m 644 /tmp/vpn-vpnbot.service /etc/systemd/system/vpn-vpnbot.service
fi
systemctl daemon-reload

echo
echo "Next:"
echo "  1) @BotFather → /newbot → paste TELEGRAM_BOT_TOKEN into $ROOT/vpnbot.env"
echo "  2) @CryptoBot → Crypto Pay → your app → paste CRYPTO_PAY_TOKEN into $ROOT/vpnbot.env"
echo "  3) Crypto Pay → Webhooks → enable:"
echo "       ${PUBLIC_BASE}${WEBHOOK_PATH}"
echo "  4) systemctl enable --now vpn-vpnbot"
echo "  5) Open your bot in Telegram and send /start"
echo
