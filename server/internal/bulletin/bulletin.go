// Package bulletin is Vendetta/X's bulletins: short sysop-authored
// announcements shown to callers at logon. It owns its own SQLite table over
// the shared database handle. New creates the table and seeds a single welcome
// bulletin into an empty table (idempotent).
package bulletin

import (
	"database/sql"
	"time"
)

// Bulletin is one short announcement shown to callers at logon.
type Bulletin struct {
	ID          int64
	Title, Body string
	Author      string
	Posted      time.Time
}

// Store is the bulletins data layer over the shared *sql.DB.
type Store struct {
	db *sql.DB
}

// New returns a Store, creating the bulletins table if absent and seeding a
// single welcome bulletin into an empty table (idempotent).
func New(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	if err := s.seed(); err != nil {
		return nil, err
	}
	return s, nil
}

// migrate creates the table.
func (s *Store) migrate() error {
	const schema = `CREATE TABLE IF NOT EXISTS bulletins (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL DEFAULT '',
		body TEXT NOT NULL DEFAULT '',
		author TEXT NOT NULL DEFAULT '',
		posted INTEGER NOT NULL DEFAULT 0
	);`
	_, err := s.db.Exec(schema)
	return err
}

// seed inserts a single welcome bulletin if the table is empty (idempotent).
func (s *Store) seed() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM bulletins`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err := s.db.Exec(
		`INSERT INTO bulletins (title, body, author, posted) VALUES (?, ?, ?, ?)`,
		"Welcome to Vendetta/X",
		"Glad to have you on the board, caller.\n\nWatch this space for news from the SysOp -- new file areas, events,\nand the occasional word from the desk. Make yourself at home.",
		"SysOp",
		time.Now().Unix())
	return err
}

// List returns all bulletins newest first, including their bodies.
func (s *Store) List() ([]Bulletin, error) {
	rows, err := s.db.Query(
		`SELECT id, title, body, author, posted FROM bulletins ORDER BY posted DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Bulletin
	for rows.Next() {
		var (
			b      Bulletin
			posted int64
		)
		if err := rows.Scan(&b.ID, &b.Title, &b.Body, &b.Author, &posted); err != nil {
			return nil, err
		}
		if posted != 0 {
			b.Posted = time.Unix(posted, 0)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// Get returns one bulletin by id, or nil,nil if not found.
func (s *Store) Get(id int64) (*Bulletin, error) {
	var (
		b      Bulletin
		posted int64
	)
	err := s.db.QueryRow(
		`SELECT id, title, body, author, posted FROM bulletins WHERE id = ?`, id).
		Scan(&b.ID, &b.Title, &b.Body, &b.Author, &posted)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if posted != 0 {
		b.Posted = time.Unix(posted, 0)
	}
	return &b, nil
}

// Add inserts a new bulletin (Posted stamps to now if zero), returning its id.
func (s *Store) Add(b *Bulletin) (int64, error) {
	posted := b.Posted
	if posted.IsZero() {
		posted = time.Now()
	}
	res, err := s.db.Exec(
		`INSERT INTO bulletins (title, body, author, posted) VALUES (?, ?, ?, ?)`,
		b.Title, b.Body, b.Author, posted.Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Update writes the editable fields (title, body, author) of an existing
// bulletin, keyed by b.ID. Posted is left untouched.
func (s *Store) Update(b *Bulletin) error {
	_, err := s.db.Exec(
		`UPDATE bulletins SET title = ?, body = ?, author = ? WHERE id = ?`,
		b.Title, b.Body, b.Author, b.ID)
	return err
}

// Delete removes a bulletin by id.
func (s *Store) Delete(id int64) error {
	_, err := s.db.Exec(`DELETE FROM bulletins WHERE id = ?`, id)
	return err
}
