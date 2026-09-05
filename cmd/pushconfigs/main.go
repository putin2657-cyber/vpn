// One-shot: push regenerated AmneziaWG configs to every Telegram-linked member.
package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"vpn-server/pkg/awg"
	"vpn-server/pkg/db"
	"vpn-server/pkg/tgbot"
)

func main() {
	dbPath := env("VPN_DB", "/opt/vpn/vpn.db")
	token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN is required")
	}

	store, err := db.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	server, err := store.Server()
	if err != nil {
		log.Fatal(err)
	}
	bot := &tgbot.Bot{Token: token}

	members, err := store.ListMembers()
	if err != nil {
		log.Fatal(err)
	}

	caption := "Обновлённый конфиг ecstasysVPN.\n\n" +
		"Импортируй файл заново в AmneziaWG и переподключи VPN."

	sent := 0
	for _, m := range members {
		tgID, err := store.MemberTelegramUserID(m.ID)
		if err != nil || tgID == 0 {
			log.Printf("skip %s: no telegram id", m.PeerName)
			continue
		}
		peer, err := store.GetPeer(m.PeerName)
		if err != nil {
			log.Printf("skip %s: %v", m.PeerName, err)
			continue
		}
		cfg, err := server.ClientConfig(awg.Peer{
			Name:         peer.Name,
			PrivateKey:   peer.PrivateKey,
			PublicKey:    peer.PublicKey,
			PresharedKey: peer.PresharedKey,
			Address:      peer.Address,
		}, awg.TunnelSmart)
		if err != nil {
			log.Printf("skip %s: config: %v", m.PeerName, err)
			continue
		}
		name := "ecstasysVPN-" + peer.Name + ".conf"
		if err := bot.SendDocument(tgID, name, caption, cfg); err != nil {
			log.Printf("send %s -> %d: %v", m.PeerName, tgID, err)
			continue
		}
		log.Printf("sent %s -> %d", m.PeerName, tgID)
		sent++
	}
	fmt.Printf("done: sent %d configs\n", sent)
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}
