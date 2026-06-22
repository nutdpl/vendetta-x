// Package mail is Vendetta/X's private user-to-user mail (Email). It owns its
// own SQLite table over the shared database handle, so it is fully isolated and
// testable on its own. This file is the CONTRACT; New creates the table and the
// methods are stubs until implemented.
package mail

import (
	"database/sql"
	"time"

	"vendetta-x/server/internal/sanitize"
)

// Message is one piece of private mail.
type Message struct {
	ID            int64
	From, To      string
	Subject, Body string
	Sent          time.Time
	Read          bool
}

// Store is the mail data layer over the shared *sql.DB.
type Store struct {
	db *sql.DB
}

// New returns a Store, creating the mail table if absent.
func New(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

// migrate creates the mail table (and its to_who index) if absent.
func (s *Store) migrate() error {
	const schema = `CREATE TABLE IF NOT EXISTS mail (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		from_who TEXT NOT NULL,
		to_who TEXT NOT NULL,
		subject TEXT NOT NULL DEFAULT '',
		body TEXT NOT NULL DEFAULT '',
		sent INTEGER NOT NULL DEFAULT 0,
		read INTEGER NOT NULL DEFAULT 0
	);`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_mail_to ON mail(to_who)`); err != nil {
		return err
	}
	return nil
}

// Send delivers a message from -> to. Times stamp to now; read starts false.
func (s *Store) Send(from, to, subject, body string) error {
	from, to = sanitize.Line(from), sanitize.Line(to)
	subject, body = sanitize.Line(subject), sanitize.Text(body)
	_, err := s.db.Exec(
		`INSERT INTO mail (from_who, to_who, subject, body, sent, read) VALUES (?, ?, ?, ?, ?, 0)`,
		from, to, subject, body, time.Now().Unix())
	return err
}

// Inbox returns messages addressed to handle, newest first (case-insensitive).
func (s *Store) Inbox(handle string) ([]Message, error) {
	return s.query(
		`SELECT id, from_who, to_who, subject, body, sent, read FROM mail
		 WHERE to_who = ? COLLATE NOCASE ORDER BY sent DESC, id DESC`, handle)
}

// Outbox returns messages sent by handle, newest first (case-insensitive).
func (s *Store) Outbox(handle string) ([]Message, error) {
	return s.query(
		`SELECT id, from_who, to_who, subject, body, sent, read FROM mail
		 WHERE from_who = ? COLLATE NOCASE ORDER BY sent DESC, id DESC`, handle)
}

// Get returns one message by id, or nil,nil if not found.
func (s *Store) Get(id int64) (*Message, error) {
	row := s.db.QueryRow(
		`SELECT id, from_who, to_who, subject, body, sent, read FROM mail WHERE id = ?`, id)
	m, err := scan(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

// MarkRead flags a message read.
func (s *Store) MarkRead(id int64) error {
	_, err := s.db.Exec(`UPDATE mail SET read = 1 WHERE id = ?`, id)
	return err
}

// Delete removes a message, but only if handle is its sender or recipient
// (case-insensitive).
func (s *Store) Delete(id int64, handle string) error {
	_, err := s.db.Exec(
		`DELETE FROM mail WHERE id = ?
		 AND (from_who = ? COLLATE NOCASE OR to_who = ? COLLATE NOCASE)`,
		id, handle, handle)
	return err
}

// UnreadCount returns how many unread messages handle has.
func (s *Store) UnreadCount(handle string) (int, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM mail WHERE to_who = ? COLLATE NOCASE AND read = 0`, handle).Scan(&n)
	return n, err
}

// query runs a SELECT returning the standard column set and decodes the rows.
func (s *Store) query(sqlStr string, args ...interface{}) ([]Message, error) {
	rows, err := s.db.Query(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		m, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

// scanner is the shared interface of *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...interface{}) error
}

// scan decodes one row (id, from, to, subject, body, sent unix, read) into a
// Message, converting the Unix timestamp into a time.Time.
func scan(r scanner) (*Message, error) {
	var (
		m    Message
		sent int64
		read int
	)
	if err := r.Scan(&m.ID, &m.From, &m.To, &m.Subject, &m.Body, &sent, &read); err != nil {
		return nil, err
	}
	if sent != 0 {
		m.Sent = time.Unix(sent, 0)
	}
	m.Read = read != 0
	return &m, nil
}
