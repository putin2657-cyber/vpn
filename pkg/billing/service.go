// Package billing implements family subscriptions at a fixed monthly price.
package billing

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"vpn-server/pkg/db"
	"vpn-server/pkg/yookassa"
)

const (
	// PriceRUB is the symbolic family contribution per month.
	PriceRUB = "120.00"
	// Period is one calendar month of access after a successful charge.
	Period = 30 * 24 * time.Hour
	// Grace is how long access stays up after paid_until before the peer is cut.
	Grace = 3 * 24 * time.Hour
	// RenewAhead starts charging this long before expiry.
	RenewAhead = 24 * time.Hour
)

// OwnerPeers are the admin's own devices. They stay free of billing forever:
// no invite, no auto-disable for non-payment.
var OwnerPeers = map[string]struct{}{
	"my-phone":  {}, // 1st
	"my-second": {}, // 2nd
	"device-03": {}, // 3rd
	"device-06": {}, // 6th
}

// IsOwnerPeer reports whether this peer is reserved for the admin.
func IsOwnerPeer(name string) bool {
	_, ok := OwnerPeers[strings.TrimSpace(name)]
	return ok
}

// Config is loaded from the environment on the VPS.
type Config struct {
	DatabasePath string
	PublicBase   string // https://pay.example.com — no trailing slash
	AdminToken   string
	ShopID       string
	SecretKey    string
	SyncCommand  string // optional shell command after enable/disable changes
	ListenAddr   string
}

// Service owns subscription state and YooKassa calls.
type Service struct {
	DB     *db.DB
	YK     *yookassa.Client
	Config Config
}

func (s *Service) Invite(displayName, peerName string) (*db.Member, string, error) {
	peerName = strings.TrimSpace(peerName)
	if IsOwnerPeer(peerName) {
		return nil, "", fmt.Errorf("peer %q is reserved for the admin and cannot be billed", peerName)
	}
	member, err := s.DB.CreateMember(displayName, peerName)
	if err != nil {
		return nil, "", err
	}
	return member, s.Config.PublicBase + "/i/" + member.Token, nil
}

func (s *Service) StartCheckout(token string) (confirmURL string, err error) {
	member, err := s.DB.GetMemberByToken(token)
	if err != nil {
		return "", err
	}
	idem := fmt.Sprintf("initial-%d-%d", member.ID, time.Now().Unix()/60)
	returnURL := s.Config.PublicBase + "/i/" + member.Token + "/done"
	payment, err := s.YK.CreateRedirectPayment(
		idem,
		PriceRUB,
		fmt.Sprintf("VPN для %s — 1 месяц", member.DisplayName),
		returnURL,
		map[string]string{
			"member_id": fmt.Sprintf("%d", member.ID),
			"kind":      db.PaymentInitial,
		},
	)
	if err != nil {
		return "", err
	}
	if err := s.DB.RecordPayment(member.ID, payment.ID, PriceRUB, payment.Status, db.PaymentInitial); err != nil {
		return "", err
	}
	if payment.Confirmation == nil || payment.Confirmation.ConfirmationURL == "" {
		return "", fmt.Errorf("yookassa did not return a confirmation URL")
	}
	return payment.Confirmation.ConfirmationURL, nil
}

// HandleYooKassaNotification re-fetches the payment and applies success/failure.
func (s *Service) HandleYooKassaNotification(paymentID string) error {
	payment, err := s.YK.GetPayment(paymentID)
	if err != nil {
		return err
	}
	kind := payment.Metadata["kind"]
	if kind == "" {
		kind = db.PaymentInitial
	}
	memberID := 0
	fmt.Sscanf(payment.Metadata["member_id"], "%d", &memberID)
	if memberID == 0 {
		stored, err := s.DB.GetPayment(payment.ID)
		if err != nil {
			return fmt.Errorf("payment %s has no member metadata", payment.ID)
		}
		memberID = stored.MemberID
	}
	if err := s.DB.RecordPayment(memberID, payment.ID, payment.Amount.Value, payment.Status, kind); err != nil {
		return err
	}
	if payment.Status != "succeeded" || !payment.Paid {
		return nil
	}
	return s.applySuccessfulPayment(memberID, payment)
}

func (s *Service) applySuccessfulPayment(memberID int, payment *yookassa.Payment) error {
	member, err := s.DB.GetMemberByID(memberID)
	if err != nil {
		return err
	}
	if payment.PaymentMethod != nil && payment.PaymentMethod.ID != "" {
		if err := s.DB.UpdateMemberPaymentMethod(member.ID, payment.PaymentMethod.ID); err != nil {
			return err
		}
	}
	paidUntil := db.ExtendPaidUntil(member.PaidUntil, time.Now(), Period)
	if err := s.DB.UpdateMemberStatus(member.ID, db.MemberActive, paidUntil); err != nil {
		return err
	}
	if err := s.DB.SetPeerEnabled(member.PeerName, true); err != nil {
		return err
	}
	return s.syncPeers()
}

// RunMaintenance renews due cards and disables peers past grace.
func (s *Service) RunMaintenance() error {
	now := time.Now().UTC()
	due, err := s.DB.MembersDueForRenewal(now, RenewAhead)
	if err != nil {
		return err
	}
	for _, member := range due {
		if err := s.renew(member); err != nil {
			log.Printf("renew member %d (%s): %v", member.ID, member.DisplayName, err)
			_ = s.DB.UpdateMemberStatus(member.ID, db.MemberPastDue, member.PaidUntil)
		}
	}

	past, err := s.DB.MembersPastGrace(now, Grace)
	if err != nil {
		return err
	}
	changed := false
	for _, member := range past {
		if member.Status == db.MemberCanceled {
			continue
		}
		if IsOwnerPeer(member.PeerName) {
			continue
		}
		peer, err := s.DB.GetPeer(member.PeerName)
		if err != nil {
			continue
		}
		if peer.Enabled {
			log.Printf("disabling unpaid peer %s (%s)", member.PeerName, member.DisplayName)
			if err := s.DB.SetPeerEnabled(member.PeerName, false); err != nil {
				log.Printf("disable peer: %v", err)
				continue
			}
			_ = s.DB.UpdateMemberStatus(member.ID, db.MemberPastDue, member.PaidUntil)
			changed = true
		}
	}
	if changed {
		return s.syncPeers()
	}
	return nil
}

func (s *Service) renew(member *db.Member) error {
	idem := fmt.Sprintf("renew-%d-%s", member.ID, strings.ReplaceAll(member.PaidUntil, ":", ""))
	payment, err := s.YK.ChargeSavedMethod(
		idem,
		PriceRUB,
		fmt.Sprintf("VPN автопродление — %s", member.DisplayName),
		member.PaymentMethodID,
		map[string]string{
			"member_id": fmt.Sprintf("%d", member.ID),
			"kind":      db.PaymentRenewal,
		},
	)
	if err != nil {
		return err
	}
	if err := s.DB.RecordPayment(member.ID, payment.ID, PriceRUB, payment.Status, db.PaymentRenewal); err != nil {
		return err
	}
	if payment.Status == "succeeded" && payment.Paid {
		return s.applySuccessfulPayment(member.ID, payment)
	}
	return fmt.Errorf("renewal status %s", payment.Status)
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
