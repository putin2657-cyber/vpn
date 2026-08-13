package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Member statuses used by the family billing portal.
const (
	MemberInvited  = "invited"
	MemberActive   = "active"
	MemberPastDue  = "past_due"
	MemberCanceled = "canceled"
)

// Payment kinds.
const (
	PaymentInitial = "initial"
	PaymentRenewal = "renewal"
)

// ErrMemberNotFound is returned when an invite token or id does not match.
var ErrMemberNotFound = errors.New("member not found")

// Member is one paying family user linked to a VPN peer.
type Member struct {
	ID               int
	UserID           int
	DisplayName      string
	PeerName         string
	Token            string
	PaymentMethodID  string
	Status           string
	PaidUntil        string
	Created          string
	TelegramUsername string
}

// Payment is one YooKassa charge attempt.
type Payment struct {
	ID          int
	MemberID    int
	YKPaymentID string
	AmountValue string
	Status      string
	Kind        string
	Created     string
}

// CreateMember links a display name to an existing peer and issues an invite token.
func (db *DB) CreateMember(displayName, peerName string) (*Member, error) {
	displayName = strings.TrimSpace(displayName)
	peerName = strings.TrimSpace(peerName)
	if displayName == "" || peerName == "" {
		return nil, fmt.Errorf("display name and peer name are required")
	}
	if _, err := db.GetPeer(peerName); err != nil {
		return nil, err
	}

	token, err := randomToken(16)
	if err != nil {
		return nil, err
	}

	result, err := db.conn.Exec(`
		INSERT INTO members (display_name, peer_name, token, status, user_id)
		VALUES (?, ?, ?, ?, 0)`,
		displayName, peerName, token, MemberInvited,
	)
	if err != nil {
		return nil, fmt.Errorf("create member: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return db.GetMemberByID(int(id))
}

// CreateMemberForUser links a self-registered account to a peer.
func (db *DB) CreateMemberForUser(userID int, displayName, peerName string) (*Member, error) {
	displayName = strings.TrimSpace(displayName)
	peerName = strings.TrimSpace(peerName)
	if userID <= 0 || displayName == "" || peerName == "" {
		return nil, fmt.Errorf("user, display name and peer name are required")
	}
	if _, err := db.GetPeer(peerName); err != nil {
		return nil, err
	}
	token, err := randomToken(16)
	if err != nil {
		return nil, err
	}
	result, err := db.conn.Exec(`
		INSERT INTO members (display_name, peer_name, token, status, user_id)
		VALUES (?, ?, ?, ?, ?)`,
		displayName, peerName, token, MemberInvited, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("create member: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return db.GetMemberByID(int(id))
}

func (db *DB) GetMemberByID(id int) (*Member, error) {
	row := db.conn.QueryRow(`
		SELECT id, user_id, display_name, peer_name, token, payment_method_id, status, paid_until, created, telegram_username
		FROM members WHERE id = ?`, id)
	member, err := scanMember(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMemberNotFound
	}
	return member, err
}

func (db *DB) GetMemberByToken(token string) (*Member, error) {
	row := db.conn.QueryRow(`
		SELECT id, user_id, display_name, peer_name, token, payment_method_id, status, paid_until, created, telegram_username
		FROM members WHERE token = ?`, token)
	member, err := scanMember(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMemberNotFound
	}
	return member, err
}

func (db *DB) GetMemberByUserID(userID int) (*Member, error) {
	row := db.conn.QueryRow(`
		SELECT id, user_id, display_name, peer_name, token, payment_method_id, status, paid_until, created, telegram_username
		FROM members WHERE user_id = ? ORDER BY id LIMIT 1`, userID)
	member, err := scanMember(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMemberNotFound
	}
	return member, err
}

// GetMemberByPeerName returns the billing member linked to a VPN peer, if any.
func (db *DB) GetMemberByPeerName(peerName string) (*Member, error) {
	peerName = strings.TrimSpace(peerName)
	if peerName == "" {
		return nil, ErrMemberNotFound
	}
	row := db.conn.QueryRow(`
		SELECT id, user_id, display_name, peer_name, token, payment_method_id, status, paid_until, created, telegram_username
		FROM members WHERE peer_name = ? ORDER BY id LIMIT 1`, peerName)
	member, err := scanMember(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMemberNotFound
	}
	return member, err
}

func (db *DB) ListMembers() ([]*Member, error) {
	rows, err := db.conn.Query(`
		SELECT id, user_id, display_name, peer_name, token, payment_method_id, status, paid_until, created, telegram_username
		FROM members ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []*Member
	for rows.Next() {
		member, err := scanMember(rows)
		if err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

func (db *DB) UpdateMemberPaymentMethod(id int, methodID string) error {
	_, err := db.conn.Exec(`UPDATE members SET payment_method_id = ? WHERE id = ?`, methodID, id)
	return err
}

func (db *DB) UpdateMemberStatus(id int, status, paidUntil string) error {
	_, err := db.conn.Exec(`UPDATE members SET status = ?, paid_until = ? WHERE id = ?`, status, paidUntil, id)
	return err
}

func (db *DB) RecordPayment(memberID int, ykPaymentID, amountValue, status, kind string) error {
	_, err := db.conn.Exec(`
		INSERT INTO payments (member_id, yk_payment_id, amount_value, status, kind)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(yk_payment_id) DO UPDATE SET status = excluded.status`,
		memberID, ykPaymentID, amountValue, status, kind,
	)
	return err
}

func (db *DB) GetPayment(ykPaymentID string) (*Payment, error) {
	row := db.conn.QueryRow(`
		SELECT id, member_id, yk_payment_id, amount_value, status, kind, created
		FROM payments WHERE yk_payment_id = ?`, ykPaymentID)
	var p Payment
	err := row.Scan(&p.ID, &p.MemberID, &p.YKPaymentID, &p.AmountValue, &p.Status, &p.Kind, &p.Created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("payment not found")
	}
	return &p, err
}

// MembersDueForRenewal returns active members whose paid_until is within the window.
func (db *DB) MembersDueForRenewal(now time.Time, ahead time.Duration) ([]*Member, error) {
	cutoff := now.Add(ahead).UTC().Format(time.RFC3339)
	rows, err := db.conn.Query(`
		SELECT id, user_id, display_name, peer_name, token, payment_method_id, status, paid_until, created, telegram_username
		FROM members
		WHERE status IN (?, ?)
		  AND payment_method_id != ''
		  AND paid_until != ''
		  AND paid_until <= ?`,
		MemberActive, MemberPastDue, cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []*Member
	for rows.Next() {
		member, err := scanMember(rows)
		if err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

// MembersPastGrace returns members whose paid_until is older than grace.
func (db *DB) MembersPastGrace(now time.Time, grace time.Duration) ([]*Member, error) {
	cutoff := now.Add(-grace).UTC().Format(time.RFC3339)
	rows, err := db.conn.Query(`
		SELECT id, user_id, display_name, peer_name, token, payment_method_id, status, paid_until, created, telegram_username
		FROM members
		WHERE status != ?
		  AND paid_until != ''
		  AND paid_until < ?`,
		MemberCanceled, cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []*Member
	for rows.Next() {
		member, err := scanMember(rows)
		if err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

func scanMember(src scanner) (*Member, error) {
	var m Member
	err := src.Scan(&m.ID, &m.UserID, &m.DisplayName, &m.PeerName, &m.Token, &m.PaymentMethodID, &m.Status, &m.PaidUntil, &m.Created, &m.TelegramUsername)
	return &m, err
}

func randomToken(n int) (string, error) {
	raw := make([]byte, n)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

// ExtendPaidUntil adds one billing period from the later of now or current paid_until.
func ExtendPaidUntil(current string, now time.Time, period time.Duration) string {
	start := now.UTC()
	if current != "" {
		if parsed, err := time.Parse(time.RFC3339, current); err == nil && parsed.After(start) {
			start = parsed
		}
	}
	return start.Add(period).Format(time.RFC3339)
}
