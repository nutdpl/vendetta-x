package main

import (
	"testing"
	"time"

	"vendetta-x/server/internal/schedule"
	"vendetta-x/server/internal/store"
)

// TestScheduledActionsMatchCatalog keeps the sysop panel's action dropdown
// (built from schedule.Catalog) and the runtime dispatch table
// (scheduledActions) from silently drifting apart: a catalog entry with no
// handler would let a sysop schedule an event that can never run; a handler
// with no catalog entry would be unreachable from the web UI.
func TestScheduledActionsMatchCatalog(t *testing.T) {
	for _, a := range schedule.Catalog {
		if _, ok := scheduledActions[a.Key]; !ok {
			t.Errorf("catalog action %q has no dispatch handler", a.Key)
		}
	}
	for key := range scheduledActions {
		found := false
		for _, a := range schedule.Catalog {
			if a.Key == key {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("dispatch handler %q is not in schedule.Catalog", key)
		}
	}
}

func newSchedTestBoard(t *testing.T) *board {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Seed(); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	evs, err := schedule.New(st.DB())
	if err != nil {
		t.Fatalf("schedule.New: %v", err)
	}
	return &board{st: st, events: evs}
}

func TestRunDueEventsFiresAndMarksRun(t *testing.T) {
	b := newSchedTestBoard(t)

	boardID, err := b.st.AddBoard(&store.Board{Tag: "t", Name: "Test"})
	if err != nil {
		t.Fatalf("AddBoard: %v", err)
	}
	now := time.Now()
	if _, err := b.st.PostMessage(&store.Message{
		BoardID: boardID, Subject: "ancient", Posted: now.AddDate(0, 0, -400),
	}); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	due := now.Add(-time.Minute)
	id, err := b.events.Add(&schedule.Event{
		Name: "nightly purge", Action: "messages.purge_old",
		TimeOfDay: due.Format("15:04"), Enabled: true,
	})
	if err != nil {
		t.Fatalf("Add event: %v", err)
	}

	b.runDueEvents(now)

	msgs, _ := b.st.Messages(boardID, 0)
	if len(msgs) != 0 {
		t.Fatalf("event didn't purge the old message: %d left", len(msgs))
	}

	ev, err := b.events.Get(id)
	if err != nil || ev == nil {
		t.Fatalf("Get after run: %+v, %v", ev, err)
	}
	if ev.LastRun.IsZero() {
		t.Fatal("LastRun not stamped after running")
	}

	// A second pass on the same day must not re-fire (LastRun already covers
	// today's due moment). Prove it by adding another old message: if the
	// event fired again it would be purged.
	if _, err := b.st.PostMessage(&store.Message{
		BoardID: boardID, Subject: "ancient again", Posted: now.AddDate(0, 0, -400),
	}); err != nil {
		t.Fatalf("PostMessage 2: %v", err)
	}
	b.runDueEvents(now.Add(time.Minute))
	msgs, _ = b.st.Messages(boardID, 0)
	if len(msgs) != 1 {
		t.Fatalf("event re-fired same day: %d messages left, want 1", len(msgs))
	}
}

func TestRunDueEventsSkipsUnknownAction(t *testing.T) {
	b := newSchedTestBoard(t)
	now := time.Now()
	id, err := b.events.Add(&schedule.Event{
		Name: "mystery", Action: "no.such.action",
		TimeOfDay: now.Add(-time.Minute).Format("15:04"), Enabled: true,
	})
	if err != nil {
		t.Fatalf("Add event: %v", err)
	}

	// Must not panic, and must not mark the event run (so an operator fixing
	// the typo sees it fire on the next tick instead of it looking "handled").
	b.runDueEvents(now)

	ev, _ := b.events.Get(id)
	if ev == nil {
		t.Fatal("event vanished")
	}
	if !ev.LastRun.IsZero() {
		t.Fatal("unknown-action event should not be marked run")
	}
}
