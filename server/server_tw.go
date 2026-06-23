package main

import (
	"math/rand"
	"time"

	"vendetta-x/server/internal/sanitize"
	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/term"
	"vendetta-x/server/internal/tw"
)

// tradeWars launches the native "Trade Wars" (TradeWars-2002-style) door. The
// galaxy is shared across all players; each board account flies one persistent
// ship, created on first play.
func (b *board) tradeWars(s *term.Session, user *store.User) {
	if b.tw == nil {
		s.Notice("Trade Wars isn't available right now.")
		return
	}

	tr, found, err := b.tw.Load(user.Handle)
	if err != nil {
		s.Notice("Could not load your ship's log.")
		return
	}
	if !found {
		tr = b.newTrader(s, user)
		if tr == nil {
			return
		}
	}

	g := tw.NewGame(s, b.tw, tr, rand.New(rand.NewSource(time.Now().UnixNano())))
	g.Run()
}

// newTrader runs the one-time captain registration for a first-time player.
func (b *board) newTrader(s *term.Session, user *store.User) *tw.Trader {
	s.Print("\x1b[0m\x1b[2J\x1b[H")
	s.Print("\x1b[1;36m  T R A D E   W A R S   2 0 0 2\x1b[0m\r\n")
	s.Print("\x1b[1;30m  " + cp437rule(52) + "\x1b[0m\r\n\r\n")
	s.Print("\x1b[0;37m  A new ship rolls off the line at Sol Station.\x1b[0m\r\n\r\n")
	s.Printf("\x1b[0;37m  What name shall the registry show, Captain? \x1b[1;30m[%s]\x1b[0;37m: \x1b[1;37m", user.Handle)
	s.Flush()

	name := sanitize.Line(s.ReadLine(20))
	if name == "" {
		name = user.Handle
	}

	tr := tw.NewTrader(user.Handle, name)
	if err := b.tw.Save(tr); err != nil {
		s.Notice("Could not register your ship.")
		return nil
	}
	s.Printf("\r\n\x1b[1;32m  Cleared for launch, Captain %s. The galaxy awaits.\x1b[0m\r\n", name)
	s.Pause()
	return tr
}
