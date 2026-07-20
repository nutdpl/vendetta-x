package main

import (
	"fmt"
	"strconv"
	"strings"

	"vendetta-x/server/internal/sanitize"
	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/term"
)

// sendNodeMessage prompts for a target node and a one-line message and queues
// it for delivery at that caller's next menu. The text is control-byte
// sanitized before it can reach another caller's screen, and tagged with the
// sender's handle.
func (b *board) sendNodeMessage(s *term.Session, user *store.User) {
	s.Print("\r\n\x1b[0;37m  Send to node \x1b[1;37m#\x1b[0;37m: \x1b[1;37m")
	s.Flush()
	node, err := strconv.Atoi(strings.TrimSpace(s.ReadLine(5)))
	if err != nil || node < 1 {
		s.Notice("That's not a node number.")
		return
	}
	s.Print("\x1b[0;37m  Message: \x1b[1;37m")
	s.Flush()
	text := sanitize.Line(strings.TrimSpace(s.ReadLine(70)))
	if text == "" {
		return
	}
	line := fmt.Sprintf("node message from %s: %s", user.Handle, text)
	if !b.pres.send(node, line) {
		s.Notice(fmt.Sprintf("No one is on node %d right now.", node))
		return
	}
	s.Printf("\x1b[1;32m  Sent to node %d.\x1b[0m\r\n", node)
	s.Pause()
}

// deliverNodeMessages flushes any node-to-node messages waiting for this node,
// shown full-screen before the main menu repaints. A no-op when the inbox is
// empty.
func (b *board) deliverNodeMessages(s *term.Session, node int) {
	msgs := b.pres.drain(node)
	if len(msgs) == 0 {
		return
	}
	s.Print("\x1b[0m\x1b[2J\x1b[H")
	b.screenHeader(s, "node messages")
	for _, m := range msgs {
		s.Printf("  \x1b[1;33m\xfe \x1b[1;37m%s\x1b[0m\r\n", m)
	}
	s.Pause()
}
