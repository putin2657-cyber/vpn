package tgbot

import (
	"errors"
	"fmt"
	"strings"

	"vpn-server/pkg/awg"
	"vpn-server/pkg/billing"
	"vpn-server/pkg/db"
)

// SlotKind classifies a VPN peer for the admin inventory.
type SlotKind string

const (
	SlotOwner  SlotKind = "owner"
	SlotBusy   SlotKind = "busy"
	SlotFree   SlotKind = "free"
	SlotUnpaid SlotKind = "unpaid"
)

// Slot is one peer row in the manager view.
type Slot struct {
	Peer      *db.Peer
	Member    *db.Member
	Kind      SlotKind
	Online    bool
	Handshake string
}

// Inventory is the admin snapshot of free vs occupied configs.
type Inventory struct {
	Slots   []Slot
	Owner   int
	Busy    int
	Unpaid  int
	Free    int
	Online  int
	Total   int
	MaxSoft int // practical capacity hint for this VPS size
}

// BuildInventory lists peers and marks which configs are free to hand out.
func (s *Service) BuildInventory() (*Inventory, error) {
	peers, err := s.DB.ListPeers()
	if err != nil {
		return nil, err
	}
	live, _ := awg.ShowPeers(awg.DefaultInterface)

	inv := &Inventory{
		Slots:   make([]Slot, 0, len(peers)),
		MaxSoft: 25,
	}
	for _, peer := range peers {
		slot := Slot{Peer: peer, Handshake: "—"}
		if live != nil {
			if st, ok := live[peer.PublicKey.String()]; ok {
				slot.Handshake = st.HandshakeHuman
				// Treat recent handshakes as online (seconds/minutes).
				hs := st.HandshakeHuman
				if strings.HasSuffix(hs, "s ago") || strings.HasSuffix(hs, "m ago") {
					slot.Online = true
				}
			}
		}

		member, err := s.DB.GetMemberByPeerName(peer.Name)
		if err != nil && !errors.Is(err, db.ErrMemberNotFound) {
			return nil, err
		}
		if err == nil {
			slot.Member = member
		}

		switch {
		case billing.IsOwnerPeer(peer.Name):
			slot.Kind = SlotOwner
			inv.Owner++
		case slot.Member != nil && slot.Member.Status == db.MemberActive:
			slot.Kind = SlotBusy
			inv.Busy++
		case slot.Member != nil:
			slot.Kind = SlotUnpaid
			inv.Unpaid++
		default:
			slot.Kind = SlotFree
			inv.Free++
		}
		if slot.Online {
			inv.Online++
		}
		inv.Slots = append(inv.Slots, slot)
		inv.Total++
	}
	return inv, nil
}

// FormatInventory builds an HTML report for the admin chat.
func FormatInventory(inv *Inventory) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<b>%s · менеджер</b>\n────────────\n", BrandName)
	fmt.Fprintf(&b, "Всего: <b>%d</b> · свободно <b>%d</b>\n", inv.Total, inv.Free)
	fmt.Fprintf(&b, "Оплачено: <b>%d</b> · без оплаты <b>%d</b>\n", inv.Busy, inv.Unpaid)
	fmt.Fprintf(&b, "Owner: <b>%d</b> · онлайн <b>%d</b>\n", inv.Owner, inv.Online)

	writeOccupied := func(title string, kind SlotKind) {
		var lines []string
		for _, slot := range inv.Slots {
			if slot.Kind != kind {
				continue
			}
			label := slot.Peer.Name
			if slot.Member != nil {
				label = memberTelegramLabel(slot.Member)
			}
			on := "выкл"
			if slot.Peer.Enabled {
				on = "вкл"
			}
			extra := ""
			if slot.Online {
				extra = " · online"
			}
			if slot.Member != nil && slot.Member.Status == db.MemberActive && slot.Member.PaidUntil != "" {
				extra += " · до " + formatLocal(slot.Member.PaidUntil)
			} else if slot.Member != nil && slot.Member.Status != db.MemberActive {
				extra += " · " + slot.Member.Status
			}
			lines = append(lines, fmt.Sprintf("• <b>%s</b> (<code>%s</code>) — %s%s",
				escapeHTMLPlain(label), slot.Peer.Name, on, extra))
		}
		if len(lines) == 0 {
			return
		}
		fmt.Fprintf(&b, "\n\n<b>%s</b>\n%s", title, strings.Join(lines, "\n"))
	}
	writeOccupied("Занятые (оплачено)", SlotBusy)
	writeOccupied("Занятые (без оплаты)", SlotUnpaid)

	fmt.Fprintf(&b, "\n\nКнопки ниже — вкл/выкл. Сначала занятые, потом свободные. Owner скрыты.")
	return strings.TrimSpace(b.String())
}

// ManageKeyboard builds inline toggle buttons for non-owner peers.
// Occupied (busy/unpaid) rows come first, then free. callback_data ≤64 bytes.
func ManageKeyboard(inv *Inventory) [][]map[string]any {
	var rows [][]map[string]any
	appendSlot := func(slot Slot) {
		if slot.Kind == SlotOwner || slot.Peer == nil {
			return
		}
		name := slot.Peer.Name
		if len("peer_info:")+len(name) > 64 {
			return
		}
		label := name
		if slot.Member != nil {
			label = memberTelegramLabel(slot.Member)
		}
		if len(label) > 22 {
			label = label[:22] + "…"
		}
		state := "OFF"
		if slot.Peer.Enabled {
			state = "ON"
		}
		prefix := "св"
		switch slot.Kind {
		case SlotBusy:
			prefix = "опл"
		case SlotUnpaid:
			prefix = "долг"
		}
		if slot.Online {
			state += "·live"
		}
		title := fmt.Sprintf("%s · %s · %s", prefix, label, state)
		var action map[string]any
		if slot.Peer.Enabled {
			action = map[string]any{"text": "Выкл", "callback_data": "peer_off:" + name}
		} else {
			action = map[string]any{"text": "Вкл", "callback_data": "peer_on:" + name}
		}
		rows = append(rows, []map[string]any{
			{"text": title, "callback_data": "peer_info:" + name},
			action,
		})
	}
	for _, kind := range []SlotKind{SlotBusy, SlotUnpaid, SlotFree} {
		for _, slot := range inv.Slots {
			if slot.Kind == kind {
				appendSlot(slot)
			}
		}
	}
	rows = append(rows, []map[string]any{
		{"text": "Обновить", "callback_data": "mgr_refresh"},
	})
	return rows
}

func memberTelegramLabel(m *db.Member) string {
	if m == nil {
		return "unknown"
	}
	if u := strings.TrimSpace(m.TelegramUsername); u != "" {
		return "@" + strings.TrimPrefix(u, "@")
	}
	// Hidden username: show display name, never the device/peer id.
	if m.DisplayName != "" {
		return m.DisplayName
	}
	return m.PeerName
}

// NextFreePeer returns the first unused non-owner peer, if any.
func (s *Service) NextFreePeer() (*db.Peer, error) {
	inv, err := s.BuildInventory()
	if err != nil {
		return nil, err
	}
	for _, slot := range inv.Slots {
		if slot.Kind == SlotFree {
			return slot.Peer, nil
		}
	}
	return nil, fmt.Errorf("свободных конфигов нет — добавь peer через vpn-admin или пусть клиент оплатит в боте (создастся новый)")
}

// PeerConfig returns a smart client config for any peer (admin handout).
func (s *Service) PeerConfig(peerName string) (string, *db.Peer, error) {
	peer, err := s.DB.GetPeer(peerName)
	if err != nil {
		return "", nil, err
	}
	server, err := s.DB.Server()
	if err != nil {
		return "", nil, err
	}
	cfg, err := server.ClientConfig(awg.Peer{
		Name:         peer.Name,
		PrivateKey:   peer.PrivateKey,
		PublicKey:    peer.PublicKey,
		PresharedKey: peer.PresharedKey,
		Address:      peer.Address,
	}, awg.TunnelSmart)
	return cfg, peer, err
}
