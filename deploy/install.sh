#!/bin/bash
# Installs AmneziaWG on a fresh Debian or Ubuntu VPS and brings up the tunnel.
#
#   scp awg0.conf root@your-vps:/tmp/
#   scp deploy/install.sh root@your-vps:/tmp/
#   ssh root@your-vps 'bash /tmp/install.sh /tmp/awg0.conf'
set -euo pipefail

CONFIG_SRC="${1:-}"
INTERFACE="awg0"
CONFIG_DIR="/etc/amnezia/amneziawg"

if [ "$(id -u)" -ne 0 ]; then
    echo "Run as root." >&2
    exit 1
fi

if [ -z "$CONFIG_SRC" ] || [ ! -f "$CONFIG_SRC" ]; then
    echo "Usage: bash install.sh /path/to/awg0.conf" >&2
    echo "Generate it with: ./vpn-admin -action server-config -out awg0.conf" >&2
    exit 1
fi

echo "==> Installing AmneziaWG"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq software-properties-common iptables

# The kernel module has to match the running kernel, so headers come first.
apt-get install -y -qq "linux-headers-$(uname -r)" || {
    echo "No headers for kernel $(uname -r); trying the generic metapackage." >&2
    apt-get install -y -qq linux-headers-generic
}

if ! grep -rq "amnezia" /etc/apt/sources.list.d/ 2>/dev/null; then
    add-apt-repository -y ppa:amnezia/ppa
    apt-get update -qq
fi
apt-get install -y -qq amneziawg amneziawg-tools

echo "==> Installing the configuration"
install -d -m 700 "$CONFIG_DIR"
install -m 600 "$CONFIG_SRC" "$CONFIG_DIR/$INTERFACE.conf"

# The port the config listens on has to be reachable from outside.
LISTEN_PORT="$(awk -F' *= *' '/^ListenPort/ {print $2}' "$CONFIG_DIR/$INTERFACE.conf")"
echo "==> Tunnel will listen on UDP ${LISTEN_PORT:-51820}"

echo "==> Enabling IP forwarding"
cat > /etc/sysctl.d/99-amneziawg.conf <<'EOF'
net.ipv4.ip_forward = 1
EOF
sysctl -q --system

echo "==> Starting the tunnel"
# Restart rather than start so re-running the script picks up a new config.
systemctl enable "awg-quick@$INTERFACE" >/dev/null 2>&1 || true
systemctl restart "awg-quick@$INTERFACE"

echo
echo "==> Status"
systemctl --no-pager --lines=5 status "awg-quick@$INTERFACE" || true
echo
awg show || true

cat <<EOF

Done. Check that UDP ${LISTEN_PORT:-51820} is open in your provider's firewall,
then import a client config and confirm your public address changes:

    curl -s https://api.ipify.org

To apply peer changes later, copy a fresh awg0.conf and run:

    systemctl restart awg-quick@$INTERFACE
EOF
