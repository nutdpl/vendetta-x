// Package gfiles is Vendetta/X's G-Files: a library of text/info documents
// (the classic "general files" -- rules, NFOs, scene docs). It owns its own
// SQLite table over the shared database handle. This file is the CONTRACT;
// New creates the table and seeds a couple of docs; methods are stubs.
package gfiles

import (
	"database/sql"
	"time"
)

// GFile is one text document in the library.
type GFile struct {
	ID                    int64
	Category, Title, Body string
	Author                string
	Added                 time.Time
}

// Store is the g-files data layer over the shared *sql.DB.
type Store struct {
	db *sql.DB
}

// New returns a Store, creating the gfiles table if absent and seeding a couple
// of documents into an empty library (idempotent).
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
	const schema = `CREATE TABLE IF NOT EXISTS gfiles (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		category TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL DEFAULT '',
		body TEXT NOT NULL DEFAULT '',
		author TEXT NOT NULL DEFAULT '',
		added INTEGER NOT NULL DEFAULT 0
	);`
	_, err := s.db.Exec(schema)
	return err
}

// seed inserts a couple of starter docs if the library is empty (idempotent).
func (s *Store) seed() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM gfiles`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	docs := []GFile{
		{Category: "Info", Title: "Welcome to Vendetta/X", Author: "SysOp",
			Body: "Welcome, caller.\n\nVendetta/X is a modern board built in the old spirit -- ANSI on the\nwire, a web face for the rest. Make yourself at home: read the bases,\ntrade files, leave a mark on the wall.\n\nLong distance is on us. Stay a while."},
		{Category: "Info", Title: "The Rules", Author: "SysOp",
			Body: "1. No flooding, no nuking, no carding.\n2. Respect the other callers.\n3. What's said on the board stays on the board.\n4. The SysOp's word is final.\n\nBreak these and you ride the modem home."},
		{Category: "Scene", Title: "A Brief History of the Scene", Author: "Archivist",
			Body: "Before the web there was the wire. Boards rose and fell on the\nstrength of their file areas, their door games, and the personalities\nthat ran them.\n\nThe art scene -- ACiD, iCE, and the rest -- turned the humble ANSI\nterminal into a canvas. Couriers raced warez across continents at\n2400 baud. It was glorious, and it was ours."},
	}
	now := time.Now().Unix()
	for _, g := range docs {
		if _, err := s.db.Exec(
			`INSERT INTO gfiles (category, title, body, author, added) VALUES (?, ?, ?, ?, ?)`,
			g.Category, g.Title, g.Body, g.Author, now); err != nil {
			return err
		}
	}
	return nil
}

// Categories returns the distinct categories, sorted.
func (s *Store) Categories() ([]string, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT category FROM gfiles ORDER BY category COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// List returns documents in category (or all when category is ""), newest
// first, WITHOUT the body (a lightweight index for listings).
func (s *Store) List(category string) ([]GFile, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if category == "" {
		rows, err = s.db.Query(
			`SELECT id, category, title, author, added FROM gfiles ORDER BY added DESC, id DESC`)
	} else {
		rows, err = s.db.Query(
			`SELECT id, category, title, author, added FROM gfiles
			 WHERE category = ? COLLATE NOCASE ORDER BY added DESC, id DESC`, category)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GFile
	for rows.Next() {
		var (
			g     GFile
			added int64
		)
		if err := rows.Scan(&g.ID, &g.Category, &g.Title, &g.Author, &added); err != nil {
			return nil, err
		}
		if added != 0 {
			g.Added = time.Unix(added, 0)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// Get returns one document by id (with its body), or nil,nil if not found.
func (s *Store) Get(id int64) (*GFile, error) {
	var (
		g     GFile
		added int64
	)
	err := s.db.QueryRow(
		`SELECT id, category, title, body, author, added FROM gfiles WHERE id = ?`, id).
		Scan(&g.ID, &g.Category, &g.Title, &g.Body, &g.Author, &added)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if added != 0 {
		g.Added = time.Unix(added, 0)
	}
	return &g, nil
}

// Add inserts a new document (Added stamps to now), returning its id.
func (s *Store) Add(g *GFile) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO gfiles (category, title, body, author, added) VALUES (?, ?, ?, ?, ?)`,
		g.Category, g.Title, g.Body, g.Author, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Update writes the editable fields (category, title, body, author) of an
// existing document, keyed by g.ID. Added is left untouched.
func (s *Store) Update(g *GFile) error {
	_, err := s.db.Exec(
		`UPDATE gfiles SET category = ?, title = ?, body = ?, author = ? WHERE id = ?`,
		g.Category, g.Title, g.Body, g.Author, g.ID)
	return err
}

// Delete removes a document by id.
func (s *Store) Delete(id int64) error {
	_, err := s.db.Exec(`DELETE FROM gfiles WHERE id = ?`, id)
	return err
}
