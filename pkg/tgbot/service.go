// Package tgbot runs the family VPN Telegram bot with Crypto Pay billing.
package tgbot

import (
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"vpn-server/pkg/awg"
	"vpn-server/pkg/billing"
	"vpn-server/pkg/cryptopay"
	"vpn-server/pkg/db"
)

const (
	PriceRUB         = "140.00"
	PriceRubHuman    = "140"
	InvoiceAsset     = "USDT"
	InvoiceExpiresIn = 3600 // 1 hour to pay
	PaymentSBP       = "sbp"
)

var nonPeerChars = regexp.MustCompile(`[^a-z0-9-]+`)

// Config is loaded from the environment on the VPS.
type Config struct {
	DatabasePath string
	SyncCommand  string
	ListenAddr   string
	WebhookPath  string
	BotUsername  string

	// SBP personal transfer (no merchant acquiring).
	SBPPhone      string
	SBPBank       string
	SBPName       string
	AdminTelegram []int64

	// Support and Android app distribution.
	SupportURL    string // https://t.me/...
	AndroidAPKURL string // public HTTPS link
	AndroidAPKPath string // optional local file to send as document
}

// SBPEnabled reports whether personal SBP requisites are configured.
func (c Config) SBPEnabled() bool {
	return strings.TrimSpace(c.SBPPhone) != "" && strings.TrimSpace(c.SBPBank) != ""
}

// IsAdmin reports whether telegramUserID may confirm SBP payments.
func (c Config) IsAdmin(telegramUserID int64) bool {
	for _, id := range c.AdminTelegram {
		if id == telegramUserID {
			return true
		}
	}
	return false
}

// Service links Telegram users to VPN peers and Crypto Pay invoices.
type Service struct {
	DB     *db.DB
	CP     *cryptopay.Client
	Config Config
}

// EnsureMember creates a peer and billing row for a Telegram user if needed.
func (s *Service) EnsureMember(telegramUserID int64, displayName, username string) (*db.Member, bool, error) {
	displayName = strings.TrimSpace(displayName)
	username = strings.TrimSpace(strings.TrimPrefix(username, "@"))
	if displayName == "" {
		displayName = username
	}
	if displayName == "" {
		displayName = fmt.Sprintf("tg-%d", telegramUserID)
	}

	member, err := s.DB.GetMemberByTelegramID(telegramUserID)
	if err == nil {
		_ = s.DB.UpdateMemberTelegramProfile(telegramUserID, displayName, username)
		member.DisplayName = displayName
		member.TelegramUsername = username
		return member, false, nil
	}
	if !errors.Is(err, db.ErrTelegramMemberNotFound) {
		return nil, false, err
	}

	peerName, err := s.allocatePeerName(telegramUserID, username)
	if err != nil {
		return nil, false, err
	}
	if _, err := s.DB.AddPeer(peerName); err != nil {
		return nil, false, err
	}
	if err := s.DB.SetPeerEnabled(peerName, false); err != nil {
		return nil, false, err
	}
	member, err = s.DB.CreateTelegramMember(telegramUserID, displayName, peerName, username)
	if err != nil {
		return nil, false, err
	}
	return member, true, nil
}

func (s *Service) allocatePeerName(telegramUserID int64, username string) (string, error) {
	base := "tg-" + fmt.Sprintf("%d", telegramUserID)
	if u := strings.TrimSpace(strings.ToLower(username)); u != "" {
		u = strings.TrimPrefix(u, "@")
		u = nonPeerChars.ReplaceAllString(u, "")
		u = strings.Trim(u, "-")
		if u != "" && len(u) <= 20 {
			base = "tg-" + u
		}
	}
	candidates := []string{base, base + "-2", base + "-3"}
	for i := 0; i < 5; i++ {
		suffix, err := randomSuffix(4)
		if err != nil {
			return "", err
		}
		candidates = append(candidates, base+"-"+suffix)
	}
	for _, name := range candidates {
		if billing.IsOwnerPeer(name) {
			continue
		}
		if _, err := s.DB.GetPeer(name); err != nil {
			if errors.Is(err, db.ErrPeerNotFound) {
				return name, nil
			}
			return "", err
		}
	}
	return "", fmt.Errorf("could not allocate a free peer name")
}

func randomSuffix(n int) (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i := range b {
		out[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(out), nil
}

// StartCheckout creates a Crypto Pay invoice for one month of VPN access.
func (s *Service) StartCheckout(member *db.Member) (*cryptopay.Invoice, error) {
	kind := db.PaymentInitial
	if member.Status == db.MemberActive || member.PaidUntil != "" {
		kind = db.PaymentRenewal
	}
	payload := fmt.Sprintf("member:%d:%s:%d", member.ID, kind, time.Now().Unix()/60)
	invoice, err := s.CP.CreateInvoice(cryptopay.CreateInvoiceParams{
		CurrencyType:   "fiat",
		Fiat:           "RUB",
		Amount:         PriceRUB,
		AcceptedAssets: InvoiceAsset,
		Description:    fmt.Sprintf("VPN — %s, 1 месяц", member.DisplayName),
		HiddenMessage:  "Спасибо! Вернитесь в бот и нажмите «Мой конфиг».",
		Payload:        payload,
		ExpiresIn:      InvoiceExpiresIn,
	})
	if err != nil {
		return nil, err
	}
	ref := fmt.Sprintf("cp:%d", invoice.InvoiceID)
	if err := s.DB.RecordPayment(member.ID, ref, PriceRUB, invoice.Status, kind); err != nil {
		return nil, err
	}
	return invoice, nil
}

// HandlePaidInvoice activates or extends a subscription after Crypto Pay webhook.
func (s *Service) HandlePaidInvoice(invoice cryptopay.Invoice) error {
	if invoice.Status != "paid" {
		return nil
	}
	ref := fmt.Sprintf("cp:%d", invoice.InvoiceID)
	stored, err := s.DB.GetPayment(ref)
	if err != nil {
		return fmt.Errorf("unknown invoice cp:%d: %w", invoice.InvoiceID, err)
	}
	if err := s.DB.RecordPayment(stored.MemberID, ref, PriceRUB, invoice.Status, stored.Kind); err != nil {
		return err
	}
	_, err = s.ActivateMember(stored.MemberID)
	return err
}

// SBPCheckout is a pending personal SBP transfer.
type SBPCheckout struct {
	Code   string
	Kind   string
	Amount string
}

// StartSBPCheckout creates a pending SBP payment with a unique comment code.
func (s *Service) StartSBPCheckout(member *db.Member) (*SBPCheckout, error) {
	if !s.Config.SBPEnabled() {
		return nil, fmt.Errorf("СБП не настроена (нужны SBP_PHONE и SBP_BANK)")
	}
	kind := db.PaymentInitial
	if member.Status == db.MemberActive || member.PaidUntil != "" {
		kind = db.PaymentRenewal
	}
	code, err := randomSuffix(4)
	if err != nil {
		return nil, err
	}
	code = strings.ToUpper(code)
	ref := "sbp:" + code
	if err := s.DB.RecordPayment(member.ID, ref, PriceRUB, "pending", kind); err != nil {
		return nil, err
	}
	return &SBPCheckout{Code: code, Kind: kind, Amount: PriceRUB}, nil
}

// MarkSBPReported marks that the user claimed they transferred money.
func (s *Service) MarkSBPReported(code string) (*db.Payment, *db.Member, error) {
	ref := "sbp:" + strings.ToUpper(strings.TrimSpace(code))
	payment, err := s.DB.GetPayment(ref)
	if err != nil {
		return nil, nil, fmt.Errorf("счёт %s не найден", code)
	}
	if payment.Status == "paid" {
		member, err := s.DB.GetMemberByID(payment.MemberID)
		return payment, member, err
	}
	if err := s.DB.RecordPayment(payment.MemberID, ref, payment.AmountValue, "reported", payment.Kind); err != nil {
		return nil, nil, err
	}
	payment.Status = "reported"
	member, err := s.DB.GetMemberByID(payment.MemberID)
	return payment, member, err
}

// ConfirmSBPPayment activates the member after an admin verifies the transfer.
func (s *Service) ConfirmSBPPayment(code string) (*db.Member, error) {
	ref := "sbp:" + strings.ToUpper(strings.TrimSpace(code))
	payment, err := s.DB.GetPayment(ref)
	if err != nil {
		return nil, fmt.Errorf("счёт %s не найден", code)
	}
	if payment.Status == "paid" {
		return s.DB.GetMemberByID(payment.MemberID)
	}
	if err := s.DB.RecordPayment(payment.MemberID, ref, payment.AmountValue, "paid", payment.Kind); err != nil {
		return nil, err
	}
	return s.ActivateMember(payment.MemberID)
}

// RejectSBPPayment marks an SBP invoice as rejected.
func (s *Service) RejectSBPPayment(code string) error {
	ref := "sbp:" + strings.ToUpper(strings.TrimSpace(code))
	payment, err := s.DB.GetPayment(ref)
	if err != nil {
		return fmt.Errorf("счёт %s не найден", code)
	}
	if payment.Status == "paid" {
		return fmt.Errorf("счёт %s уже подтверждён", code)
	}
	return s.DB.RecordPayment(payment.MemberID, ref, payment.AmountValue, "rejected", payment.Kind)
}

// ActivateMember extends paid_until, enables the peer and syncs awg.
func (s *Service) ActivateMember(memberID int) (*db.Member, error) {
	member, err := s.DB.GetMemberByID(memberID)
	if err != nil {
		return nil, err
	}
	paidUntil := db.ExtendPaidUntil(member.PaidUntil, time.Now(), billing.Period)
	if err := s.DB.UpdateMemberStatus(member.ID, db.MemberActive, paidUntil); err != nil {
		return nil, err
	}
	if err := s.DB.SetPeerEnabled(member.PeerName, true); err != nil {
		return nil, err
	}
	if err := s.syncPeers(); err != nil {
		return nil, err
	}
	return s.DB.GetMemberByID(memberID)
}

// SBPInstructions formats transfer details for the payer.
func (s *Service) SBPInstructions(code string) string {
	nameLine := ""
	if n := strings.TrimSpace(s.Config.SBPName); n != "" {
		nameLine = fmt.Sprintf("Получатель: <b>%s</b>\n", escapeHTMLPlain(n))
	}
	return fmt.Sprintf(
		"<b>%s · оплата СБП</b>\n────────────\n"+
			"Сумма: <b>%s ₽</b>\n"+
			"%sТелефон: <code>%s</code>\n"+
			"Банк: <b>%s</b>\n"+
			"Комментарий: <code>VPN-%s</code>\n\n"+
			"1. Банк → СБП → перевод по телефону\n"+
			"2. Обязательно укажите комментарий <code>VPN-%s</code>\n"+
			"3. Нажмите «Я оплатил» — подтверждение придёт админу",
		BrandName, PriceRubHuman, nameLine, escapeHTMLPlain(s.Config.SBPPhone),
		escapeHTMLPlain(s.Config.SBPBank), code, code,
	)
}

func escapeHTMLPlain(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// ClientConfig returns the AmneziaWG config for an active Telegram subscriber.
func (s *Service) ClientConfig(member *db.Member) (string, error) {
	if member.Status != db.MemberActive {
		return "", fmt.Errorf("подписка не активна — сначала оплатите %s ₽", PriceRUB)
	}
	peer, err := s.DB.GetPeer(member.PeerName)
	if err != nil {
		return "", err
	}
	if !peer.Enabled {
		return "", fmt.Errorf("VPN отключён — оплатите подписку")
	}
	server, err := s.DB.Server()
	if err != nil {
		return "", err
	}
	return server.ClientConfig(awg.Peer{
		Name:         peer.Name,
		PrivateKey:   peer.PrivateKey,
		PublicKey:    peer.PublicKey,
		PresharedKey: peer.PresharedKey,
		Address:      peer.Address,
	}, awg.TunnelSmart)
}

// StatusText formats subscription state for chat messages.
func StatusText(member *db.Member) string {
	switch member.Status {
	case db.MemberActive:
		if member.PaidUntil != "" {
			return fmt.Sprintf("Подписка: <b>активна</b>\nДо: <b>%s</b>", formatLocal(member.PaidUntil))
		}
		return "Подписка: <b>активна</b>"
	case db.MemberPastDue:
		return fmt.Sprintf("Подписка: <b>просрочена</b>\nБыла до: %s\nОплатите, чтобы снова включить VPN.", formatLocal(member.PaidUntil))
	default:
		return fmt.Sprintf("Подписка: <b>не активна</b>\nСтоимость: <b>%s ₽</b> / месяц", PriceRubHuman)
	}
}

func formatLocal(rfc3339 string) string {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return rfc3339
	}
	return t.Local().Format("02.01.2006 15:04")
}

// RunMaintenance sends renewal reminders and disables unpaid peers.
func (s *Service) RunMaintenance(notify func(telegramUserID int64, text string) error) error {
	now := time.Now().UTC()

	expiring, err := s.DB.MembersExpiringWithin(now, 3*24*time.Hour)
	if err != nil {
		return err
	}
	for _, member := range expiring {
		tgID, err := s.telegramID(member)
		if err != nil {
			continue
		}
		text := fmt.Sprintf(
			"Напоминание: VPN для %s истекает %s.\nНажмите «Оплатить», чтобы продлить на месяц (%s ₽ в USDT).",
			member.DisplayName, formatLocal(member.PaidUntil), PriceRUB,
		)
		if err := notify(tgID, text); err != nil {
			log.Printf("remind tg %d: %v", tgID, err)
		}
	}

	past, err := s.DB.MembersPastGrace(now, billing.Grace)
	if err != nil {
		return err
	}
	changed := false
	for _, member := range past {
		if member.Status == db.MemberCanceled {
			continue
		}
		if billing.IsOwnerPeer(member.PeerName) {
			continue
		}
		peer, err := s.DB.GetPeer(member.PeerName)
		if err != nil {
			continue
		}
		if !peer.Enabled {
			continue
		}
		log.Printf("disabling unpaid telegram peer %s (%s)", member.PeerName, member.DisplayName)
		if err := s.DB.SetPeerEnabled(member.PeerName, false); err != nil {
			log.Printf("disable peer: %v", err)
			continue
		}
		_ = s.DB.UpdateMemberStatus(member.ID, db.MemberPastDue, member.PaidUntil)
		changed = true
		if tgID, err := s.telegramID(member); err == nil {
			_ = notify(tgID, "VPN отключён из‑за неоплаты. Нажмите «Оплатить», чтобы снова включить доступ.")
		}
	}
	if changed {
		return s.syncPeers()
	}
	return nil
}

func (s *Service) telegramID(member *db.Member) (int64, error) {
	return s.DB.MemberTelegramUserID(member.ID)
}

// SetPeerAccess enables or disables a VPN peer and syncs awg.
// Owner peers are protected. Linked members get status updated to match access.
func (s *Service) SetPeerAccess(peerName string, enabled bool) (*db.Peer, *db.Member, error) {
	peerName = strings.TrimSpace(peerName)
	if peerName == "" {
		return nil, nil, fmt.Errorf("пустой peer")
	}
	if billing.IsOwnerPeer(peerName) {
		return nil, nil, fmt.Errorf("свои (owner) слоты здесь не трогаем")
	}
	peer, err := s.DB.GetPeer(peerName)
	if err != nil {
		return nil, nil, err
	}
	if err := s.DB.SetPeerEnabled(peerName, enabled); err != nil {
		return nil, nil, err
	}
	peer.Enabled = enabled

	var member *db.Member
	if m, err := s.DB.GetMemberByPeerName(peerName); err == nil {
		member = m
		if enabled {
			if member.Status != db.MemberActive {
				_ = s.DB.UpdateMemberStatus(member.ID, db.MemberActive, member.PaidUntil)
				member.Status = db.MemberActive
			}
		} else if member.Status == db.MemberActive {
			_ = s.DB.UpdateMemberStatus(member.ID, db.MemberPastDue, member.PaidUntil)
			member.Status = db.MemberPastDue
		}
	}
	if err := s.syncPeers(); err != nil {
		return peer, member, err
	}
	return peer, member, nil
}

func (s *Service) syncPeers() error {
	cmd := strings.TrimSpace(s.Config.SyncCommand)
	if cmd == "" {
		return nil
	}
	log.Printf("sync peers: %s", cmd)
	c := exec.Command("bash", "-lc", cmd)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Env = os.Environ()
	return c.Run()
}
