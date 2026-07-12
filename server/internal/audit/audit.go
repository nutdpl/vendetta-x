// Package audit is the board's durable record of every sysop mutation: who
// changed what, from where, and when. The web admin choke point writes one row
// per state-changing request through here, and the sysop reads them back on a
// filterable page. It's a thin table over the shared DB, mirroring the
// feature-package pattern the rest of the board follows.
package audit

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"vendetta-x/server/internal/sanitize"
)

// Entry is one recorded action.
type Entry struct {
	ID     int64
	At     time.Time
	Actor  string // the handle that did it
	Method string // HTTP method (POST/DELETE/...)
	Path   string // the route touched
	IP     string // source address
}

type Store struct{ db *sql.DB }

// New creates the audit table if absent. Idempotent.
func New(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS audit_log (
		id     INTEGER PRIMARY KEY AUTOINCREMENT,
		at     INTEGER NOT NULL DEFAULT 0,
		actor  TEXT NOT NULL DEFAULT '',
		method TEXT NOT NULL DEFAULT '',
		path   TEXT NOT NULL DEFAULT '',
		ip     TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		return nil, fmt.Errorf("audit: create table: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_audit_at ON audit_log (at DESC)`); err != nil {
		return nil, fmt.Errorf("audit: create index: %w", err)
	}
	return s, nil
}

// Record appends one action. User text is sanitized at the boundary. A record
// failure is the caller's to log; it never blocks the mutation it describes.
func (s *Store) Record(actor, method, path, ip string) error {
	_, err := s.db.Exec(
		`INSERT INTO audit_log (at, actor, method, path, ip) VALUES (?, ?, ?, ?, ?)`,
		time.Now().Unix(), sanitize.Line(actor), sanitize.Line(method),
		sanitize.Line(path), sanitize.Line(ip))
	if err != nil {
		return fmt.Errorf("audit: record: %w", err)
	}
	return nil
}

// Recent returns the newest entries, most-recent first, capped at limit. When
// actor is non-empty, only that actor's actions are returned (case-insensitive
// substring), the filter the sysop page exposes.
func (s *Store) Recent(limit int, actor string) ([]Entry, error) {
	q := `SELECT id, at, actor, method, path, ip FROM audit_log`
	var args []any
	if a := strings.TrimSpace(actor); a != "" {
		q += ` WHERE actor LIKE ? ESCAPE '\'`
		args = append(args, "%"+likeEscape(a)+"%")
	}
	q += ` ORDER BY at DESC, id DESC`
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("audit: recent: %w", err)
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		var at int64
		if err := rows.Scan(&e.ID, &at, &e.Actor, &e.Method, &e.Path, &e.IP); err != nil {
			return nil, fmt.Errorf("audit: scan: %w", err)
		}
		e.At = time.Unix(at, 0)
		out = append(out, e)
	}
	return out, rows.Err()
}

// Count returns the total number of recorded actions.
func (s *Store) Count() (int, error) {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM audit_log`).Scan(&n); err != nil {
		return 0, fmt.Errorf("audit: count: %w", err)
	}
	return n, nil
}

// likeEscape escapes a filter string for a LIKE with `ESCAPE '\'`.
func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
