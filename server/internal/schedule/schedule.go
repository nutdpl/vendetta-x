// Package schedule is Vendetta/X's event scheduler: sysop-configured, timed
// maintenance actions (message purges, wall trims, and whatever else gets
// added to the Catalog below) that fire once a day at a chosen time,
// unattended, with no caller session driving them. It owns its own SQLite
// table over the shared database handle, matching the shape of the other
// small feature stores (bulletin, gfiles, ...).
//
// This package holds the data layer and the shared catalog of known action
// keys/labels only. The actual dispatch (mapping an action key to the Go
// closure that performs it) lives in package main (server_schedule.go),
// which is the only place with access to the board's other stores.
package schedule

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"vendetta-x/server/internal/sanitize"
)

// ActionDef describes one action a scheduled event can run: a stable key
// (stored in the events table and matched against package main's dispatch
// table), a short label for the sysop panel, and a one-line description of
// what it does.
type ActionDef struct {
	Key, Label, Desc string
}

// Catalog is the fixed set of actions a scheduled event can be pointed at.
// Both the sysop web panel (for the action dropdown) and package main's
// dispatch table (for the actual implementation) are built from this list,
// so the two can never drift out of sync on available keys.
var Catalog = []ActionDef{
	{
		Key:   "messages.purge_old",
		Label: "Purge old messages",
		Desc:  "Deletes messages older than the configured retention window from every message board.",
	},
	{
		Key:   "oneliners.trim",
		Label: "Trim the oneliner wall",
		Desc:  "Keeps only the most recent oneliners, deleting the rest.",
	},
}

// Event is one scheduled action: run Action once a day at TimeOfDay
// (HH:MM, 24-hour, server local time) when Enabled.
type Event struct {
	ID        int64
	Name      string
	Action    string
	TimeOfDay string
	Enabled   bool
	LastRun   time.Time
}

// DueAt reports whether e should run at instant now: enabled, past today's
// TimeOfDay, and not already run since that moment today. A never-run event
// (LastRun zero) is always due once its time has passed.
func (e Event) DueAt(now time.Time) bool {
	if !e.Enabled {
		return false
	}
	hh, mm, ok := parseTimeOfDay(e.TimeOfDay)
	if !ok {
		return false
	}
	due := time.Date(now.Year(), now.Month(), now.Day(), hh, mm, 0, 0, now.Location())
	if now.Before(due) {
		return false
	}
	return e.LastRun.Before(due)
}

// parseTimeOfDay parses a "HH:MM" 24-hour string.
func parseTimeOfDay(s string) (hh, mm int, ok bool) {
	h, m, found := strings.Cut(s, ":")
	if !found {
		return 0, 0, false
	}
	hh, err := strconv.Atoi(h)
	if err != nil || hh < 0 || hh > 23 {
		return 0, 0, false
	}
	mm, err = strconv.Atoi(m)
	if err != nil || mm < 0 || mm > 59 {
		return 0, 0, false
	}
	return hh, mm, true
}

// Store is the scheduled-events data layer over the shared *sql.DB.
type Store struct {
	db *sql.DB
}

// New returns a Store, creating the events table if absent.
func New(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	const schema = `CREATE TABLE IF NOT EXISTS schedule_events (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		name        TEXT NOT NULL DEFAULT '',
		action      TEXT NOT NULL DEFAULT '',
		time_of_day TEXT NOT NULL DEFAULT '',
		enabled     INTEGER NOT NULL DEFAULT 1,
		last_run    INTEGER NOT NULL DEFAULT 0
	);`
	_, err := s.db.Exec(schema)
	return err
}

// List returns every scheduled event, alphabetical by name.
func (s *Store) List() ([]Event, error) {
	rows, err := s.db.Query(
		`SELECT id, name, action, time_of_day, enabled, last_run FROM schedule_events ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Get returns one event by id, or nil,nil if not found.
func (s *Store) Get(id int64) (*Event, error) {
	row := s.db.QueryRow(
		`SELECT id, name, action, time_of_day, enabled, last_run FROM schedule_events WHERE id = ?`, id)
	e, err := scanEvent(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanEvent(r rowScanner) (Event, error) {
	var (
		e       Event
		enabled int
		lastRun int64
	)
	if err := r.Scan(&e.ID, &e.Name, &e.Action, &e.TimeOfDay, &enabled, &lastRun); err != nil {
		return Event{}, err
	}
	e.Enabled = enabled != 0
	if lastRun != 0 {
		e.LastRun = time.Unix(lastRun, 0)
	}
	return e, nil
}

// Add inserts a new scheduled event, returning its id.
func (s *Store) Add(e *Event) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO schedule_events (name, action, time_of_day, enabled, last_run) VALUES (?, ?, ?, ?, ?)`,
		sanitize.Line(e.Name), sanitize.Line(e.Action), sanitize.Line(e.TimeOfDay), boolInt(e.Enabled), 0)
	if err != nil {
		return 0, fmt.Errorf("schedule: add event: %w", err)
	}
	return res.LastInsertId()
}

// Update writes the editable fields (name, action, time of day, enabled) of
// an existing event, keyed by e.ID. LastRun is left untouched -- use MarkRun.
func (s *Store) Update(e *Event) error {
	_, err := s.db.Exec(
		`UPDATE schedule_events SET name = ?, action = ?, time_of_day = ?, enabled = ? WHERE id = ?`,
		sanitize.Line(e.Name), sanitize.Line(e.Action), sanitize.Line(e.TimeOfDay), boolInt(e.Enabled), e.ID)
	if err != nil {
		return fmt.Errorf("schedule: update event %d: %w", e.ID, err)
	}
	return nil
}

// Delete removes a scheduled event by id.
func (s *Store) Delete(id int64) error {
	_, err := s.db.Exec(`DELETE FROM schedule_events WHERE id = ?`, id)
	return err
}

// MarkRun stamps an event's last-run time, so DueAt won't fire it again until
// tomorrow's occurrence of its TimeOfDay.
func (s *Store) MarkRun(id int64, when time.Time) error {
	_, err := s.db.Exec(`UPDATE schedule_events SET last_run = ? WHERE id = ?`, when.Unix(), id)
	return err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
