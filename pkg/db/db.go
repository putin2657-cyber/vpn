package db

import (
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
)

// User представляет VPN пользователя
type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Token    string `json:"token"`
	Email    string `json:"email"`
	Created  string `json:"created"`
}

// DB управляет подключением к SQLite
type DB struct {
	conn *sql.DB
}

// Open открывает или создает базу данных
func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	// Сервер, API и admin CLI работают с одним файлом: WAL позволяет читать во
	// время записи, busy_timeout переживает короткие блокировки.
	if _, err := conn.Exec("PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;"); err != nil {
		conn.Close()
		return nil, err
	}
	db := &DB{conn: conn}
	if err := db.init(); err != nil {
		return nil, err
	}
	return db, nil
}

// init создает таблицы если их еще нет
func (db *DB) init() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		token TEXT UNIQUE NOT NULL,
		email TEXT,
		created DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS vpn_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		action TEXT,
		ip_assigned TEXT,
		connected_at DATETIME,
		disconnected_at DATETIME,
		FOREIGN KEY(user_id) REFERENCES users(id)
	);
	`
	_, err := db.conn.Exec(schema)
	return err
}

// AddUser добавляет нового пользователя
func (db *DB) AddUser(username, token, email string) (*User, error) {
	result, err := db.conn.Exec(
		"INSERT INTO users (username, token, email) VALUES (?, ?, ?)",
		username, token, email,
	)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &User{
		ID:       int(id),
		Username: username,
		Token:    token,
		Email:    email,
	}, nil
}

// GetUser получает пользователя по токену
func (db *DB) GetUser(token string) (*User, error) {
	user := &User{}
	err := db.conn.QueryRow(
		"SELECT id, username, token, email, created FROM users WHERE token = ?",
		token,
	).Scan(&user.ID, &user.Username, &user.Token, &user.Email, &user.Created)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, err
	}
	return user, nil
}

// ListUsers возвращает всех пользователей
func (db *DB) ListUsers() ([]*User, error) {
	rows, err := db.conn.Query(
		"SELECT id, username, token, email, created FROM users",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		user := &User{}
		err := rows.Scan(&user.ID, &user.Username, &user.Token, &user.Email, &user.Created)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

// DeleteUser удаляет пользователя по ID
func (db *DB) DeleteUser(id int) error {
	_, err := db.conn.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

// Session — запись о подключении клиента из vpn_logs
type Session struct {
	ID             int64  `json:"id"`
	UserID         int    `json:"user_id"`
	Username       string `json:"username"`
	Action         string `json:"action"`
	IPAssigned     string `json:"ip_assigned"`
	ConnectedAt    string `json:"connected_at"`
	DisconnectedAt string `json:"disconnected_at,omitempty"`
}

// ListSessions возвращает последние записи подключений, начиная с самых свежих
func (db *DB) ListSessions(limit int) ([]*Session, error) {
	rows, err := db.conn.Query(
		`SELECT l.id, l.user_id, COALESCE(u.username, '?'), COALESCE(l.action, ''),
		        COALESCE(l.ip_assigned, ''), COALESCE(l.connected_at, ''),
		        COALESCE(l.disconnected_at, '')
		 FROM vpn_logs l
		 LEFT JOIN users u ON u.id = l.user_id
		 ORDER BY l.id DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		s := &Session{}
		err := rows.Scan(&s.ID, &s.UserID, &s.Username, &s.Action,
			&s.IPAssigned, &s.ConnectedAt, &s.DisconnectedAt)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// StartSession записывает подключение клиента и возвращает ID записи в vpn_logs
func (db *DB) StartSession(userID int, assignedIP string) (int64, error) {
	result, err := db.conn.Exec(
		`INSERT INTO vpn_logs (user_id, action, ip_assigned, connected_at)
		 VALUES (?, 'connect', ?, CURRENT_TIMESTAMP)`,
		userID, assignedIP,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// EndSession проставляет время отключения для ранее открытой сессии
func (db *DB) EndSession(sessionID int64) error {
	_, err := db.conn.Exec(
		`UPDATE vpn_logs SET action = 'disconnect', disconnected_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		sessionID,
	)
	return err
}

// Close закрывает подключение
func (db *DB) Close() error {
	return db.conn.Close()
}
