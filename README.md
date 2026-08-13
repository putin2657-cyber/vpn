# Self-hosted AmneziaWG VPN

A small VPN for about ten people: an AmneziaWG server on a VPS abroad, managed
from the command line, used through the official Amnezia apps on macOS, Android,
iOS, Windows and Linux.

AmneziaWG is WireGuard with junk traffic and randomised header magics. Plain
WireGuard is easy to recognise and gets throttled on some networks; the extra
parameters change the packet sizes and message types that give it away.

## Architecture

```
┌──────────────┐        AmneziaWG (UDP)        ┌─────────────────┐
│ Amnezia app  │ ◄────────────────────────────►│  VPS abroad     │
│ Mac/iOS/     │                               │  awg0 10.8.0.1  │──► internet
│ Android/etc  │                               │  NAT + forward  │
└──────────────┘                               └────────┬────────┘
                                                        │
       vpn-admin ──── writes awg0.conf ─────────────────┘
           │
           └── vpn.db: server identity + peers
```

The server itself is stock AmneziaWG. This project is the management layer: it
generates keys, assigns addresses, renders configs and prints QR codes.

## Requirements

- A VPS outside your country's filtering, Debian or Ubuntu
- Go 1.24 or newer to build the tools

Running the server at home does not help if the goal is a different exit
address: traffic leaves from wherever the server sits.

## Set up

Build the tools:

```bash
./test_vpn.sh
```

Create the server identity once. The endpoint is how clients reach the VPS, and
the interface is its internet-facing NIC (`ip route show default` will name it):

```bash
./vpn-admin -action init -endpoint 203.0.113.10:51820 -interface eth0
```

Re-running `init` is refused on purpose: new keys would invalidate every config
already handed out.

Add a peer per device, not per person — each one needs its own address:

```bash
./vpn-admin -action add -name alice-phone
./vpn-admin -action add -name alice-laptop
```

## Hand configs to people

By default clients get a **full tunnel**: Telegram, YouTube, CryptoBot/Wallet,
Apple and the rest of the public internet go through the VPS
(`AllowedIPs = 0.0.0.0/0, ::/0`). A true “everything except RU” CIDR list is too
large for mobile Amnezia imports — if a bank or Gosuslugi must stay on the ISP
IP, exclude that site/app in the Amnezia client UI.

Print a config to paste into a desktop client:

```bash
./vpn-admin -action config -name alice-laptop
```

Or show a QR code, which is how the phone apps import a tunnel:

```bash
./vpn-admin -action qr -name alice-phone
```

`-mode smart` and `-mode full` currently produce the same routing.

Configs contain the peer's private key. Send them over something private.

## Deploy

Generate the server config and copy it over:

```bash
./vpn-admin -action server-config -out awg0.conf
scp awg0.conf deploy/install.sh root@your-vps:/tmp/
ssh root@your-vps 'bash /tmp/install.sh /tmp/awg0.conf'
```

The script installs AmneziaWG, enables IP forwarding and starts
`awg-quick@awg0`. NAT rules live in the config's `PostUp` lines, so the tunnel
brings them up and tears them down with itself.

Open the UDP port in your provider's firewall — that step is outside the VPS and
is the usual reason a first attempt hangs.

After adding or removing peers, ship a fresh config:

```bash
./vpn-admin -action server-config -out awg0.conf
scp awg0.conf root@your-vps:/etc/amnezia/amneziawg/awg0.conf
ssh root@your-vps 'systemctl restart awg-quick@awg0'
```

## Clients

Install the Amnezia app on each device, import the config or scan the QR code.
The apps handle reconnection, sleep and network changes themselves.

With the default smart profile, `curl` to a random site still shows your ISP IP.
Check a tunneled destination instead (or export `-mode full` once to verify the
exit address):

```bash
./vpn-admin -action config -name alice-phone -mode full -out /tmp/full.conf
# import /tmp/full.conf, then:
curl -s https://api.ipify.org
```

## Live peer sync and status

On the VPS, `add` / `remove` / `sync` apply peers with `awg set` — no tunnel
restart for everyone else:

```bash
./vpn-admin -action add -name guest-01 -server-conf /etc/amnezia/amneziawg/awg0.conf
./vpn-admin -action list          # handshake + RX/TX from `awg show`
./vpn-admin -action sync -server-conf /etc/amnezia/amneziawg/awg0.conf
```

Health timer + Mac status API live under `deploy/vpn-monitor.*` and
`cmd/statusd`. The old Electron panel was removed; use `macos/VPNControl`
(SwiftUI) and `android/FamilyVPN` instead.

## Family billing (120 ₽ / month)

A small portal on the same VPS lets relatives **register by email**, pay a
symbolic **120 RUB** through [YooKassa](https://yookassa.ru/), save the card,
and auto-renew every 30 days. After payment they download their Amnezia config
from the personal cabinet — you do not collect cards or hand-make invite links.

If renewal fails there is a **3-day grace**, then that peer is dropped from
`awg0.conf` until they pay again.

```bash
GOOS=linux GOARCH=amd64 go build -o vpn-billing ./cmd/billing
GOOS=linux GOARCH=amd64 go build -o vpn-admin ./cmd/admin
scp vpn-billing vpn-admin vpn.db deploy/sync-awg.sh deploy/vpn-billing.service \
  deploy/billing.env.example deploy/install-billing.sh root@YOUR_VPS:/tmp/
ssh root@YOUR_VPS 'bash /tmp/install-billing.sh'
```

Edit `/opt/vpn/billing.env` with your YooKassa shop id and secret, enable the
unit, and point the YooKassa HTTP notifications to
`https://<host>/webhook/yookassa`.

Share the public site URL (for example `https://138.249.141.164.sslip.io`).
People open **Создать аккаунт**, pay, then **Скачать конфиг**.

These peers are reserved for the admin and never go through billing:
`my-phone`, `my-second`, `device-03`, `device-06`. Legacy invite links still
work if you need them:

```bash
ssh root@YOUR_VPS '/opt/vpn/vpn-billing -action invite -name Mama -peer device-04'
```

## Layout

| Path | Description |
|------|-------------|
| `pkg/awg` | Keys, configs, live `awg` apply + show parsing |
| `pkg/billing` | 120 ₽ subscriptions, renewals, peer sync |
| `pkg/yookassa` | YooKassa Payments API client |
| `pkg/db` | Server identity, peers and members in SQLite |
| `cmd/admin` | CLI: peers, QR, live sync, handshake list |
| `cmd/billing` | Family payment portal |
| `cmd/statusd` | Status JSON for the Mac control app |
| `cmd/api` | Optional loopback peer API |
| `deploy` | VPS install, billing, monitor units |
| `macos/VPNControl` | Native Mac status utility (SwiftUI) |
| `android/FamilyVPN` | Own Android app (AmneziaWG tunnel, no Amnezia UI) |

## Notes

- `vpn.db` holds every peer's private key, so the database is as sensitive as
  the configs themselves. Back it up somewhere encrypted.
- Obfuscation parameters are generated per deployment and stored in the
  database; server and clients must use identical values.
- If the VPS address gets blocked, rotating it means a new `-endpoint` and
  reissuing configs. Pick a provider where changing the address is cheap.
