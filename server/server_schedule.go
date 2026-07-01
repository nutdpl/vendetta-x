package main

import (
	"context"
	"log"
	"time"
)

// scheduledActions is the dispatch table for schedule.Catalog: every key in
// the catalog must have a handler here, and vice versa (enforced by
// TestScheduledActionsMatchCatalog). Handlers run unattended on the
// scheduler's own goroutine -- no caller session, no terminal -- so they
// touch only the store, never s *term.Session.
var scheduledActions = map[string]func(b *board) error{
	"messages.purge_old": func(b *board) error {
		days := b.st.SettingInt("schedule.messages.purge_days", 180)
		n, err := b.st.PurgeOldMessages(time.Now().AddDate(0, 0, -days))
		if err != nil {
			return err
		}
		if n > 0 {
			log.Printf("schedule: purged %d message(s) older than %d days", n, days)
		}
		return nil
	},
	"oneliners.trim": func(b *board) error {
		keep := b.st.SettingInt("schedule.oneliners.keep", 200)
		n, err := b.st.TrimOneliners(keep)
		if err != nil {
			return err
		}
		if n > 0 {
			log.Printf("schedule: trimmed %d oneliner(s), kept %d most recent", n, keep)
		}
		return nil
	},
}

// runScheduler is the board's event loop: once a minute (plus an immediate
// check at startup, so an event due while the board was offline still fires
// promptly on restart) it runs every enabled event whose time has come. It
// exits when ctx is cancelled.
func (b *board) runScheduler(ctx context.Context) {
	b.runDueEvents(time.Now())

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			b.runDueEvents(now)
		}
	}
}

// runDueEvents runs (and marks run) every enabled scheduled event whose
// TimeOfDay has come and hasn't already fired today. An unknown action key or
// a handler error is logged and skipped -- it never crashes the scheduler
// loop or blocks other due events.
func (b *board) runDueEvents(now time.Time) {
	if b.events == nil {
		return
	}
	evs, err := b.events.List()
	if err != nil {
		log.Printf("schedule: list events: %v", err)
		return
	}
	for _, e := range evs {
		if !e.DueAt(now) {
			continue
		}
		run, ok := scheduledActions[e.Action]
		if !ok {
			log.Printf("schedule: event %q: unknown action %q", e.Name, e.Action)
			continue
		}
		if err := run(b); err != nil {
			log.Printf("schedule: event %q (%s): %v", e.Name, e.Action, err)
		}
		if err := b.events.MarkRun(e.ID, now); err != nil {
			log.Printf("schedule: mark run %q: %v", e.Name, err)
		}
	}
}
