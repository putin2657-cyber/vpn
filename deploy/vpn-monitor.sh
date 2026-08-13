#!/bin/bash
# Checks awg0 + basic host health. Exit 1 on failure (for cron/systemd).
# Optional Telegram alert: set TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID.
set -euo pipefail

IFACE="${AWG_IFACE:-awg0}"
MIN_MEM_MB="${MIN_MEM_MB:-80}"
MAX_DISK_PCT="${MAX_DISK_PCT:-90}"

fail() {
  echo "FAIL: $*" >&2
  if [ -n "${TELEGRAM_BOT_TOKEN:-}" ] && [ -n "${TELEGRAM_CHAT_ID:-}" ]; then
    curl -fsS -X POST "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/sendMessage" \
      -d chat_id="$TELEGRAM_CHAT_ID" \
      --data-urlencode text="VPN monitor: $*" >/dev/null || true
  fi
  exit 1
}

systemctl is-active --quiet "awg-quick@${IFACE}" || fail "${IFACE} is not active"
awg show "$IFACE" >/dev/null || fail "awg show ${IFACE} failed"

avail_mb="$(awk '/MemAvailable/ {print int($2/1024)}' /proc/meminfo)"
if [ "$avail_mb" -lt "$MIN_MEM_MB" ]; then
  fail "low memory: ${avail_mb}MB available"
fi

disk_pct="$(df -P / | awk 'END{gsub(/%/,"",$5); print $5}')"
if [ "$disk_pct" -ge "$MAX_DISK_PCT" ]; then
  fail "disk ${disk_pct}% full"
fi

echo "OK $(date -Is) iface=${IFACE} mem_avail=${avail_mb}MB disk=${disk_pct}%"
