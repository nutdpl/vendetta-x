// Package bbslist is Vendetta/X's BBS List: a directory of other boards. It
// owns its own SQLite table over the shared database handle. This file is the
// CONTRACT; New creates the table and seeds a few classics; methods are stubs.
package bbslist

import (
	"database/sql"
	"time"

	"vendetta-x/server/internal/sanitize"
)

// Entry is one board in the directory.
type Entry struct {
	ID                            int64
	Name, Address, Software, Desc string
	Sysop                         string
	Added                         time.Time
}

// Store is the bbs-list data layer over the shared *sql.DB.
type Store struct {
	db *sql.DB
}

// New returns a Store, creating the bbs_list table if absent and seeding a few
// classic boards into an empty list (idempotent).
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
	const schema = `CREATE TABLE IF NOT EXISTS bbs_list (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL DEFAULT '',
		address TEXT NOT NULL DEFAULT '',
		software TEXT NOT NULL DEFAULT '',
		sysop TEXT NOT NULL DEFAULT '',
		descr TEXT NOT NULL DEFAULT '',
		added INTEGER NOT NULL DEFAULT 0
	);`
	_, err := s.db.Exec(schema)
	return err
}

// seed inserts a few well-known boards if the list is empty (idempotent).
func (s *Store) seed() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM bbs_list`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	classics := []Entry{
		{Name: "Mindvox", Address: "phantom.com", Software: "Custom UNIX", Sysop: "Bruce Fancher", Desc: "The legendary cyberpunk/hacker hangout of the early '90s."},
		{Name: "The Works", Address: "the-works.bbs", Software: "Citadel", Sysop: "Eric Rosenquist", Desc: "Long-running Boston-area Citadel board."},
		{Name: "Rusty n Edie's", Address: "rusty.bbs", Software: "PCBoard", Sysop: "Russell & Edwina Hardenburgh", Desc: "Sprawling Ohio file-trading megaboard."},
		{Name: "ACiD Underworld", Address: "underworld.acid", Software: "Oblivion/2", Sysop: "ACiD Productions", Desc: "ANSI art scene HQ and distro site."},
	}
	now := time.Now().Unix()
	for _, e := range classics {
		if _, err := s.db.Exec(
			`INSERT INTO bbs_list (name, address, software, sysop, descr, added) VALUES (?, ?, ?, ?, ?, ?)`,
			e.Name, e.Address, e.Software, e.Sysop, e.Desc, now); err != nil {
			return err
		}
	}
	return nil
}

// List returns all directory entries, ordered by name (case-insensitive).
func (s *Store) List() ([]Entry, error) {
	rows, err := s.db.Query(
		`SELECT id, name, address, software, sysop, descr, added FROM bbs_list
		 ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		e, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

// Get returns one entry by id, or nil,nil if not found.
func (s *Store) Get(id int64) (*Entry, error) {
	row := s.db.QueryRow(
		`SELECT id, name, address, software, sysop, descr, added FROM bbs_list WHERE id = ?`, id)
	e, err := scan(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return e, nil
}

// clean strips control bytes from an entry's user-supplied fields (these show
// in the telnet BBS-list screen, so ANSI must not leak through).
func clean(e *Entry) {
	e.Name = sanitize.Line(e.Name)
	e.Address = sanitize.Line(e.Address)
	e.Software = sanitize.Line(e.Software)
	e.Sysop = sanitize.Line(e.Sysop)
	e.Desc = sanitize.Line(e.Desc)
}

// Add inserts a new directory entry (Added stamps to now), returning its id.
func (s *Store) Add(e *Entry) (int64, error) {
	clean(e)
	res, err := s.db.Exec(
		`INSERT INTO bbs_list (name, address, software, sysop, descr, added) VALUES (?, ?, ?, ?, ?, ?)`,
		e.Name, e.Address, e.Software, e.Sysop, e.Desc, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Update writes the editable fields of an existing entry, matched by ID.
func (s *Store) Update(e *Entry) error {
	clean(e)
	_, err := s.db.Exec(
		`UPDATE bbs_list SET name = ?, address = ?, software = ?, sysop = ?, descr = ? WHERE id = ?`,
		e.Name, e.Address, e.Software, e.Sysop, e.Desc, e.ID)
	return err
}

// Delete removes a directory entry by id.
func (s *Store) Delete(id int64) error {
	_, err := s.db.Exec(`DELETE FROM bbs_list WHERE id = ?`, id)
	return err
}

// scanner is the shared interface of *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...interface{}) error
}

// scan decodes one row into an Entry, converting the Unix timestamp.
func scan(r scanner) (*Entry, error) {
	var (
		e     Entry
		added int64
	)
	if err := r.Scan(&e.ID, &e.Name, &e.Address, &e.Software, &e.Sysop, &e.Desc, &added); err != nil {
		return nil, err
	}
	if added != 0 {
		e.Added = time.Unix(added, 0)
	}
	return &e, nil
}
