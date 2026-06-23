package main

import (
	"math/rand"
	"time"

	"vendetta-x/server/internal/dragon"
	"vendetta-x/server/internal/sanitize"
	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/term"
)

// redDragon launches the native "Red Dragon" door for the caller. Each board
// account has one persistent warrior, stored in the dragon package's own table;
// first play rolls a fresh level-1 hero.
func (b *board) redDragon(s *term.Session, user *store.User) {
	if b.dragon == nil {
		s.Notice("Red Dragon isn't available right now.")
		return
	}

	ch, found, err := b.dragon.Load(user.Handle)
	if err != nil {
		s.Notice("Could not load your warrior.")
		return
	}
	if !found {
		ch = b.newWarrior(s, user)
		if ch == nil {
			return // caller bailed during creation
		}
	}

	g := dragon.NewGame(s, b.dragon, ch, rand.New(rand.NewSource(time.Now().UnixNano())))
	g.Run()
}

// newWarrior runs the one-time character creation for a first-time player.
func (b *board) newWarrior(s *term.Session, user *store.User) *dragon.Character {
	s.Print("\x1b[0m\x1b[2J\x1b[H")
	s.Print("\x1b[1;31m  R E D   D R A G O N\x1b[0m\r\n")
	s.Print("\x1b[1;30m  " + cp437rule(52) + "\x1b[0m\r\n\r\n")
	s.Print("\x1b[0;37m  A new warrior steps into the realm.\x1b[0m\r\n\r\n")
	s.Printf("\x1b[0;37m  What name shall the bards know you by? \x1b[1;30m[%s]\x1b[0;37m: \x1b[1;37m", user.Handle)
	s.Flush()

	name := sanitize.Line(s.ReadLine(20))
	if name == "" {
		name = user.Handle
	}

	ch := dragon.NewCharacter(user.Handle, name)
	if err := b.dragon.Save(ch); err != nil {
		s.Notice("Could not create your warrior.")
		return nil
	}
	s.Printf("\r\n\x1b[1;32m  Welcome to the realm, %s. Your legend begins.\x1b[0m\r\n", name)
	s.Pause()
	return ch
}
