#!/bin/bash
# Installs the family billing portal behind Caddy on the VPN VPS.
#
#   scp vpn-billing vpn-admin deploy/sync-awg.sh deploy/vpn-billing.service \
#       deploy/billing.env.example deploy/install-billing.sh root@VPS:/tmp/
#   ssh root@VPS 'bash /tmp/install-billing.sh'
set -euo pipefail

ROOT=/opt/vpn
PUBLIC_HOST="${1:-}"

if [ "$(id -u)" -ne 0 ]; then
  echo "Run as root." >&2
  exit 1
fi

if [ -z "$PUBLIC_HOST" ]; then
  # Default sticky hostname for the current SpaceWeb VPS IP.
  PUBLIC_HOST="138.249.141.164.sslip.io"
fi

echo "==> Installing Caddy"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq debian-keyring debian-archive-keyring apt-transport-https curl
if ! command -v caddy >/dev/null 2>&1; then
  curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' \
    | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
  curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' \
    | tee /etc/apt/sources.list.d/caddy-stable.list >/dev/null
  apt-get update -qq
  apt-get install -y -qq caddy
fi

echo "==> Installing billing binaries"
install -d -m 755 "$ROOT"
if [ -f /tmp/vpn-billing ]; then install -m 755 /tmp/vpn-billing "$ROOT/vpn-billing"; fi
if [ -f /tmp/vpn-admin ]; then install -m 755 /tmp/vpn-admin "$ROOT/vpn-admin"; fi
if [ -f /tmp/sync-awg.sh ]; then install -m 755 /tmp/sync-awg.sh "$ROOT/sync-awg.sh"; fi
if [ -f /tmp/vpn.db ]; then install -m 600 /tmp/vpn.db "$ROOT/vpn.db"; fi

if [ ! -f "$ROOT/billing.env" ]; then
  if [ -f /tmp/billing.env.example ]; then
    install -m 600 /tmp/billing.env.example "$ROOT/billing.env"
  else
    cat > "$ROOT/billing.env" <<EOF
YOOKASSA_SHOP_ID=replace_me
YOOKASSA_SECRET_KEY=replace_me
BILLING_PUBLIC_URL=https://${PUBLIC_HOST}
BILLING_ADMIN_TOKEN=$(openssl rand -hex 24)
BILLING_ADDR=127.0.0.1:8088
BILLING_DB=${ROOT}/vpn.db
BILLING_SYNC_CMD=${ROOT}/sync-awg.sh
EOF
    chmod 600 "$ROOT/billing.env"
  fi
  # Always stamp the public URL we install with.
  sed -i "s|^BILLING_PUBLIC_URL=.*|BILLING_PUBLIC_URL=https://${PUBLIC_HOST}|" "$ROOT/billing.env"
fi

echo "==> Caddy reverse proxy for ${PUBLIC_HOST}"
cat > /etc/caddy/Caddyfile <<EOF
${PUBLIC_HOST} {
	encode gzip
	reverse_proxy 127.0.0.1:8088
}
EOF
systemctl enable caddy
systemctl restart caddy

echo "==> systemd unit"
if [ -f /tmp/vpn-billing.service ]; then
  install -m 644 /tmp/vpn-billing.service /etc/systemd/system/vpn-billing.service
else
  install -m 644 "$ROOT/vpn-billing.service" /etc/systemd/system/vpn-billing.service 2>/dev/null || true
fi
systemctl daemon-reload

echo
echo "Next:"
echo "  1) Edit $ROOT/billing.env — put real YOOKASSA_SHOP_ID / YOOKASSA_SECRET_KEY"
echo "  2) systemctl enable --now vpn-billing"
echo "  3) In YooKassa cabinet set webhook URL to:"
echo "       https://${PUBLIC_HOST}/webhook/yookassa"
echo "     events: payment.succeeded, payment.canceled"
echo "  4) Invite a relative:"
echo "       $ROOT/vpn-billing -action invite -name Mama -peer device-03"
echo
