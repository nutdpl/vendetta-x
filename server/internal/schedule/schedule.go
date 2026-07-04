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
	{
		Key:   "qwknet.exchange",
		Label: "QWK-net exchange",
		Desc:  "Uploads new local messages to the QWK network hub and imports the network's new messages. Configure the hub under sysop / qwk-net first.",
	},
	{
		Key:   "db.backup",
		Label: "Database backup",
		Desc:  "Writes a consistent snapshot of the whole board (one SQLite file) into the backup directory and prunes old snapshots. The board ships with this scheduled nightly.",
	},
}

// Event is one scheduled action, in one of two modes: run Action once a day
// at TimeOfDay (HH:MM, 24-hour, server local time), or -- when Interval is
// set -- run it every Interval minutes (mail-network polls and other
// keep-fresh work). Interval takes precedence over TimeOfDay.
type Event struct {
	ID        int64
	Name      string
	Action    string
	TimeOfDay string
	// Interval, in minutes, switches the event from daily-at-a-time to
	// every-N-minutes. 0 means daily mode.
	Interval int
	Enabled  bool
	LastRun  time.Time
}

// DueAt reports whether e should run at instant now. Interval mode: due when
// at least Interval minutes have passed since LastRun (a never-run event is
// due immediately). Daily mode: due once past today's TimeOfDay if it hasn't
// already run since that moment.
func (e Event) DueAt(now time.Time) bool {
	if !e.Enabled {
		return false
	}
	if e.Interval > 0 {
		return !now.Before(e.LastRun.Add(time.Duration(e.Interval) * time.Minute))
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
		last_run    INTEGER NOT NULL DEFAULT 0,
		interval_min INTEGER NOT NULL DEFAULT 0
	);`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	// Idempotent upgrade for tables created before interval events existed
	// (SQLite has no ADD COLUMN IF NOT EXISTS).
	_, err := s.db.Exec(`ALTER TABLE schedule_events ADD COLUMN interval_min INTEGER NOT NULL DEFAULT 0`)
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	return nil
}

// List returns every scheduled event, alphabetical by name.
func (s *Store) List() ([]Event, error) {
	rows, err := s.db.Query(
		`SELECT id, name, action, time_of_day, enabled, last_run, interval_min
		 FROM schedule_events ORDER BY name, id`)
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
		`SELECT id, name, action, time_of_day, enabled, last_run, interval_min
		 FROM schedule_events WHERE id = ?`, id)
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
	if err := r.Scan(&e.ID, &e.Name, &e.Action, &e.TimeOfDay, &enabled, &lastRun, &e.Interval); err != nil {
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
		`INSERT INTO schedule_events (name, action, time_of_day, enabled, last_run, interval_min)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		sanitize.Line(e.Name), sanitize.Line(e.Action), sanitize.Line(e.TimeOfDay),
		boolInt(e.Enabled), 0, max(e.Interval, 0))
	if err != nil {
		return 0, fmt.Errorf("schedule: add event: %w", err)
	}
	return res.LastInsertId()
}

// Update writes the editable fields (name, action, time of day, interval,
// enabled) of an existing event, keyed by e.ID. LastRun is left untouched --
// use MarkRun.
func (s *Store) Update(e *Event) error {
	_, err := s.db.Exec(
		`UPDATE schedule_events SET name = ?, action = ?, time_of_day = ?, enabled = ?, interval_min = ?
		 WHERE id = ?`,
		sanitize.Line(e.Name), sanitize.Line(e.Action), sanitize.Line(e.TimeOfDay),
		boolInt(e.Enabled), max(e.Interval, 0), e.ID)
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
