package main

import (
	"bytes"
	"fmt"
	"os"

	"vendetta-x/server/internal/menu"
	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/term"
)

// The sysop-configurable main menu: which command lives in each on-screen
// slot, its label, and the key that picks it -- editable from the sysop
// panel instead of baked into the art. The physical layout -- 10 slots in
// the left column, 9 in the right, at fixed rows/cols -- is still baked into
// art/mainmenu.pp (legacy/tools/mkmainmenu.py); what's no longer baked in is
// which action fills each slot. mainMenu() composes the current bindings
// into the art's @@MENU_OPTIONS@@ placeholder on every render.

// mainMenuSlotPos is where each slot paints -- must match the reserved
// layout in art/mainmenu.pp (base row 18, just under the divider; left
// column at col 8, right at col 44). If that art is ever regenerated with a
// different logo height or column count, this map and mkmainmenu.py's
// LEFT_SLOTS/RIGHT_SLOTS/LCOL/RCOL need to move in lockstep -- a test
// (TestMainMenuSlotsComplete) at least catches the two lists drifting apart
// from each other, though not from the art itself.
var mainMenuSlotPos = map[string]struct{ Row, Col int }{
	"L0": {18, 8}, "L1": {19, 8}, "L2": {20, 8}, "L3": {21, 8}, "L4": {22, 8},
	"L5": {23, 8}, "L6": {24, 8}, "L7": {25, 8}, "L8": {26, 8}, "L9": {27, 8},
	"R0": {18, 44}, "R1": {19, 44}, "R2": {20, 44}, "R3": {21, 44}, "R4": {22, 44},
	"R5": {23, 44}, "R6": {24, 44}, "R7": {25, 44}, "R8": {26, 44},
}

// mainMenuDefaults seeds a fresh board's main menu, matching exactly what
// shipped as static art before menu bindings became sysop-editable.
var mainMenuDefaults = []menu.Item{
	{Slot: "L0", Action: "messages", Label: "Message Bases", Hotkey: "M", Enabled: true},
	{Slot: "L1", Action: "files", Label: "File Areas", Hotkey: "F", Enabled: true},
	{Slot: "L2", Action: "email", Label: "Email", Hotkey: "E", Enabled: true},
	{Slot: "L3", Action: "oneliners", Label: "Oneliners", Hotkey: "O", Enabled: true},
	{Slot: "L4", Action: "whoson", Label: "Who's Online", Hotkey: "W", Enabled: true},
	{Slot: "L5", Action: "teleconference", Label: "Teleconference", Hotkey: "C", Enabled: true},
	{Slot: "L6", Action: "page", Label: "Page Sysop", Hotkey: "P", Enabled: true},
	{Slot: "L7", Action: "doors", Label: "Doors", Hotkey: "D", Enabled: true},
	{Slot: "L8", Action: "qwk", Label: "QWK Mail", Hotkey: "Q", Enabled: true},
	{Slot: "L9", Action: "newfiles", Label: "New Files", Hotkey: "N", Enabled: true},
	{Slot: "R0", Action: "gfiles", Label: "G-Files", Hotkey: "T", Enabled: true},
	{Slot: "R1", Action: "bbslist", Label: "BBS List", Hotkey: "B", Enabled: true},
	{Slot: "R2", Action: "voting", Label: "Voting Booth", Hotkey: "V", Enabled: true},
	{Slot: "R3", Action: "lastcallers", Label: "Last Callers", Hotkey: "L", Enabled: true},
	{Slot: "R4", Action: "userlist", Label: "User List", Hotkey: "U", Enabled: true},
	{Slot: "R5", Action: "stats", Label: "Your Stats", Hotkey: "Z", Enabled: true},
	{Slot: "R6", Action: "sysinfo", Label: "System Info", Hotkey: "I", Enabled: true},
	{Slot: "R7", Action: "settings", Label: "Settings", Hotkey: "X", Enabled: true},
	{Slot: "R8", Action: "goodbye", Label: "Goodbye", Hotkey: "G", Enabled: true},
}

// mainMenuHandler is one bindable action's runtime behavior: the feature
// toggle that gates it ("" = core, always on), the who's-online activity
// string shown while a caller is inside it ("" = leave it unchanged), and
// the handler itself. Run == nil is the "goodbye" sentinel: mainMenu returns
// instead of calling it.
type mainMenuHandler struct {
	Feature string
	Doing   string
	Run     func(b *board, s *term.Session, tok map[string]string, user *store.User)
}

// mainMenuDispatch implements every action in menu.Catalog. A test
// (TestMainMenuActionsMatchCatalog) keeps the two from drifting apart --
// mirrors scheduledActions/schedule.Catalog's established pattern.
var mainMenuDispatch = map[string]mainMenuHandler{
	"messages": {Doing: "in the message bases", Run: func(b *board, s *term.Session, tok map[string]string, user *store.User) {
		b.messageMenu(s, tok, user)
	}},
	"files": {Doing: "in the file areas", Run: func(b *board, s *term.Session, tok map[string]string, user *store.User) {
		b.fileMenu(s, tok, user)
	}},
	"email": {Feature: "email", Doing: "reading mail", Run: func(b *board, s *term.Session, tok map[string]string, user *store.User) {
		b.email(s, tok, user)
	}},
	"oneliners": {Feature: "oneliners", Doing: "at the wall", Run: func(b *board, s *term.Session, tok map[string]string, user *store.User) {
		b.oneliners(s, user)
	}},
	"whoson": {Run: func(b *board, s *term.Session, tok map[string]string, user *store.User) {
		b.whosOnline(s)
	}},
	"teleconference": {Feature: "teleconference", Doing: "in teleconference", Run: func(b *board, s *term.Session, tok map[string]string, user *store.User) {
		b.teleconference(s, user)
	}},
	"page": {Feature: "paging", Doing: "paging the sysop", Run: func(b *board, s *term.Session, tok map[string]string, user *store.User) {
		b.pageSysop(s, user)
	}},
	"doors": {Feature: "doors", Doing: "in the doors", Run: func(b *board, s *term.Session, tok map[string]string, user *store.User) {
		b.doors(s, tok, user)
	}},
	"qwk": {Feature: "qwk", Doing: "packing qwk mail", Run: func(b *board, s *term.Session, tok map[string]string, user *store.User) {
		b.qwk(s, tok, user)
	}},
	"newfiles": {Feature: "newfiles", Run: func(b *board, s *term.Session, tok map[string]string, user *store.User) {
		b.newFiles(s, user)
	}},
	"gfiles": {Feature: "gfiles", Doing: "reading g-files", Run: func(b *board, s *term.Session, tok map[string]string, user *store.User) {
		b.gFiles(s, tok, user)
	}},
	"bbslist": {Feature: "bbslist", Run: func(b *board, s *term.Session, tok map[string]string, user *store.User) {
		b.bbsList(s, tok, user)
	}},
	"voting": {Feature: "voting", Doing: "in the voting booth", Run: func(b *board, s *term.Session, tok map[string]string, user *store.User) {
		b.votingBooth(s, tok, user)
	}},
	"lastcallers": {Run: func(b *board, s *term.Session, tok map[string]string, user *store.User) {
		b.lastCallers(s)
	}},
	"userlist": {Run: func(b *board, s *term.Session, tok map[string]string, user *store.User) {
		b.userList(s)
	}},
	"stats": {Run: func(b *board, s *term.Session, tok map[string]string, user *store.User) {
		b.profile(s, user)
	}},
	"sysinfo": {Run: func(b *board, s *term.Session, tok map[string]string, user *store.User) {
		b.sysInfo(s, tok, user)
	}},
	"settings": {Run: func(b *board, s *term.Session, tok map[string]string, user *store.User) {
		b.settings(s, tok, user)
	}},
	"goodbye": {}, // Run == nil sentinel; see mainMenu
}

// mainMenuOptions builds the pipe-code lines for every bound, enabled slot
// (one |{row,col,key,label} marker each, in menu.MainMenuSlots order) and the
// hotkey-byte -> action lookup mainMenu dispatches on. A slot missing from
// items (should only happen if the DB predates seeding) falls back to its
// shipped default; an unknown or blank-action slot is skipped, leaving that
// row of the menu blank.
func mainMenuOptions(items map[string]menu.Item) (optionsText string, keyToAction map[byte]string) {
	defaults := make(map[string]menu.Item, len(mainMenuDefaults))
	for _, d := range mainMenuDefaults {
		defaults[d.Slot] = d
	}

	keyToAction = map[byte]string{}
	var lines []string
	for _, slot := range menu.MainMenuSlots {
		it, ok := items[slot]
		if !ok {
			it = defaults[slot]
		}
		if !it.Enabled || it.Action == "" || it.Hotkey == "" {
			continue
		}
		if _, known := mainMenuDispatch[it.Action]; !known {
			continue // stale action from a since-removed catalog entry
		}
		pos, ok := mainMenuSlotPos[slot]
		if !ok {
			continue
		}
		lines = append(lines, fmt.Sprintf("|{%d,%d,%s,%s}", pos.Row, pos.Col, it.Hotkey, it.Label))
		keyToAction[it.Hotkey[0]] = it.Action
	}

	joined := ""
	for i, ln := range lines {
		if i > 0 {
			joined += "\n"
		}
		joined += ln
	}
	return joined, keyToAction
}

// mainMenuTemplate loads art/mainmenu.pp and splices the current bindings
// into its @@MENU_OPTIONS@@ placeholder -- one composed byte stream so the
// options inherit whatever color the chrome left set (see the render engine:
// a |{...} marker draws in whatever SGR state is already active, and that
// state only carries across a single Render pass, not between two).
func (b *board) mainMenuTemplate(items map[string]menu.Item) ([]byte, map[byte]string) {
	raw, err := os.ReadFile(b.art + "/mainmenu.pp")
	if err != nil {
		return nil, nil
	}
	optionsText, keyToAction := mainMenuOptions(items)
	return bytes.Replace(raw, []byte("@@MENU_OPTIONS@@"), []byte(optionsText), 1), keyToAction
}
