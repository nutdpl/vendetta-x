package menu

import (
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	s, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestNilStoreItemsIsSafe(t *testing.T) {
	var s *Store
	items, err := s.Items("main")
	if items != nil || err != nil {
		t.Fatalf("nil store Items = %v, %v; want nil, nil", items, err)
	}
}

func TestEnsureSeededOnlyOnce(t *testing.T) {
	s := newTestStore(t)
	defaults := []Item{
		{Slot: "L0", Action: "messages", Label: "Message Bases", Hotkey: "M", Enabled: true},
	}
	if err := s.EnsureSeeded("main", defaults); err != nil {
		t.Fatalf("EnsureSeeded: %v", err)
	}
	// Sysop customizes...
	if err := s.Save("main", []Item{
		{Slot: "L0", Action: "messages", Label: "Msgs", Hotkey: "1", Enabled: true},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// ...and a second EnsureSeeded (simulating a restart) must not clobber it.
	if err := s.EnsureSeeded("main", defaults); err != nil {
		t.Fatalf("EnsureSeeded 2: %v", err)
	}
	items, err := s.Items("main")
	if err != nil {
		t.Fatalf("Items: %v", err)
	}
	if items["L0"].Label != "Msgs" || items["L0"].Hotkey != "1" {
		t.Fatalf("customization clobbered: %+v", items["L0"])
	}
}

func TestSaveRejectsDuplicateHotkeys(t *testing.T) {
	s := newTestStore(t)
	err := s.Save("main", []Item{
		{Slot: "L0", Action: "messages", Label: "Msgs", Hotkey: "M", Enabled: true},
		{Slot: "L1", Action: "files", Label: "Files", Hotkey: "m", Enabled: true},
	})
	if err == nil {
		t.Fatal("duplicate (case-insensitive) hotkey accepted")
	}

	// A duplicate against a DISABLED slot is fine -- it can't be pressed.
	if err := s.Save("main", []Item{
		{Slot: "L0", Action: "messages", Label: "Msgs", Hotkey: "M", Enabled: true},
		{Slot: "L1", Action: "files", Label: "Files", Hotkey: "M", Enabled: false},
	}); err != nil {
		t.Fatalf("disabled dupe wrongly rejected: %v", err)
	}
}

func TestSaveValidatesBoundSlots(t *testing.T) {
	s := newTestStore(t)
	if err := s.Save("main", []Item{{Slot: "L0", Action: "messages", Label: "", Hotkey: "M", Enabled: true}}); err == nil {
		t.Fatal("empty label on a bound slot accepted")
	}
	if err := s.Save("main", []Item{{Slot: "L0", Action: "messages", Label: "Msgs", Hotkey: "MM", Enabled: true}}); err == nil {
		t.Fatal("multi-character hotkey accepted")
	}
	if err := s.Save("main", []Item{{Slot: "L0", Action: "messages", Label: "Msgs", Hotkey: "", Enabled: true}}); err == nil {
		t.Fatal("empty hotkey on a bound slot accepted")
	}
}

func TestSaveClearsBlankSlot(t *testing.T) {
	s := newTestStore(t)
	if err := s.Save("main", []Item{
		{Slot: "L0", Action: "", Label: "leftover", Hotkey: "M", Enabled: true},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	items, _ := s.Items("main")
	got := items["L0"]
	if got.Label != "" || got.Hotkey != "" || got.Enabled {
		t.Fatalf("blank-action slot not cleared: %+v", got)
	}
}

func TestSaveStripsMarkerBreakers(t *testing.T) {
	s := newTestStore(t)
	if err := s.Save("main", []Item{
		{Slot: "L0", Action: "messages", Label: "Msg|s}", Hotkey: "M", Enabled: true},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	items, _ := s.Items("main")
	if strings.ContainsAny(items["L0"].Label, "|}") {
		t.Fatalf("marker-breaking bytes survived: %q", items["L0"].Label)
	}
}

func TestSaveIsAFullReplace(t *testing.T) {
	s := newTestStore(t)
	if err := s.Save("main", []Item{
		{Slot: "L0", Action: "messages", Label: "Msgs", Hotkey: "M", Enabled: true},
		{Slot: "L1", Action: "files", Label: "Files", Hotkey: "F", Enabled: true},
	}); err != nil {
		t.Fatalf("Save 1: %v", err)
	}
	if err := s.Save("main", []Item{
		{Slot: "L0", Action: "messages", Label: "Msgs", Hotkey: "M", Enabled: true},
	}); err != nil {
		t.Fatalf("Save 2: %v", err)
	}
	items, _ := s.Items("main")
	if _, ok := items["L1"]; ok {
		t.Fatalf("L1 survived a replace that omitted it: %+v", items)
	}
}
