package store

import (
	"fmt"
	"strings"

	"vendetta-x/server/internal/sanitize"
)

// The twit list (a.k.a. ignore list): per user, a set of handles whose posts
// and wall lines the board hides from them. A classic caller-side moderation
// tool -- you don't see someone, without anyone else's view changing.

func (s *Store) migrateTwits() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS twits (
		user_id INTEGER NOT NULL,
		handle  TEXT NOT NULL,
		PRIMARY KEY (user_id, handle COLLATE NOCASE)
	)`)
	if err != nil {
		return fmt.Errorf("store: create twits: %w", err)
	}
	return nil
}

// AddTwit adds handle to a user's ignore list (idempotent, case-insensitive).
// A blank handle, or a user ignoring themselves, is refused.
func (s *Store) AddTwit(userID int64, handle string) error {
	handle = strings.TrimSpace(sanitize.Line(handle))
	if handle == "" {
		return fmt.Errorf("store: empty twit handle")
	}
	_, err := s.db.Exec(
		`INSERT INTO twits (user_id, handle) VALUES (?, ?)
		 ON CONFLICT(user_id, handle COLLATE NOCASE) DO NOTHING`, userID, handle)
	if err != nil {
		return fmt.Errorf("store: add twit: %w", err)
	}
	return nil
}

// RemoveTwit drops handle from a user's ignore list (case-insensitive).
func (s *Store) RemoveTwit(userID int64, handle string) error {
	if _, err := s.db.Exec(
		`DELETE FROM twits WHERE user_id = ? AND handle = ? COLLATE NOCASE`,
		userID, strings.TrimSpace(handle)); err != nil {
		return fmt.Errorf("store: remove twit: %w", err)
	}
	return nil
}

// Twits returns a user's ignore list, sorted case-insensitively.
func (s *Store) Twits(userID int64) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT handle FROM twits WHERE user_id = ? ORDER BY handle COLLATE NOCASE`, userID)
	if err != nil {
		return nil, fmt.Errorf("store: twits: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, fmt.Errorf("store: twits scan: %w", err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// TwitSet returns a user's ignore list as a set of lower-cased handles, for
// cheap membership tests while filtering a listing.
func (s *Store) TwitSet(userID int64) (map[string]bool, error) {
	handles, err := s.Twits(userID)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(handles))
	for _, h := range handles {
		set[strings.ToLower(h)] = true
	}
	return set, nil
}
