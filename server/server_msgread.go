package main

import (
	"strconv"
	"strings"

	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/term"
)

// showMessage renders one message full-screen in the classic leet-board style:
// a framed header (drawn from the art/msgread.pp pipe-code template, with the
// group / from / to / subject / date / counter spliced in as |XX tokens) over
// the body, then a navigation footer. The caller's read loop (readBoard)
// handles the keystrokes; this only paints the screen.
func (b *board) showMessage(s *term.Session, bd *store.Board, msgs []store.Message, i int, canPost bool) {
	m := msgs[i]

	// The header frame is an art template; every field is a token so the
	// renderer's width modifiers (|XX\<NN) keep the right border aligned. Set
	// all keys (empty string included) so none ever falls back to a literal.
	s.RenderScreen(b.art+"/msgread.pp", map[string]string{
		"MG": bd.Name,
		"MC": "msg #" + strconv.Itoa(i+1) + " of " + strconv.Itoa(len(msgs)),
		"MF": m.From,
		"MT": m.To,
		"MS": m.Subject,
		"MD": m.Posted.Format("Mon 2006-01-02 15:04"),
	})

	// The body, plainly indented under the header.
	for _, ln := range strings.Split(m.Body, "\n") {
		s.Print("  \x1b[0;37m" + ln + "\x1b[0m\r\n")
	}

	// Navigation footer.
	s.Print("\r\n\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n")
	reply := ""
	if canPost {
		reply = "  \x1b[0;37m[\x1b[1;37mR\x1b[0;37m]eply"
	}
	thread := ""
	if m.ReplyTo != 0 {
		thread = "  \x1b[0;37m[\x1b[1;37mT\x1b[0;37m]hread\x1b[1;30m\x18\x1b[0;37m" // up-arrow: this is a reply
	}
	s.Printf("  \x1b[0;37m[\x1b[1;37mP\x1b[0;37m]rev  [\x1b[1;37mN\x1b[0;37m]ext%s%s  [\x1b[1;37mQ\x1b[0;37m]uit \x1b[1;30m\xb7 \x1b[0;37mmsg \x1b[1;37m%d\x1b[0;37m/\x1b[1;37m%d \x1b[1;36m> \x1b[1;37m",
		reply, thread, i+1, len(msgs))
	s.Flush()
}

// threadParent returns the index in msgs of m's parent, or -1 when m isn't a
// reply or the original has scrolled out of the loaded window.
func threadParent(msgs []store.Message, m store.Message) int {
	if m.ReplyTo == 0 {
		return -1
	}
	for j := range msgs {
		if msgs[j].ID == m.ReplyTo {
			return j
		}
	}
	return -1
}

// postReply composes a threaded reply to m: addressed back to its sender,
// subject pre-filled with a "Re:" of the original (not doubled if it already
// has one), the editor opened on the original >-quoted.
func (b *board) postReply(s *term.Session, bd *store.Board, user *store.User, m store.Message) {
	subj := strings.TrimSpace(m.Subject)
	if !strings.HasPrefix(strings.ToLower(subj), "re:") {
		subj = "Re: " + subj
	}
	b.compose(s, bd, user, m.From, subj, &m)
}
