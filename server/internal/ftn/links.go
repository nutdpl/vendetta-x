package ftn

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"vendetta-x/server/internal/sanitize"
)

// Store owns the FTN networking tables: the uplinks (one row per network
// the board has joined -- fsxNet, FidoNet, AgoraNet, ... each with its own
// address and password), the echo<->board map per link, and the seen-MSGID
// ledger that makes duplicates and loops impossible.
type Store struct{ db *sql.DB }

// Link is one network uplink.
type Link struct {
	ID   int64
	Name string // network tag, e.g. "fsxNet"; stamps imported messages' Origin
	Host string // hub host[:port], port 24554 assumed
	// OurAddr is the node address the network assigned us; HubAddr is the
	// uplink we poll. Both full 4D strings ("21:1/999").
	OurAddr  string
	HubAddr  string
	Password string
	Enabled  bool
}

// Echo maps one echo tag on a link to one local board.
type Echo struct {
	ID       int64
	LinkID   int64
	Tag      string // echo tag, e.g. FSX_GEN
	BoardTag string // local board tag, e.g. gen
	// LastExport is the export high-water mark: the largest local message id
	// already sent to this echo, advanced only after the hub accepts.
	LastExport int64
}

func NewStore(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	const schema = `
CREATE TABLE IF NOT EXISTS ftn_links (
	id       INTEGER PRIMARY KEY AUTOINCREMENT,
	name     TEXT NOT NULL,
	host     TEXT NOT NULL DEFAULT '',
	our_addr TEXT NOT NULL DEFAULT '',
	hub_addr TEXT NOT NULL DEFAULT '',
	password TEXT NOT NULL DEFAULT '',
	enabled  INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE IF NOT EXISTS ftn_echoes (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	link_id     INTEGER NOT NULL,
	tag         TEXT NOT NULL,
	board_tag   TEXT NOT NULL,
	last_export INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS ftn_seen (
	msgid  TEXT PRIMARY KEY,
	msg_id INTEGER NOT NULL DEFAULT 0,
	at     INTEGER NOT NULL DEFAULT 0
);`
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("ftn: create tables: %w", err)
	}
	return s, nil
}

func (s *Store) Links() ([]Link, error) {
	rows, err := s.db.Query(
		`SELECT id, name, host, our_addr, hub_addr, password, enabled
		 FROM ftn_links ORDER BY name, id`)
	if err != nil {
		return nil, fmt.Errorf("ftn: links: %w", err)
	}
	defer rows.Close()
	var out []Link
	for rows.Next() {
		var l Link
		if err := rows.Scan(&l.ID, &l.Name, &l.Host, &l.OurAddr, &l.HubAddr, &l.Password, &l.Enabled); err != nil {
			return nil, fmt.Errorf("ftn: links scan: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) LinkByID(id int64) (*Link, error) {
	links, err := s.Links()
	if err != nil {
		return nil, err
	}
	for i := range links {
		if links[i].ID == id {
			return &links[i], nil
		}
	}
	return nil, nil
}

// SaveLink inserts (ID==0) or updates a link after validating addresses.
func (s *Store) SaveLink(l *Link) error {
	l.Name = strings.TrimSpace(sanitize.Line(l.Name))
	l.Host = strings.TrimSpace(sanitize.Line(l.Host))
	l.Password = sanitize.Line(l.Password)
	if l.Name == "" || l.Host == "" {
		return fmt.Errorf("ftn: a link needs a name and a hub host")
	}
	if _, err := ParseAddress(l.OurAddr); err != nil {
		return fmt.Errorf("ftn: our address: %w", err)
	}
	if _, err := ParseAddress(l.HubAddr); err != nil {
		return fmt.Errorf("ftn: hub address: %w", err)
	}
	if l.ID == 0 {
		res, err := s.db.Exec(
			`INSERT INTO ftn_links (name, host, our_addr, hub_addr, password, enabled)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			l.Name, l.Host, l.OurAddr, l.HubAddr, l.Password, l.Enabled)
		if err != nil {
			return fmt.Errorf("ftn: add link: %w", err)
		}
		l.ID, _ = res.LastInsertId()
		return nil
	}
	_, err := s.db.Exec(
		`UPDATE ftn_links SET name=?, host=?, our_addr=?, hub_addr=?, password=?, enabled=?
		 WHERE id=?`,
		l.Name, l.Host, l.OurAddr, l.HubAddr, l.Password, l.Enabled, l.ID)
	if err != nil {
		return fmt.Errorf("ftn: update link: %w", err)
	}
	return nil
}

// DeleteLink removes a link and its echo map (the seen ledger stays: those
// messages remain imported and must stay deduped if the link comes back).
func (s *Store) DeleteLink(id int64) error {
	if _, err := s.db.Exec(`DELETE FROM ftn_echoes WHERE link_id=?`, id); err != nil {
		return fmt.Errorf("ftn: delete echoes: %w", err)
	}
	if _, err := s.db.Exec(`DELETE FROM ftn_links WHERE id=?`, id); err != nil {
		return fmt.Errorf("ftn: delete link: %w", err)
	}
	return nil
}

func (s *Store) Echoes(linkID int64) ([]Echo, error) {
	rows, err := s.db.Query(
		`SELECT id, link_id, tag, board_tag, last_export
		 FROM ftn_echoes WHERE link_id=? ORDER BY tag`, linkID)
	if err != nil {
		return nil, fmt.Errorf("ftn: echoes: %w", err)
	}
	defer rows.Close()
	var out []Echo
	for rows.Next() {
		var e Echo
		if err := rows.Scan(&e.ID, &e.LinkID, &e.Tag, &e.BoardTag, &e.LastExport); err != nil {
			return nil, fmt.Errorf("ftn: echoes scan: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// SetEchoes replaces a link's echo map with the given tag->board pairs,
// preserving each surviving echo's export high-water mark.
func (s *Store) SetEchoes(linkID int64, pairs map[string]string) error {
	old, err := s.Echoes(linkID)
	if err != nil {
		return err
	}
	marks := map[string]int64{}
	for _, e := range old {
		marks[e.Tag] = e.LastExport
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("ftn: set echoes: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM ftn_echoes WHERE link_id=?`, linkID); err != nil {
		return fmt.Errorf("ftn: set echoes clear: %w", err)
	}
	for tag, board := range pairs {
		tag = strings.ToUpper(strings.TrimSpace(sanitize.Line(tag)))
		board = strings.TrimSpace(sanitize.Line(board))
		if tag == "" || board == "" {
			continue
		}
		if _, err := tx.Exec(
			`INSERT INTO ftn_echoes (link_id, tag, board_tag, last_export) VALUES (?, ?, ?, ?)`,
			linkID, tag, board, marks[tag]); err != nil {
			return fmt.Errorf("ftn: set echoes insert: %w", err)
		}
	}
	return tx.Commit()
}

// AdvanceExport lifts an echo's high-water mark (monotonic).
func (s *Store) AdvanceExport(echoID, msgID int64) error {
	_, err := s.db.Exec(
		`UPDATE ftn_echoes SET last_export = MAX(last_export, ?) WHERE id = ?`, msgID, echoID)
	if err != nil {
		return fmt.Errorf("ftn: advance export: %w", err)
	}
	return nil
}

// Seen looks a network MSGID up in the dedupe ledger; the second return is
// the local message id it was stored under (0 when unknown), which threads
// REPLY kludges back onto local messages.
func (s *Store) Seen(msgid string) (bool, int64, error) {
	if msgid == "" {
		return false, 0, nil
	}
	var localID int64
	err := s.db.QueryRow(`SELECT msg_id FROM ftn_seen WHERE msgid=?`, msgid).Scan(&localID)
	if err == sql.ErrNoRows {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, fmt.Errorf("ftn: seen: %w", err)
	}
	return true, localID, nil
}

// MarkSeen records a MSGID and the local message row that carries it.
func (s *Store) MarkSeen(msgid string, localID int64) error {
	if msgid == "" {
		return nil
	}
	_, err := s.db.Exec(
		`INSERT INTO ftn_seen (msgid, msg_id, at) VALUES (?, ?, ?)
		 ON CONFLICT(msgid) DO NOTHING`, msgid, localID, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("ftn: mark seen: %w", err)
	}
	return nil
}
