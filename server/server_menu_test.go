package main

import (
	"testing"

	"vendetta-x/server/internal/menu"
)

// TestMainMenuActionsMatchCatalog keeps the sysop panel's action dropdown
// (built from menu.Catalog) and the runtime dispatch table
// (mainMenuDispatch) from silently drifting apart: a catalog entry with no
// handler would let a sysop bind a slot to an action that can never run; a
// handler with no catalog entry would be unreachable from the web UI.
// Mirrors server_schedule_test.go's TestScheduledActionsMatchCatalog.
func TestMainMenuActionsMatchCatalog(t *testing.T) {
	for _, a := range menu.Catalog {
		if _, ok := mainMenuDispatch[a.Key]; !ok {
			t.Errorf("catalog action %q has no dispatch handler", a.Key)
		}
	}
	for key := range mainMenuDispatch {
		found := false
		for _, a := range menu.Catalog {
			if a.Key == key {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("dispatch handler %q is not in menu.Catalog", key)
		}
	}
}

// TestMainMenuSlotsComplete keeps the slot layout (mainMenuSlotPos, the
// physical row/col each slot paints at) and the abstract slot list
// (menu.MainMenuSlots, what the sysop panel edits) from drifting apart.
func TestMainMenuSlotsComplete(t *testing.T) {
	for _, slot := range menu.MainMenuSlots {
		if _, ok := mainMenuSlotPos[slot]; !ok {
			t.Errorf("slot %q has no screen position in mainMenuSlotPos", slot)
		}
	}
	for slot := range mainMenuSlotPos {
		found := false
		for _, s := range menu.MainMenuSlots {
			if s == slot {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("mainMenuSlotPos has position for %q, not in menu.MainMenuSlots", slot)
		}
	}
}

// TestMainMenuDefaultsAreComplete checks the shipped default bindings cover
// every slot exactly once, each with a dispatchable action -- so a fresh
// board's menu (before any sysop customization) always renders and works.
func TestMainMenuDefaultsAreComplete(t *testing.T) {
	bySlot := map[string]bool{}
	for _, d := range mainMenuDefaults {
		if bySlot[d.Slot] {
			t.Errorf("default slot %q defined twice", d.Slot)
		}
		bySlot[d.Slot] = true
		if !d.Enabled || d.Action == "" {
			t.Errorf("default slot %q should ship enabled and bound", d.Slot)
		}
		if _, ok := mainMenuDispatch[d.Action]; !ok {
			t.Errorf("default slot %q binds unknown action %q", d.Slot, d.Action)
		}
		if d.Hotkey == "" {
			t.Errorf("default slot %q has no hotkey", d.Slot)
		}
	}
	for _, slot := range menu.MainMenuSlots {
		if !bySlot[slot] {
			t.Errorf("slot %q has no default binding", slot)
		}
	}
}

// TestMainMenuOptionsSkipsDisabledAndBlank verifies the rendering-side
// filter: disabled slots, blank actions, and unknown actions never produce a
// marker line or a reachable hotkey.
func TestMainMenuOptionsSkipsDisabledAndBlank(t *testing.T) {
	// Every slot explicitly present (all but L0 blank) so the fallback-to-
	// defaults path (covered separately below) can't leak an extra binding
	// into this test's assertions.
	items := map[string]menu.Item{}
	for _, slot := range menu.MainMenuSlots {
		items[slot] = menu.Item{Slot: slot}
	}
	items["L0"] = menu.Item{Slot: "L0", Action: "messages", Label: "Msgs", Hotkey: "M", Enabled: true}
	items["L1"] = menu.Item{Slot: "L1", Action: "files", Label: "Files", Hotkey: "F", Enabled: false} // disabled
	items["L3"] = menu.Item{Slot: "L3", Action: "no-such-action", Label: "Ghost", Hotkey: "K", Enabled: true}

	text, keyToAction := mainMenuOptions(items)
	if _, ok := keyToAction['F']; ok {
		t.Fatal("disabled slot produced a reachable hotkey")
	}
	if _, ok := keyToAction['K']; ok {
		t.Fatal("unknown-action slot produced a reachable hotkey")
	}
	if keyToAction['M'] != "messages" {
		t.Fatalf("enabled slot not reachable: %v", keyToAction)
	}
	if want := "|{14,8,M,Msgs}"; text != want {
		t.Fatalf("options text = %q, want exactly %q (only the one live slot)", text, want)
	}
}

// TestMainMenuOptionsFallsBackToDefaults checks a slot absent from the
// stored items (e.g. a fresh table row never written) still renders its
// shipped default rather than vanishing.
func TestMainMenuOptionsFallsBackToDefaults(t *testing.T) {
	_, keyToAction := mainMenuOptions(map[string]menu.Item{})
	if keyToAction['M'] != "messages" {
		t.Fatalf("default binding for L0 missing: %v", keyToAction)
	}
	if len(keyToAction) != len(mainMenuDefaults) {
		t.Fatalf("got %d reachable hotkeys, want %d (one per default)", len(keyToAction), len(mainMenuDefaults))
	}
}
