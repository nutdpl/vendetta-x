package store

import (
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"vendetta-x/server/internal/sanitize"
)

// Global, sysop-editable configuration lives in the settings key/value table
// (created in migrate). Values are stored as strings; the typed helpers below
// parse them on the way out and fall back to a caller-supplied default when a
// key is unset or unparseable, so a fresh database needs no seeding.

// Setting returns the string value for key, or def when the key is unset.
func (s *Store) Setting(key, def string) string {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return def
	}
	if err != nil {
		return def
	}
	return v
}

// SettingBool returns the boolean value for key, or def when unset/unparseable.
// Stored as "1"/"0" (also accepts true/false via strconv).
func (s *Store) SettingBool(key string, def bool) bool {
	v := s.Setting(key, "")
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

// SettingInt returns the integer value for key, or def when unset/unparseable.
func (s *Store) SettingInt(key string, def int) int {
	v := s.Setting(key, "")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// SetSetting upserts a single key/value pair.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("store: set setting %q: %w", key, err)
	}
	return nil
}

// Feature is one toggleable board feature: a stable key, a label for the sysop
// config screen, and a short description. Disabling one hides it on every face.
type Feature struct {
	Key, Label, Desc string
}

// Features is the canonical registry of optionally-disableable menu features.
// Core navigation (message bases, file areas, user list, your stats, goodbye)
// is always on and intentionally absent here.
var Features = []Feature{
	{"email", "Email", "Private user-to-user mail"},
	{"voting", "Voting Booth", "Polls with one vote per user"},
	{"gfiles", "G-Files", "Text / info document library"},
	{"bbslist", "BBS List", "Directory of other boards"},
	{"doors", "Doors", "Native door games"},
	{"qwk", "QWK Mail", "Offline-mail summary + digest"},
	{"newfiles", "New Files", "Newest-uploads scan"},
	{"search", "Search", "Search messages and files across every base"},
	{"feeds", "Atom Feed", "Public /feed.atom of recent posts for the open web"},
	{"oneliners", "Oneliners", "The wall"},
	{"teleconference", "Teleconference", "Multi-node live chat"},
	{"paging", "Page Sysop", "Callers can page the operator (delivered as mail)"},
}

// ---- the automessage --------------------------------------------------------
//
// One board-wide announcement, claimable by any caller (WWIV heritage): the
// newest claim simply replaces the last. Settings-backed -- three keys, no
// table -- because there is only ever one.

// Automessage returns the current announcement (empty text = none set).
func (s *Store) Automessage() (author, text string, at time.Time) {
	author = s.Setting("automessage.author", "")
	text = s.Setting("automessage.text", "")
	if n := s.SettingInt("automessage.at", 0); n > 0 {
		at = time.Unix(int64(n), 0)
	}
	return author, text, at
}

// SetAutomessage claims the automessage for author (empty text clears it).
func (s *Store) SetAutomessage(author, text string) error {
	author = sanitize.Line(author)
	text = sanitize.Line(text)
	if err := s.SetSetting("automessage.author", author); err != nil {
		return err
	}
	if err := s.SetSetting("automessage.text", text); err != nil {
		return err
	}
	return s.SetSetting("automessage.at", strconv.FormatInt(time.Now().Unix(), 10))
}

// FeatureEnabled reports whether the named feature (by its registry Key) is
// switched on. Unknown/unset keys default to on, so the board ships fully open.
func (s *Store) FeatureEnabled(key string) bool {
	return s.SettingBool("feature."+key, true)
}

// Settings returns every stored key/value pair (for the sysop config screen).
func (s *Store) Settings() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return nil, fmt.Errorf("store: settings: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("store: settings scan: %w", err)
		}
		out[k] = v
	}
	return out, rows.Err()
}
