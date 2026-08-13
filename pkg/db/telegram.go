package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrTelegramMemberNotFound means no member is linked to this Telegram user.
var ErrTelegramMemberNotFound = errors.New("telegram member not found")

func (db *DB) ensureTelegramSchema() error {
	if err := db.ensureColumn("members", "telegram_user_id", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := db.ensureColumn("members", "telegram_username", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	_, err := db.conn.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_members_telegram_user_id
		ON members(telegram_user_id) WHERE telegram_user_id != 0`)
	return err
}

// GetMemberByTelegramID returns the member linked to a Telegram user id.
func (db *DB) GetMemberByTelegramID(userID int64) (*Member, error) {
	if userID <= 0 {
		return nil, ErrTelegramMemberNotFound
	}
	row := db.conn.QueryRow(`
		SELECT id, user_id, display_name, peer_name, token, payment_method_id, status, paid_until, created, telegram_username
		FROM members WHERE telegram_user_id = ?`, userID)
	member, err := scanMember(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTelegramMemberNotFound
	}
	return member, err
}

// CreateTelegramMember registers a Telegram user with a fresh peer and invite token.
func (db *DB) CreateTelegramMember(telegramUserID int64, displayName, peerName, username string) (*Member, error) {
	displayName = strings.TrimSpace(displayName)
	peerName = strings.TrimSpace(peerName)
	username = normalizeTelegramUsername(username)
	if telegramUserID <= 0 || displayName == "" || peerName == "" {
		return nil, fmt.Errorf("telegram user, display name and peer name are required")
	}
	if _, err := db.GetPeer(peerName); err != nil {
		return nil, err
	}
	token, err := randomToken(16)
	if err != nil {
		return nil, err
	}
	result, err := db.conn.Exec(`
		INSERT INTO members (display_name, peer_name, token, status, user_id, telegram_user_id, telegram_username)
		VALUES (?, ?, ?, ?, 0, ?, ?)`,
		displayName, peerName, token, MemberInvited, telegramUserID, username,
	)
	if err != nil {
		return nil, fmt.Errorf("create telegram member: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return db.GetMemberByID(int(id))
}

// UpdateMemberTelegramProfile refreshes display name and @username for a Telegram member.
func (db *DB) UpdateMemberTelegramProfile(telegramUserID int64, displayName, username string) error {
	displayName = strings.TrimSpace(displayName)
	username = normalizeTelegramUsername(username)
	_, err := db.conn.Exec(`
		UPDATE members SET display_name = CASE WHEN ? != '' THEN ? ELSE display_name END,
		                    telegram_username = ?
		WHERE telegram_user_id = ?`,
		displayName, displayName, username, telegramUserID,
	)
	return err
}

func normalizeTelegramUsername(username string) string {
	username = strings.TrimSpace(username)
	username = strings.TrimPrefix(username, "@")
	return username
}

// MemberTelegramUserID returns the linked Telegram user id for a member row.
func (db *DB) MemberTelegramUserID(memberID int) (int64, error) {
	var id int64
	err := db.conn.QueryRow(`SELECT telegram_user_id FROM members WHERE id = ?`, memberID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrMemberNotFound
	}
	if id <= 0 {
		return 0, ErrTelegramMemberNotFound
	}
	return id, err
}
func (db *DB) MembersExpiringWithin(now time.Time, within time.Duration) ([]*Member, error) {
	start := now.UTC().Format(time.RFC3339)
	end := now.Add(within).UTC().Format(time.RFC3339)
	rows, err := db.conn.Query(`
		SELECT id, user_id, display_name, peer_name, token, payment_method_id, status, paid_until, created, telegram_username
		FROM members
		WHERE telegram_user_id != 0
		  AND status IN (?, ?)
		  AND paid_until != ''
		  AND paid_until > ?
		  AND paid_until <= ?`,
		MemberActive, MemberPastDue, start, end,
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
