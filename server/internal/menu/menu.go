// Package menu is the sysop-configurable main menu: which command lives in
// each on-screen slot, what it's labeled, and what key presses it. The
// catalog of bindable actions is data (mirroring internal/schedule's
// Catalog); the handlers themselves live in package main's dispatch table,
// kept in lockstep with this catalog by a test (server_menu_test.go).
package menu

import (
	"database/sql"
	"fmt"
	"strings"

	"vendetta-x/server/internal/sanitize"
)

// ActionDef is one command a main-menu slot can be bound to.
type ActionDef struct {
	Key   string // stable id, e.g. "messages" -- what a slot's Action stores
	Label string // the default/dropdown label, e.g. "Message Bases"
	// Feature is the sysop settings toggle that gates this command ("" means
	// it's core and always available regardless of which slot it's bound to).
	Feature string
}

// Catalog is the fixed set of actions a main-menu slot can be pointed at.
// Both the sysop panel (the action dropdown) and package main's dispatch
// table are built from this list, so the two can never drift out of sync.
var Catalog = []ActionDef{
	{Key: "messages", Label: "Message Bases"},
	{Key: "files", Label: "File Areas"},
	{Key: "email", Label: "Email", Feature: "email"},
	{Key: "oneliners", Label: "Oneliners", Feature: "oneliners"},
	{Key: "whoson", Label: "Who's Online"},
	{Key: "teleconference", Label: "Teleconference", Feature: "teleconference"},
	{Key: "page", Label: "Page Sysop", Feature: "paging"},
	{Key: "doors", Label: "Doors", Feature: "doors"},
	{Key: "qwk", Label: "QWK Mail", Feature: "qwk"},
	{Key: "newfiles", Label: "New Files", Feature: "newfiles"},
	{Key: "gfiles", Label: "G-Files", Feature: "gfiles"},
	{Key: "bbslist", Label: "BBS List", Feature: "bbslist"},
	{Key: "voting", Label: "Voting Booth", Feature: "voting"},
	{Key: "lastcallers", Label: "Last Callers"},
	{Key: "userlist", Label: "User List"},
	{Key: "stats", Label: "Your Stats"},
	{Key: "sysinfo", Label: "System Info"},
	{Key: "settings", Label: "Settings"},
	{Key: "goodbye", Label: "Goodbye"},
}

// MainMenuSlots is the fixed, ordered list of slots the main menu binds
// commands to: 10 in the left column, 9 in the right, matching the physical
// layout reserved in art/mainmenu.pp. Package main maps each id to its
// on-screen row/col (server_menu.go's mainMenuSlotPos); this package only
// needs the ids themselves to store and edit bindings against.
var MainMenuSlots = []string{
	"L0", "L1", "L2", "L3", "L4", "L5", "L6", "L7", "L8", "L9",
	"R0", "R1", "R2", "R3", "R4", "R5", "R6", "R7", "R8",
}

// Item is one slot's binding: which action it runs, what it's labeled, and
// what key presses it. Action == "" means the slot is blank (skipped when
// the menu paints).
type Item struct {
	Menu    string
	Slot    string
	Action  string
	Label   string
	Hotkey  string
	Enabled bool
}

type Store struct{ db *sql.DB }

func New(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	const schema = `CREATE TABLE IF NOT EXISTS menu_items (
		menu    TEXT NOT NULL,
		slot    TEXT NOT NULL,
		action  TEXT NOT NULL DEFAULT '',
		label   TEXT NOT NULL DEFAULT '',
		hotkey  TEXT NOT NULL DEFAULT '',
		enabled INTEGER NOT NULL DEFAULT 1,
		PRIMARY KEY (menu, slot)
	)`
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("menu: create table: %w", err)
	}
	return s, nil
}

// Items returns menuName's bindings keyed by slot. A nil Store (a harness
// that never constructed one) returns an empty map rather than panicking,
// so callers can uniformly fall back to defaults for any missing slot.
func (s *Store) Items(menuName string) (map[string]Item, error) {
	if s == nil {
		return nil, nil
	}
	rows, err := s.db.Query(
		`SELECT slot, action, label, hotkey, enabled FROM menu_items WHERE menu = ?`, menuName)
	if err != nil {
		return nil, fmt.Errorf("menu: items: %w", err)
	}
	defer rows.Close()
	out := map[string]Item{}
	for rows.Next() {
		it := Item{Menu: menuName}
		if err := rows.Scan(&it.Slot, &it.Action, &it.Label, &it.Hotkey, &it.Enabled); err != nil {
			return nil, fmt.Errorf("menu: items scan: %w", err)
		}
		out[it.Slot] = it
	}
	return out, rows.Err()
}

// EnsureSeeded inserts defaults for menuName only if it has no rows yet, so
// an upgrade (or a schema that predates this feature) never clobbers a
// sysop's customization with the stock layout.
func (s *Store) EnsureSeeded(menuName string, defaults []Item) error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM menu_items WHERE menu = ?`, menuName).Scan(&n); err != nil {
		return fmt.Errorf("menu: seed count: %w", err)
	}
	if n > 0 {
		return nil
	}
	return s.Save(menuName, defaults)
}

// Save replaces every binding for menuName with items (a full CRUD replace,
// like internal/ftn's SetEchoes): the sysop panel always submits the whole
// fixed slot set at once, so there's nothing partial to merge.
//
// '|' and '}' are stripped from labels/hotkeys before saving: both have
// special meaning to the pipe-code renderer (a stray '|' starts a new escape
// code, '}' ends a lightbar marker early), and sysop-entered text is
// otherwise-untrusted input to that renderer. A bound slot (Action != "")
// needs a non-empty label and an exactly-one-character hotkey; enabled
// slots' hotkeys must be unique (a duplicate would make one unreachable by
// key and is almost certainly a sysop typo, not intent), checked
// case-insensitively since the terminal matches hotkeys that way too.
func (s *Store) Save(menuName string, items []Item) error {
	seen := map[string]string{} // uppercased hotkey -> slot, for the dupe check
	clean := make([]Item, len(items))
	for i, it := range items {
		it.Menu = menuName
		it.Action = strings.TrimSpace(it.Action)
		it.Label = stripMarkerBreakers(sanitize.Line(it.Label))
		it.Hotkey = stripMarkerBreakers(sanitize.Line(it.Hotkey))
		if len(it.Label) > 34 {
			it.Label = it.Label[:34]
		}
		if it.Action == "" {
			it.Label, it.Hotkey, it.Enabled = "", "", false
		} else {
			if it.Label == "" {
				return fmt.Errorf("menu: slot %s needs a label", it.Slot)
			}
			if len([]rune(it.Hotkey)) != 1 {
				return fmt.Errorf("menu: slot %s needs a single-character hotkey", it.Slot)
			}
			if it.Enabled {
				k := strings.ToUpper(it.Hotkey)
				if other, dup := seen[k]; dup {
					return fmt.Errorf("menu: hotkey %q is used by both %s and %s", it.Hotkey, other, it.Slot)
				}
				seen[k] = it.Slot
			}
		}
		clean[i] = it
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("menu: save: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM menu_items WHERE menu = ?`, menuName); err != nil {
		return fmt.Errorf("menu: save clear: %w", err)
	}
	for _, it := range clean {
		if _, err := tx.Exec(
			`INSERT INTO menu_items (menu, slot, action, label, hotkey, enabled) VALUES (?, ?, ?, ?, ?, ?)`,
			it.Menu, it.Slot, it.Action, it.Label, it.Hotkey, it.Enabled); err != nil {
			return fmt.Errorf("menu: save insert %s: %w", it.Slot, err)
		}
	}
	return tx.Commit()
}

func stripMarkerBreakers(s string) string {
	return strings.NewReplacer("|", "", "}", "").Replace(s)
}
