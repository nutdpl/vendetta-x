// Package door is Vendetta/X's external-door layer: it lets a sysop register
// real DOS games (run under DOSBox) or native binaries, writes the classic BBS
// drop file (DOOR.SYS / DORINFO1.DEF) into the door's working dir, execs the
// configured command, and bridges the caller's terminal I/O to the process.
// It owns its own SQLite table over the shared database handle; the board ships
// with zero doors (no seeding).
package door

import (
	"database/sql"
	"strings"
)

// Door is one sysop-configured external door.
type Door struct {
	ID          int64
	Name        string // menu label
	Description string
	Command     string // shell-free argv, space-separated; e.g. "dosbox -conf /opt/doors/game/dosbox.conf -exit"
	WorkDir     string // dir the drop file is written into and the process runs in
	DropType    string // "DOOR.SYS" or "DORINFO1.DEF"
	// DOSPath is the DOS-side path the door expects for its own files, written
	// into DOOR.SYS fields 33/34 (e.g. "C:\\DOOR"). It is setup-specific, so it
	// is left to the sysop; empty leaves those fields blank rather than guessing.
	DOSPath string
	Enabled bool
}

// Store is the door data layer over the shared *sql.DB.
type Store struct {
	db *sql.DB
}

// New returns a Store, creating the doors table if absent. It does NOT seed any
// doors: the board ships with an empty roster until a sysop configures one.
func New(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

// migrate creates the table.
func (s *Store) migrate() error {
	const schema = `CREATE TABLE IF NOT EXISTS doors (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		command TEXT NOT NULL DEFAULT '',
		workdir TEXT NOT NULL DEFAULT '',
		drop_type TEXT NOT NULL DEFAULT 'DOOR.SYS',
		dos_path TEXT NOT NULL DEFAULT '',
		enabled INTEGER NOT NULL DEFAULT 0
	);`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	// Idempotent column add for databases created before dos_path existed.
	if _, err := s.db.Exec(
		`ALTER TABLE doors ADD COLUMN dos_path TEXT NOT NULL DEFAULT ''`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	return nil
}

// scanDoors reads rows already selecting the full column set.
func scanDoors(rows *sql.Rows) ([]Door, error) {
	defer rows.Close()
	var out []Door
	for rows.Next() {
		var (
			d       Door
			enabled int64
		)
		if err := rows.Scan(&d.ID, &d.Name, &d.Description, &d.Command,
			&d.WorkDir, &d.DropType, &d.DOSPath, &enabled); err != nil {
			return nil, err
		}
		d.Enabled = enabled != 0
		out = append(out, d)
	}
	return out, rows.Err()
}

// List returns every door, ordered by name (case-insensitive).
func (s *Store) List() ([]Door, error) {
	rows, err := s.db.Query(
		`SELECT id, name, description, command, workdir, drop_type, dos_path, enabled
		 FROM doors ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	return scanDoors(rows)
}

// Enabled returns only enabled doors, ordered by name (case-insensitive).
func (s *Store) Enabled() ([]Door, error) {
	rows, err := s.db.Query(
		`SELECT id, name, description, command, workdir, drop_type, dos_path, enabled
		 FROM doors WHERE enabled = 1 ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	return scanDoors(rows)
}

// Get returns one door by id, or nil,nil if not found.
func (s *Store) Get(id int64) (*Door, error) {
	var (
		d       Door
		enabled int64
	)
	err := s.db.QueryRow(
		`SELECT id, name, description, command, workdir, drop_type, dos_path, enabled
		 FROM doors WHERE id = ?`, id).
		Scan(&d.ID, &d.Name, &d.Description, &d.Command, &d.WorkDir, &d.DropType, &d.DOSPath, &enabled)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d.Enabled = enabled != 0
	return &d, nil
}

// Add inserts a new door, returning its id.
func (s *Store) Add(d *Door) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO doors (name, description, command, workdir, drop_type, dos_path, enabled)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		d.Name, d.Description, d.Command, d.WorkDir, d.DropType, d.DOSPath, boolToInt(d.Enabled))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Update writes the editable fields of an existing door, keyed by d.ID.
func (s *Store) Update(d *Door) error {
	_, err := s.db.Exec(
		`UPDATE doors SET name = ?, description = ?, command = ?, workdir = ?,
		 drop_type = ?, dos_path = ?, enabled = ? WHERE id = ?`,
		d.Name, d.Description, d.Command, d.WorkDir, d.DropType, d.DOSPath, boolToInt(d.Enabled), d.ID)
	return err
}

// Delete removes a door by id.
func (s *Store) Delete(id int64) error {
	_, err := s.db.Exec(`DELETE FROM doors WHERE id = ?`, id)
	return err
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
