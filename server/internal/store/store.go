// Package store is Vendetta/X's shared data spine: a single SQLite database
// that every face (telnet, web, sysop panel) reads and writes. This file is
// the CONTRACT -- the types and method signatures the rest of the platform
// codes against. The query bodies are stubs to be implemented against SQLite.
package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"vendetta-x/server/internal/sanitize"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (no CGO)
)

// ---- domain types ----------------------------------------------------------

type User struct {
	ID                  int64
	Handle, RealName    string
	Email, Location     string
	Tagline, Group      string
	SL, DSL             int
	Posts, Calls        int
	FirstCall, LastCall time.Time
	// Password is the bcrypt hash (set via SetPassword); never the plaintext.
	Password string
	// Flags is the user's ACS flag set, e.g. "AC" means flags A and C are set.
	Flags string
}

type Board struct {
	ID                   int64
	Tag, Name, Desc      string
	MinReadSL, MinPostSL int
	// ReadACS / PostACS gate read and post access (Iniquity-style ACS strings,
	// evaluated by internal/acs). Empty means "open".
	ReadACS, PostACS string
}

type Message struct {
	ID, BoardID             int64
	From, To, Subject, Body string
	Posted                  time.Time
}

type FileArea struct {
	ID              int64
	Tag, Name, Desc string
	// ACS gates access to the area (empty means "open").
	ACS string
}

type FileEntry struct {
	ID, AreaID int64
	Filename   string
	Desc       string
	Uploader   string
	Size       int64
	Uploaded   time.Time
	Downloads  int
}

type Oneliner struct {
	ID           int64
	Author, Text string
	Posted       time.Time
}

// ---- the store -------------------------------------------------------------

type Store struct {
	db *sql.DB
}

// Open opens (creating if absent) the SQLite database at path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

// DB exposes the handle for the implementer (and advanced callers).
func (s *Store) DB() *sql.DB { return s.db }

// ---- time helpers ----------------------------------------------------------
//
// time.Time values are stored as Unix seconds (int64). A zero time.Time is
// stored as 0 and read back as the zero value, keeping round-trips clean.

func toUnix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

func fromUnix(n int64) time.Time {
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(n, 0)
}

// ---- migration -------------------------------------------------------------

// migrate creates the schema if absent and enables WAL for multi-process use.
func (s *Store) migrate() error {
	if _, err := s.db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		return fmt.Errorf("store: enable WAL: %w", err)
	}
	// busy_timeout makes a writer wait (and retry) for up to 5s when another
	// writer holds the lock, instead of failing immediately with "database is
	// locked" under concurrent writes from the three faces.
	if _, err := s.db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		return fmt.Errorf("store: set busy_timeout: %w", err)
	}
	const schema = `
CREATE TABLE IF NOT EXISTS users (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	handle     TEXT NOT NULL,
	real_name  TEXT NOT NULL DEFAULT '',
	email      TEXT NOT NULL DEFAULT '',
	location   TEXT NOT NULL DEFAULT '',
	tagline    TEXT NOT NULL DEFAULT '',
	grp        TEXT NOT NULL DEFAULT '',
	sl         INTEGER NOT NULL DEFAULT 0,
	dsl        INTEGER NOT NULL DEFAULT 0,
	posts      INTEGER NOT NULL DEFAULT 0,
	calls      INTEGER NOT NULL DEFAULT 0,
	first_call INTEGER NOT NULL DEFAULT 0,
	last_call  INTEGER NOT NULL DEFAULT 0,
	password   TEXT NOT NULL DEFAULT '',
	flags      TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_handle_nocase
	ON users (handle COLLATE NOCASE);

CREATE TABLE IF NOT EXISTS boards (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	tag          TEXT NOT NULL,
	name         TEXT NOT NULL,
	descr        TEXT NOT NULL DEFAULT '',
	min_read_sl  INTEGER NOT NULL DEFAULT 0,
	min_post_sl  INTEGER NOT NULL DEFAULT 0,
	read_acs     TEXT NOT NULL DEFAULT '',
	post_acs     TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS messages (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	board_id  INTEGER NOT NULL,
	from_who  TEXT NOT NULL DEFAULT '',
	to_who    TEXT NOT NULL DEFAULT '',
	subject   TEXT NOT NULL DEFAULT '',
	body      TEXT NOT NULL DEFAULT '',
	posted    INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_messages_board ON messages (board_id, posted DESC);

CREATE TABLE IF NOT EXISTS file_areas (
	id    INTEGER PRIMARY KEY AUTOINCREMENT,
	tag   TEXT NOT NULL,
	name  TEXT NOT NULL,
	descr TEXT NOT NULL DEFAULT '',
	acs   TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS files (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	area_id   INTEGER NOT NULL,
	filename  TEXT NOT NULL,
	descr     TEXT NOT NULL DEFAULT '',
	uploader  TEXT NOT NULL DEFAULT '',
	size      INTEGER NOT NULL DEFAULT 0,
	uploaded  INTEGER NOT NULL DEFAULT 0,
	downloads INTEGER NOT NULL DEFAULT 0,
	content   BLOB
);
CREATE INDEX IF NOT EXISTS idx_files_area ON files (area_id);

CREATE TABLE IF NOT EXISTS oneliners (
	id     INTEGER PRIMARY KEY AUTOINCREMENT,
	author TEXT NOT NULL DEFAULT '',
	text   TEXT NOT NULL DEFAULT '',
	posted INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL DEFAULT ''
);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("store: create schema: %w", err)
	}

	// Idempotent column upgrades for databases created before these columns
	// existed. SQLite has no "ADD COLUMN IF NOT EXISTS", so we ignore the
	// "duplicate column name" error each ALTER raises when already applied.
	addColumns := []string{
		`ALTER TABLE users ADD COLUMN password TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN flags TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE boards ADD COLUMN read_acs TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE boards ADD COLUMN post_acs TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE file_areas ADD COLUMN acs TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE files ADD COLUMN content BLOB`,
	}
	for _, stmt := range addColumns {
		if _, err := s.db.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("store: migrate column: %w", err)
		}
	}
	return nil
}

// ---- seed ------------------------------------------------------------------

// Seed inserts default boards / file areas / sample content into an empty DB.
// It is idempotent: if any board already exists, Seed is a no-op.
func (s *Store) Seed() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM boards`).Scan(&n); err != nil {
		return fmt.Errorf("store: seed count boards: %w", err)
	}
	if n > 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("store: seed begin: %w", err)
	}
	defer tx.Rollback()

	// boards. The sysop board is gated by an ACS string: only SL >= 100 may
	// read or post -- the canonical Iniquity-style access demo.
	boards := []struct {
		tag, name, desc  string
		readACS, postACS string
	}{
		{"gen", "General", "General chatter and introductions.", "", ""},
		{"warez", "Warez Talk", "Releases, cracks, and trade talk.", "", ""},
		{"scene", "The Scene", "Groups, NFOs, and demoscene news.", "", ""},
		{"sysop", "Sysop", "Sysop announcements and board business.", "s100", "s100"},
	}
	boardID := map[string]int64{}
	for _, b := range boards {
		res, err := tx.Exec(
			`INSERT INTO boards (tag, name, descr, min_read_sl, min_post_sl, read_acs, post_acs) VALUES (?, ?, ?, 0, 0, ?, ?)`,
			b.tag, b.name, b.desc, b.readACS, b.postACS)
		if err != nil {
			return fmt.Errorf("store: seed board %q: %w", b.tag, err)
		}
		id, _ := res.LastInsertId()
		boardID[b.tag] = id
	}

	// file areas
	areas := []struct{ tag, name, desc string }{
		{"warez", "Warez Vault", "The good stuff. Mind the ratio."},
		{"util", "Utilities", "Tools, packers, and crackers' helpers."},
		{"ansi", "ANSI & Art", "ANSI screens, NFOs, and ASCII collies."},
	}
	areaID := map[string]int64{}
	for _, a := range areas {
		res, err := tx.Exec(
			`INSERT INTO file_areas (tag, name, descr) VALUES (?, ?, ?)`,
			a.tag, a.name, a.desc)
		if err != nil {
			return fmt.Errorf("store: seed area %q: %w", a.tag, err)
		}
		id, _ := res.LastInsertId()
		areaID[a.tag] = id
	}

	// sample files (so the file areas have something to show)
	files := []struct {
		area, name, desc, uploader string
		size                       int64
	}{
		{"warez", "VENDETTA-KEYGEN.ZIP", "Keygen for the whole suite. Tested, clean.", "nut", 48213},
		{"warez", "NIGHTFALL-DISK1.ARJ", "Cracked, packed, ready. 3 disks, this is 1/3.", "phantom", 1457620},
		{"warez", "SCENE-PACK-0696.RAR", "Monthly scene pack. NFOs inside.", "nut", 8923400},
		{"util", "PKZIP204G.EXE", "The one and only. Don't get caught without it.", "nut", 196734},
		{"util", "ARJ241A.EXE", "ARJ archiver, the elite choice for multi-disk.", "phantom", 121044},
		{"util", "VGACOPY.COM", "Bit-exact disk copier. For the archivists.", "nut", 33280},
		{"ansi", "VENDETTA.ANS", "Our login matrix in full ANSI glory.", "nut", 12044},
		{"ansi", "GREETS-96.NFO", "Greets and shouts to the crews still riding.", "phantom", 6210},
	}
	for _, f := range files {
		aid, ok := areaID[f.area]
		if !ok {
			return fmt.Errorf("store: seed file: unknown area %q", f.area)
		}
		// Real (small) stored content so downloads serve actual bytes; size
		// reflects the stored length so the listing never lies.
		content := seedFileContent(f.name, f.desc, f.uploader)
		if _, err := tx.Exec(
			`INSERT INTO files (area_id, filename, descr, uploader, size, uploaded, downloads, content)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			aid, f.name, f.desc, f.uploader, int64(len(content)), toUnix(time.Now()), 0, content); err != nil {
			return fmt.Errorf("store: seed file %q: %w", f.name, err)
		}
	}

	// sample users
	now := time.Now()
	users := []struct {
		handle, real, group, tagline, flags string
		sl                                  int
	}{
		{"nut", "nut", "Staff", "Running the show.", "A", 255},
		{"phantom", "Phantom", "Users", "Lurking in the static.", "", 10},
	}
	for _, u := range users {
		if _, err := tx.Exec(
			`INSERT INTO users (handle, real_name, grp, tagline, sl, dsl, first_call, last_call, flags)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			u.handle, u.real, u.group, u.tagline, u.sl, u.sl, toUnix(now), toUnix(now), u.flags); err != nil {
			return fmt.Errorf("store: seed user %q: %w", u.handle, err)
		}
	}

	// sample messages
	msgs := []struct {
		board, from, subj, body string
	}{
		{"gen", "nut", "Welcome to Vendetta/X", "Glad to have you aboard. Read the rules, post often, keep it clean-ish."},
		{"gen", "phantom", "First post", "Long-time caller, first-time poster. This board feels alive already."},
		{"warez", "nut", "Trade rules", "No begging. Maintain ratio. Fakes get you banned."},
	}
	for i, m := range msgs {
		bid, ok := boardID[m.board]
		if !ok {
			return fmt.Errorf("store: seed message: unknown board %q", m.board)
		}
		if _, err := tx.Exec(
			`INSERT INTO messages (board_id, from_who, to_who, subject, body, posted)
			 VALUES (?, ?, 'All', ?, ?, ?)`,
			bid, m.from, m.subj, m.body, toUnix(now.Add(time.Duration(i)*time.Minute))); err != nil {
			return fmt.Errorf("store: seed message: %w", err)
		}
	}

	// sample oneliners
	liners := []struct{ author, text string }{
		{"nut", "Welcome to the wall. Keep it short, keep it sharp."},
		{"phantom", "Greets to everyone still riding the modem at 3am."},
		{"nut", "The scene never died, it just went quiet."},
		{"phantom", "Ratio is law."},
	}
	for i, l := range liners {
		if _, err := tx.Exec(
			`INSERT INTO oneliners (author, text, posted) VALUES (?, ?, ?)`,
			l.author, l.text, toUnix(now.Add(time.Duration(i)*time.Second))); err != nil {
			return fmt.Errorf("store: seed oneliner: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: seed commit: %w", err)
	}
	return nil
}

// ---- users -----------------------------------------------------------------

const userCols = `id, handle, real_name, email, location, tagline, grp, sl, dsl, posts, calls, first_call, last_call, password, flags`

func scanUser(sc interface{ Scan(...any) error }) (*User, error) {
	var u User
	var first, last int64
	if err := sc.Scan(&u.ID, &u.Handle, &u.RealName, &u.Email, &u.Location,
		&u.Tagline, &u.Group, &u.SL, &u.DSL, &u.Posts, &u.Calls, &first, &last,
		&u.Password, &u.Flags); err != nil {
		return nil, err
	}
	u.FirstCall = fromUnix(first)
	u.LastCall = fromUnix(last)
	return &u, nil
}

// UserByHandle returns the user (case-insensitive), or nil,nil if not found.
func (s *Store) UserByHandle(handle string) (*User, error) {
	row := s.db.QueryRow(
		`SELECT `+userCols+` FROM users WHERE handle = ? COLLATE NOCASE`, handle)
	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: user by handle %q: %w", handle, err)
	}
	return u, nil
}

func (s *Store) AddUser(u *User) (int64, error) {
	// Strip control bytes from anything echoed to other callers' terminals.
	u.Handle = sanitize.Line(u.Handle)
	u.RealName = sanitize.Line(u.RealName)
	u.Email = sanitize.Line(u.Email)
	u.Location = sanitize.Line(u.Location)
	u.Tagline = sanitize.Line(u.Tagline)
	u.Group = sanitize.Line(u.Group)
	u.Flags = sanitize.Line(u.Flags)
	res, err := s.db.Exec(
		`INSERT INTO users (handle, real_name, email, location, tagline, grp, sl, dsl, posts, calls, first_call, last_call, password, flags)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.Handle, u.RealName, u.Email, u.Location, u.Tagline, u.Group,
		u.SL, u.DSL, u.Posts, u.Calls, toUnix(u.FirstCall), toUnix(u.LastCall),
		u.Password, u.Flags)
	if err != nil {
		return 0, fmt.Errorf("store: add user %q: %w", u.Handle, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: add user %q id: %w", u.Handle, err)
	}
	u.ID = id
	return id, nil
}

func (s *Store) Users() ([]User, error) {
	rows, err := s.db.Query(`SELECT ` + userCols + ` FROM users ORDER BY handle COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("store: users: %w", err)
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("store: users scan: %w", err)
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// ---- message boards --------------------------------------------------------

func (s *Store) Boards() ([]Board, error) {
	rows, err := s.db.Query(
		`SELECT id, tag, name, descr, min_read_sl, min_post_sl, read_acs, post_acs FROM boards ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("store: boards: %w", err)
	}
	defer rows.Close()
	var out []Board
	for rows.Next() {
		var b Board
		if err := rows.Scan(&b.ID, &b.Tag, &b.Name, &b.Desc, &b.MinReadSL, &b.MinPostSL,
			&b.ReadACS, &b.PostACS); err != nil {
			return nil, fmt.Errorf("store: boards scan: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func scanMessages(rows *sql.Rows) ([]Message, error) {
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		var posted int64
		if err := rows.Scan(&m.ID, &m.BoardID, &m.From, &m.To, &m.Subject, &m.Body, &posted); err != nil {
			return nil, fmt.Errorf("store: messages scan: %w", err)
		}
		m.Posted = fromUnix(posted)
		out = append(out, m)
	}
	return out, rows.Err()
}

// Messages returns a board's messages newest-first. limit<=0 means all.
func (s *Store) Messages(boardID int64, limit int) ([]Message, error) {
	q := `SELECT id, board_id, from_who, to_who, subject, body, posted
	      FROM messages WHERE board_id = ? ORDER BY posted DESC, id DESC`
	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = s.db.Query(q+` LIMIT ?`, boardID, limit)
	} else {
		rows, err = s.db.Query(q, boardID)
	}
	if err != nil {
		return nil, fmt.Errorf("store: messages board %d: %w", boardID, err)
	}
	return scanMessages(rows)
}

// RecentMessages returns the newest messages across all boards. limit<=0 means all.
func (s *Store) RecentMessages(limit int) ([]Message, error) {
	q := `SELECT id, board_id, from_who, to_who, subject, body, posted
	      FROM messages ORDER BY posted DESC, id DESC`
	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = s.db.Query(q+` LIMIT ?`, limit)
	} else {
		rows, err = s.db.Query(q)
	}
	if err != nil {
		return nil, fmt.Errorf("store: recent messages: %w", err)
	}
	return scanMessages(rows)
}

func (s *Store) PostMessage(m *Message) (int64, error) {
	m.From = sanitize.Line(m.From)
	m.To = sanitize.Line(m.To)
	m.Subject = sanitize.Line(m.Subject)
	m.Body = sanitize.Text(m.Body)
	posted := m.Posted
	if posted.IsZero() {
		posted = time.Now()
	}
	res, err := s.db.Exec(
		`INSERT INTO messages (board_id, from_who, to_who, subject, body, posted)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		m.BoardID, m.From, m.To, m.Subject, m.Body, toUnix(posted))
	if err != nil {
		return 0, fmt.Errorf("store: post message: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: post message id: %w", err)
	}
	m.ID = id
	m.Posted = posted
	return id, nil
}

// ---- file areas ------------------------------------------------------------

func (s *Store) FileAreas() ([]FileArea, error) {
	rows, err := s.db.Query(`SELECT id, tag, name, descr, acs FROM file_areas ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("store: file areas: %w", err)
	}
	defer rows.Close()
	var out []FileArea
	for rows.Next() {
		var a FileArea
		if err := rows.Scan(&a.ID, &a.Tag, &a.Name, &a.Desc, &a.ACS); err != nil {
			return nil, fmt.Errorf("store: file areas scan: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) Files(areaID int64) ([]FileEntry, error) {
	rows, err := s.db.Query(
		`SELECT id, area_id, filename, descr, uploader, size, uploaded, downloads
		 FROM files WHERE area_id = ? ORDER BY filename COLLATE NOCASE`, areaID)
	if err != nil {
		return nil, fmt.Errorf("store: files area %d: %w", areaID, err)
	}
	defer rows.Close()
	var out []FileEntry
	for rows.Next() {
		var f FileEntry
		var uploaded int64
		if err := rows.Scan(&f.ID, &f.AreaID, &f.Filename, &f.Desc, &f.Uploader,
			&f.Size, &uploaded, &f.Downloads); err != nil {
			return nil, fmt.Errorf("store: files scan: %w", err)
		}
		f.Uploaded = fromUnix(uploaded)
		out = append(out, f)
	}
	return out, rows.Err()
}

// FileByID returns a file's metadata (no content), or nil,nil if not found.
func (s *Store) FileByID(id int64) (*FileEntry, error) {
	row := s.db.QueryRow(
		`SELECT id, area_id, filename, descr, uploader, size, uploaded, downloads
		 FROM files WHERE id = ?`, id)
	var f FileEntry
	var uploaded int64
	err := row.Scan(&f.ID, &f.AreaID, &f.Filename, &f.Desc, &f.Uploader,
		&f.Size, &uploaded, &f.Downloads)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: file by id %d: %w", id, err)
	}
	f.Uploaded = fromUnix(uploaded)
	return &f, nil
}

// FileContent returns the stored bytes for a file (nil if the row has no
// content), or an error. A missing file id yields nil,nil.
func (s *Store) FileContent(id int64) ([]byte, error) {
	row := s.db.QueryRow(`SELECT content FROM files WHERE id = ?`, id)
	var content []byte
	err := row.Scan(&content)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: file content %d: %w", id, err)
	}
	return content, nil
}

// IncDownload bumps a file's download counter.
func (s *Store) IncDownload(id int64) error {
	if _, err := s.db.Exec(`UPDATE files SET downloads = downloads + 1 WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: inc download: %w", err)
	}
	return nil
}

// AddFile stores an uploaded file (its bytes become the content; size is the
// byte length). Returns the new file id.
func (s *Store) AddFile(areaID int64, filename, desc, uploader string, content []byte) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO files (area_id, filename, descr, uploader, size, uploaded, downloads, content)
		 VALUES (?, ?, ?, ?, ?, ?, 0, ?)`,
		areaID, filename, desc, uploader, int64(len(content)), toUnix(time.Now()), content)
	if err != nil {
		return 0, fmt.Errorf("store: add file %q: %w", filename, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: add file %q id: %w", filename, err)
	}
	return id, nil
}

// seedFileContent builds a small, real NFO-style payload for a seeded file so
// downloads serve actual bytes (the listing's size reflects this length).
func seedFileContent(name, desc, uploader string) []byte {
	const rule = "+--------------------------------------------------------------+"
	var b strings.Builder
	b.WriteString(rule + "\r\n")
	b.WriteString("|  V E N D E T T A / X            t h e   d i s t r o          |\r\n")
	b.WriteString(rule + "\r\n")
	fmt.Fprintf(&b, "\r\n  file......: %s\r\n", name)
	fmt.Fprintf(&b, "  uploader..: %s\r\n", uploader)
	fmt.Fprintf(&b, "  notes.....: %s\r\n\r\n", desc)
	b.WriteString("  This is a sample release from the Vendetta/X distro. Real\r\n")
	b.WriteString("  uploads land here once the upload path ships; for now this\r\n")
	b.WriteString("  NFO is the genuine stored payload you just downloaded.\r\n\r\n")
	b.WriteString("  greets to everyone still riding the modem at 3am.\r\n")
	b.WriteString(rule + "\r\n")
	return []byte(b.String())
}

// ---- oneliners (the wall) --------------------------------------------------

// Oneliners returns wall posts newest-first. limit<=0 means all.
func (s *Store) Oneliners(limit int) ([]Oneliner, error) {
	q := `SELECT id, author, text, posted FROM oneliners ORDER BY posted DESC, id DESC`
	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = s.db.Query(q+` LIMIT ?`, limit)
	} else {
		rows, err = s.db.Query(q)
	}
	if err != nil {
		return nil, fmt.Errorf("store: oneliners: %w", err)
	}
	defer rows.Close()
	var out []Oneliner
	for rows.Next() {
		var o Oneliner
		var posted int64
		if err := rows.Scan(&o.ID, &o.Author, &o.Text, &posted); err != nil {
			return nil, fmt.Errorf("store: oneliners scan: %w", err)
		}
		o.Posted = fromUnix(posted)
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) AddOneliner(o *Oneliner) error {
	o.Author = sanitize.Line(o.Author)
	o.Text = sanitize.Line(o.Text)
	posted := o.Posted
	if posted.IsZero() {
		posted = time.Now()
	}
	res, err := s.db.Exec(
		`INSERT INTO oneliners (author, text, posted) VALUES (?, ?, ?)`,
		o.Author, o.Text, toUnix(posted))
	if err != nil {
		return fmt.Errorf("store: add oneliner: %w", err)
	}
	if id, err := res.LastInsertId(); err == nil {
		o.ID = id
	}
	o.Posted = posted
	return nil
}

// ---- account maintenance ---------------------------------------------------

// SetPassword stores a (pre-hashed) password for the user. The store never
// sees plaintext; hashing lives in internal/auth.
func (s *Store) SetPassword(id int64, hash string) error {
	if _, err := s.db.Exec(`UPDATE users SET password = ? WHERE id = ?`, hash, id); err != nil {
		return fmt.Errorf("store: set password: %w", err)
	}
	return nil
}

// SetLevels updates a user's security and download-security levels (a sysop
// action). Values are clamped to a sane 0..255 range by the caller.
func (s *Store) SetLevels(id int64, sl, dsl int) error {
	if _, err := s.db.Exec(`UPDATE users SET sl = ?, dsl = ? WHERE id = ?`, sl, dsl, id); err != nil {
		return fmt.Errorf("store: set levels: %w", err)
	}
	return nil
}

// UpdateProfile updates a user's self-editable fields (the Settings screen).
func (s *Store) UpdateProfile(id int64, realName, email, location, tagline string) error {
	realName, email = sanitize.Line(realName), sanitize.Line(email)
	location, tagline = sanitize.Line(location), sanitize.Line(tagline)
	if _, err := s.db.Exec(
		`UPDATE users SET real_name = ?, email = ?, location = ?, tagline = ? WHERE id = ?`,
		realName, email, location, tagline, id); err != nil {
		return fmt.Errorf("store: update profile: %w", err)
	}
	return nil
}

// UserByID returns the user with the given id, or nil,nil if not found.
func (s *Store) UserByID(id int64) (*User, error) {
	row := s.db.QueryRow(`SELECT `+userCols+` FROM users WHERE id = ?`, id)
	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: user by id %d: %w", id, err)
	}
	return u, nil
}

// RecordLogin bumps the call counter and stamps last_call to now.
func (s *Store) RecordLogin(id int64) error {
	if _, err := s.db.Exec(
		`UPDATE users SET calls = calls + 1, last_call = ? WHERE id = ?`,
		toUnix(time.Now()), id); err != nil {
		return fmt.Errorf("store: record login: %w", err)
	}
	return nil
}

// IncPosts bumps a user's post counter (called when they post a message).
func (s *Store) IncPosts(handle string) error {
	if _, err := s.db.Exec(
		`UPDATE users SET posts = posts + 1 WHERE handle = ? COLLATE NOCASE`, handle); err != nil {
		return fmt.Errorf("store: inc posts: %w", err)
	}
	return nil
}
