package db

import (
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ErrUserExists means this email is already registered.
var ErrUserExists = errors.New("user already exists")

// ErrInvalidCredentials is returned for bad email/password pairs.
var ErrInvalidCredentials = errors.New("invalid email or password")

// ErrSessionNotFound means the cookie/token is missing or expired.
var ErrSessionNotFound = errors.New("session not found")

// User is a self-registered cabinet account.
type User struct {
	ID           int
	Email        string
	PasswordHash string
	DisplayName  string
	Created      string
}

// Session is a browser login cookie.
type Session struct {
	Token   string
	UserID  int
	Expires string
}

func (db *DB) ensureAuthSchema() error {
	_, err := db.conn.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			email         TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			display_name  TEXT NOT NULL,
			created       DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS sessions (
			token   TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			expires TEXT NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		);
	`)
	if err != nil {
		return err
	}
	return db.ensureColumn("members", "user_id", "INTEGER NOT NULL DEFAULT 0")
}

func (db *DB) ensureColumn(table, column, decl string) error {
	rows, err := db.conn.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == column {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = db.conn.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, decl))
	return err
}

// CreateUser registers an email/password account.
func (db *DB) CreateUser(email, password, displayName string) (*User, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return nil, err
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = strings.Split(email, "@")[0]
	}
	if len(password) < 8 {
		return nil, fmt.Errorf("password must be at least 8 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	result, err := db.conn.Exec(`
		INSERT INTO users (email, password_hash, display_name) VALUES (?, ?, ?)`,
		email, string(hash), displayName,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, ErrUserExists
		}
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return db.GetUserByID(int(id))
}

func (db *DB) GetUserByID(id int) (*User, error) {
	row := db.conn.QueryRow(`
		SELECT id, email, password_hash, display_name, created FROM users WHERE id = ?`, id)
	return scanUser(row)
}

func (db *DB) GetUserByEmail(email string) (*User, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return nil, err
	}
	row := db.conn.QueryRow(`
		SELECT id, email, password_hash, display_name, created FROM users WHERE email = ?`, email)
	user, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}
	return user, err
}

// AuthenticateUser checks email/password.
func (db *DB) AuthenticateUser(email, password string) (*User, error) {
	user, err := db.GetUserByEmail(email)
	if err != nil {
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	return user, nil
}

// CreateSession issues a new login token valid for the given duration.
func (db *DB) CreateSession(userID int, ttl time.Duration) (*Session, error) {
	token, err := randomToken(24)
	if err != nil {
		return nil, err
	}
	expires := time.Now().UTC().Add(ttl).Format(time.RFC3339)
	if _, err := db.conn.Exec(`INSERT INTO sessions (token, user_id, expires) VALUES (?, ?, ?)`,
		token, userID, expires); err != nil {
		return nil, err
	}
	return &Session{Token: token, UserID: userID, Expires: expires}, nil
}

func (db *DB) UserBySession(token string) (*User, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrSessionNotFound
	}
	row := db.conn.QueryRow(`SELECT user_id, expires FROM sessions WHERE token = ?`, token)
	var userID int
	var expires string
	if err := row.Scan(&userID, &expires); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}
	exp, err := time.Parse(time.RFC3339, expires)
	if err != nil || time.Now().UTC().After(exp) {
		_, _ = db.conn.Exec(`DELETE FROM sessions WHERE token = ?`, token)
		return nil, ErrSessionNotFound
	}
	return db.GetUserByID(userID)
}

func (db *DB) DeleteSession(token string) error {
	_, err := db.conn.Exec(`DELETE FROM sessions WHERE token = ?`, token)
	return err
}

func scanUser(src scanner) (*User, error) {
	var u User
	err := src.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.Created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return &u, err
}

func normalizeEmail(email string) (string, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address == "" {
		return "", fmt.Errorf("invalid email address")
	}
	return strings.ToLower(addr.Address), nil
}
