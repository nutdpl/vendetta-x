// Sysop configuration mutations: the create/update/delete operations the sysop
// panel needs to fully configure the board (message bases, file areas, users,
// oneliners). Read paths live in store.go; this file holds the writes that the
// "configuration program" (the web /sysop panel) drives.
package store

import (
	"database/sql"
	"fmt"

	"vendetta-x/server/internal/sanitize"
)

// tx runs fn inside a transaction, committing on success and rolling back on
// any error. It keeps the multi-statement cascades below readable.
func (s *Store) tx(fn func(*sql.Tx) error) error {
	t, err := s.db.Begin()
	if err != nil {
		return err
	}
	if err := fn(t); err != nil {
		t.Rollback()
		return err
	}
	return t.Commit()
}

// ---- message boards --------------------------------------------------------

// BoardByID returns one board by id, or nil,nil if it does not exist.
func (s *Store) BoardByID(id int64) (*Board, error) {
	var b Board
	err := s.db.QueryRow(
		`SELECT id, tag, name, descr, min_read_sl, min_post_sl, read_acs, post_acs
		 FROM boards WHERE id = ?`, id).
		Scan(&b.ID, &b.Tag, &b.Name, &b.Desc, &b.MinReadSL, &b.MinPostSL, &b.ReadACS, &b.PostACS)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: board %d: %w", id, err)
	}
	return &b, nil
}

// AddBoard inserts a new message base, returning its id (and stamping b.ID).
func (s *Store) AddBoard(b *Board) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO boards (tag, name, descr, min_read_sl, min_post_sl, read_acs, post_acs)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		b.Tag, b.Name, b.Desc, b.MinReadSL, b.MinPostSL, b.ReadACS, b.PostACS)
	if err != nil {
		return 0, fmt.Errorf("store: add board: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: add board id: %w", err)
	}
	b.ID = id
	return id, nil
}

// UpdateBoard saves an existing board's editable fields, keyed by b.ID.
func (s *Store) UpdateBoard(b *Board) error {
	_, err := s.db.Exec(
		`UPDATE boards SET tag = ?, name = ?, descr = ?, min_read_sl = ?, min_post_sl = ?,
		 read_acs = ?, post_acs = ? WHERE id = ?`,
		b.Tag, b.Name, b.Desc, b.MinReadSL, b.MinPostSL, b.ReadACS, b.PostACS, b.ID)
	if err != nil {
		return fmt.Errorf("store: update board %d: %w", b.ID, err)
	}
	return nil
}

// DeleteBoard removes a board and every message posted to it, atomically.
func (s *Store) DeleteBoard(id int64) error {
	return s.tx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM messages WHERE board_id = ?`, id); err != nil {
			return err
		}
		_, err := tx.Exec(`DELETE FROM boards WHERE id = ?`, id)
		return err
	})
}

// ---- file areas ------------------------------------------------------------

// FileAreaByID returns one file area by id, or nil,nil if it does not exist.
func (s *Store) FileAreaByID(id int64) (*FileArea, error) {
	var a FileArea
	err := s.db.QueryRow(
		`SELECT id, tag, name, descr, acs FROM file_areas WHERE id = ?`, id).
		Scan(&a.ID, &a.Tag, &a.Name, &a.Desc, &a.ACS)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: file area %d: %w", id, err)
	}
	return &a, nil
}

// AddFileArea inserts a new file area, returning its id (and stamping a.ID).
func (s *Store) AddFileArea(a *FileArea) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO file_areas (tag, name, descr, acs) VALUES (?, ?, ?, ?)`,
		a.Tag, a.Name, a.Desc, a.ACS)
	if err != nil {
		return 0, fmt.Errorf("store: add file area: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: add file area id: %w", err)
	}
	a.ID = id
	return id, nil
}

// UpdateFileArea saves an existing area's editable fields, keyed by a.ID.
func (s *Store) UpdateFileArea(a *FileArea) error {
	_, err := s.db.Exec(
		`UPDATE file_areas SET tag = ?, name = ?, descr = ?, acs = ? WHERE id = ?`,
		a.Tag, a.Name, a.Desc, a.ACS, a.ID)
	if err != nil {
		return fmt.Errorf("store: update file area %d: %w", a.ID, err)
	}
	return nil
}

// DeleteFileArea removes an area and every file in it, atomically.
func (s *Store) DeleteFileArea(id int64) error {
	return s.tx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM files WHERE area_id = ?`, id); err != nil {
			return err
		}
		_, err := tx.Exec(`DELETE FROM file_areas WHERE id = ?`, id)
		return err
	})
}

// ---- users -----------------------------------------------------------------

// UpdateUser saves a user's sysop-editable profile/access fields, keyed by
// u.ID. Handle (identity), password (set via SetPassword), and counters are
// intentionally left untouched.
func (s *Store) UpdateUser(u *User) error {
	u.RealName = sanitize.Line(u.RealName)
	u.Email = sanitize.Line(u.Email)
	u.Location = sanitize.Line(u.Location)
	u.Tagline = sanitize.Line(u.Tagline)
	u.Group = sanitize.Line(u.Group)
	u.Flags = sanitize.Line(u.Flags)
	_, err := s.db.Exec(
		`UPDATE users SET real_name = ?, email = ?, location = ?, tagline = ?,
		 grp = ?, sl = ?, dsl = ?, flags = ? WHERE id = ?`,
		u.RealName, u.Email, u.Location, u.Tagline, u.Group, u.SL, u.DSL, u.Flags, u.ID)
	if err != nil {
		return fmt.Errorf("store: update user %d: %w", u.ID, err)
	}
	return nil
}

// DeleteUser removes a user account by id. Their posted content (messages,
// oneliners, files) is left in place, attributed by handle as before.
func (s *Store) DeleteUser(id int64) error {
	if _, err := s.db.Exec(`DELETE FROM users WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: delete user %d: %w", id, err)
	}
	return nil
}

// ---- oneliners -------------------------------------------------------------

// DeleteOneliner removes one wall entry by id (sysop moderation).
func (s *Store) DeleteOneliner(id int64) error {
	if _, err := s.db.Exec(`DELETE FROM oneliners WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: delete oneliner %d: %w", id, err)
	}
	return nil
}
