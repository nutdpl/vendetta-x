// Package guard is the board's door policy: durable bans the sysop lays
// down when the throttle isn't enough. Three kinds -- a single IP, a CIDR
// range, and a handle pattern (the trashcan: substrings no new handle may
// contain) -- each with a reason and an optional expiry. Enforced before
// the connect ceremony on telnet/ssh, at login/register on the web, and at
// signup on every face. The loopback console is never blocked, so a sysop
// can't lock themselves out of their own machine.
package guard

import (
	"database/sql"
	"fmt"
	"net"
	"strings"
	"time"

	"vendetta-x/server/internal/sanitize"
)

const (
	KindIP     = "ip"
	KindCIDR   = "cidr"
	KindHandle = "handle"
)

type Ban struct {
	ID      int64
	Kind    string // ip | cidr | handle
	Value   string
	Reason  string
	Created time.Time
	// Expires is the zero time for a permanent ban.
	Expires time.Time
}

type Store struct{ db *sql.DB }

func New(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS bans (
		id      INTEGER PRIMARY KEY AUTOINCREMENT,
		kind    TEXT NOT NULL,
		value   TEXT NOT NULL,
		reason  TEXT NOT NULL DEFAULT '',
		created INTEGER NOT NULL DEFAULT 0,
		expires INTEGER NOT NULL DEFAULT 0
	)`)
	if err != nil {
		return nil, fmt.Errorf("guard: create table: %w", err)
	}
	return s, nil
}

// Add validates and records a ban. days == 0 means permanent.
func (s *Store) Add(kind, value, reason string, days int) error {
	value = strings.TrimSpace(sanitize.Line(value))
	switch kind {
	case KindIP:
		if net.ParseIP(value) == nil {
			return fmt.Errorf("guard: %q is not an IP address", value)
		}
	case KindCIDR:
		if _, _, err := net.ParseCIDR(value); err != nil {
			return fmt.Errorf("guard: %q is not a CIDR range", value)
		}
	case KindHandle:
		value = strings.ToLower(value)
		if len(value) < 2 {
			return fmt.Errorf("guard: handle pattern too short")
		}
	default:
		return fmt.Errorf("guard: unknown ban kind %q", kind)
	}
	var expires int64
	if days > 0 {
		expires = time.Now().AddDate(0, 0, days).Unix()
	}
	_, err := s.db.Exec(
		`INSERT INTO bans (kind, value, reason, created, expires) VALUES (?, ?, ?, ?, ?)`,
		kind, value, sanitize.Line(reason), time.Now().Unix(), expires)
	if err != nil {
		return fmt.Errorf("guard: add ban: %w", err)
	}
	return nil
}

func (s *Store) Delete(id int64) error {
	if _, err := s.db.Exec(`DELETE FROM bans WHERE id = ?`, id); err != nil {
		return fmt.Errorf("guard: delete ban: %w", err)
	}
	return nil
}

// List returns every ban, newest first, expired ones included (they read as
// history until the sysop deletes them; enforcement ignores them).
func (s *Store) List() ([]Ban, error) {
	rows, err := s.db.Query(
		`SELECT id, kind, value, reason, created, expires FROM bans ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("guard: list bans: %w", err)
	}
	defer rows.Close()
	var out []Ban
	for rows.Next() {
		var b Ban
		var created, expires int64
		if err := rows.Scan(&b.ID, &b.Kind, &b.Value, &b.Reason, &created, &expires); err != nil {
			return nil, fmt.Errorf("guard: scan ban: %w", err)
		}
		b.Created = time.Unix(created, 0)
		if expires > 0 {
			b.Expires = time.Unix(expires, 0)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// BlockedIP reports whether host (an IP string, no port) is banned, and why.
// Loopback is never blocked: the console must always be able to get in.
// A nil store blocks nothing (harnesses that never construct one stay open).
func (s *Store) BlockedIP(host string) (string, bool) {
	if s == nil {
		return "", false
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsLoopback() {
		return "", false
	}
	for _, b := range s.active() {
		switch b.Kind {
		case KindIP:
			if banned := net.ParseIP(b.Value); banned != nil && banned.Equal(ip) {
				return b.Reason, true
			}
		case KindCIDR:
			if _, cidr, err := net.ParseCIDR(b.Value); err == nil && cidr.Contains(ip) {
				return b.Reason, true
			}
		}
	}
	return "", false
}

// BlockedHandle reports whether a requested handle hits the trashcan (a
// banned substring, case-insensitive), and why.
func (s *Store) BlockedHandle(handle string) (string, bool) {
	if s == nil {
		return "", false
	}
	h := strings.ToLower(strings.TrimSpace(handle))
	for _, b := range s.active() {
		if b.Kind == KindHandle && strings.Contains(h, b.Value) {
			return b.Reason, true
		}
	}
	return "", false
}

// active returns the enforceable bans (unexpired). The table is tiny --
// reading it per check keeps every node and face instantly consistent.
func (s *Store) active() []Ban {
	bans, err := s.List()
	if err != nil {
		return nil
	}
	now := time.Now()
	out := bans[:0]
	for _, b := range bans {
		if b.Expires.IsZero() || b.Expires.After(now) {
			out = append(out, b)
		}
	}
	return out
}
