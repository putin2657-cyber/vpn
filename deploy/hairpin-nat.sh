#!/bin/bash
# Rebuild DNAT hairpin rules from live conntrack so two VPN clients that both
# discovered the VPS public IP (Telegram/WebRTC P2P) can still reach each other.
set -euo pipefail

PUBLIC_IP="${PUBLIC_IP:-$(ip -4 -o addr show scope global | awk 'NR==1{print $4}' | cut -d/ -f1)}"
VPN_NET="${VPN_NET:-10.8.0.0/24}"
CHAIN="AWG_HAIRPIN"

if [ -z "$PUBLIC_IP" ]; then
	echo "could not detect public IP" >&2
	exit 1
fi

iptables -t nat -N "$CHAIN" 2>/dev/null || true
iptables -t nat -F "$CHAIN"

# Jump once from PREROUTING; re-running this script only refreshes the chain body.
if ! iptables -t nat -C PREROUTING -s "$VPN_NET" -d "$PUBLIC_IP" -j "$CHAIN" 2>/dev/null; then
	iptables -t nat -A PREROUTING -s "$VPN_NET" -d "$PUBLIC_IP" -j "$CHAIN"
fi

# conntrack lines look like:
# udp 17 ... src=10.8.0.3 dst=1.2.3.4 sport=12345 dport=443 \
#            src=1.2.3.4 dst=PUBLIC sport=443 dport=56789 ...
conntrack -L -p udp 2>/dev/null | while read -r line; do
	[[ "$line" == *src=10.8.* ]] || continue
	[[ "$line" == *"dst=$PUBLIC_IP"* ]] || continue

	internal=$(sed -n 's/.*src=\(10\.8\.[0-9.]*\).*sport=\([0-9]*\).*/\1:\2/p' <<<"$line" | head -1)
	mapped=$(sed -n "s/.*dst=$PUBLIC_IP sport=[0-9]* dport=\([0-9]*\).*/\1/p" <<<"$line" | head -1)

	[ -n "$internal" ] && [ -n "$mapped" ] || continue
	iptables -t nat -A "$CHAIN" -p udp --dport "$mapped" -j DNAT --to-destination "$internal"
done

# Same for TCP (some call setups / turns use it).
conntrack -L -p tcp 2>/dev/null | while read -r line; do
	[[ "$line" == *src=10.8.* ]] || continue
	[[ "$line" == *"dst=$PUBLIC_IP"* ]] || continue

	internal=$(sed -n 's/.*src=\(10\.8\.[0-9.]*\).*sport=\([0-9]*\).*/\1:\2/p' <<<"$line" | head -1)
	mapped=$(sed -n "s/.*dst=$PUBLIC_IP sport=[0-9]* dport=\([0-9]*\).*/\1/p" <<<"$line" | head -1)

	[ -n "$internal" ] && [ -n "$mapped" ] || continue
	iptables -t nat -A "$CHAIN" -p tcp --dport "$mapped" -j DNAT --to-destination "$internal"
done
