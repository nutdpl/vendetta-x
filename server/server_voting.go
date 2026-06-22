package main

import (
	"strconv"
	"strings"

	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/term"
	"vendetta-x/server/internal/voting"
)

// votingBooth runs the Voting Booth: a numbered list of polls the caller can
// vote on or add to. Pressing the booth key from the main menu lands here. It
// mirrors the Iniquity-style list screens (pickBase / listFiles): a header rule,
// rows indented two spaces, select-by-number, [Q]uit. (Named votingBooth, not
// voting, so it doesn't collide with the board's b.voting store field.)
func (b *board) votingBooth(s *term.Session, tok map[string]string, user *store.User) {
	for {
		polls, err := b.voting.Polls()
		if err != nil {
			s.Notice("Could not load the voting booth.")
			return
		}

		b.screenHeader(s, "voting booth")
		s.Print("\x1b[1;30m   #  \x1b[0;37mQuestion                                  \x1b[1;30mVotes  You\x1b[0m\r\n")
		s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n")

		if len(polls) == 0 {
			s.Print("\x1b[0;37m  No polls yet. Start one with [N]ew.\x1b[0m\r\n")
		}
		for i := range polls {
			p := &polls[i]
			n := i + 1
			_, total, _ := b.voting.Results(p.ID)
			marker := "\x1b[1;32m\xfe open"
			if p.Closed {
				marker = "\x1b[1;30m\xfa closed"
			} else if voted, _ := b.voting.HasVoted(p.ID, user.Handle); voted {
				marker = "\x1b[1;36m\xfb voted"
			}
			s.Printf("  \x1b[1;33m%2d  \x1b[1;37m%-40s \x1b[0;37m%5d  %s\x1b[0m\r\n",
				n, truncStr(p.Question, 40), total, marker)
		}

		s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n")
		s.Print("\r\n\x1b[0;37m  [\x1b[1;37mV\x1b[0;37m]ote on #  [\x1b[1;37mN\x1b[0;37m]ew poll  [\x1b[1;37mQ\x1b[0;37m]uit \x1b[1;36m> \x1b[1;37m")
		s.Flush()
		key, ch := s.ReadKey()
		if key == term.KeyEOF {
			return
		}
		s.Print("\r\n")
		switch lc(ch) {
		case 'v':
			b.votingVote(s, polls, user)
		case 'n':
			b.votingNew(s, user)
		case 'q':
			return
		}
	}
}

// votingVote prompts for a poll number, then either casts a vote (if the caller
// hasn't voted and the poll is open) or shows the results.
func (b *board) votingVote(s *term.Session, polls []voting.Poll, user *store.User) {
	s.Print("\x1b[0;37m  Poll \x1b[1;37m#\x1b[0;37m \x1b[1;36m> \x1b[1;37m")
	s.Flush()
	line := strings.TrimSpace(s.ReadLine(5))
	if line == "" {
		return
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > len(polls) {
		s.Notice("No poll with that number.")
		return
	}
	pollID := polls[n-1].ID

	poll, opts, err := b.voting.Poll(pollID)
	if err != nil || poll == nil {
		s.Notice("That poll is gone.")
		return
	}

	voted, _ := b.voting.HasVoted(pollID, user.Handle)
	if voted || poll.Closed {
		b.votingResults(s, poll, opts)
		return
	}

	// Present the options numbered and take the caller's pick.
	s.Print("\x1b[0m\x1b[2J\x1b[H")
	s.Printf("\x1b[1;36m  %s \x1b[1;30m\xfa\x1b[0;37m %s\x1b[0m\r\n", boardName, truncStr(poll.Question, 60))
	s.Printf("\x1b[1;30m  asked by %s\x1b[0m\r\n", poll.Author)
	s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n\r\n")
	for i, o := range opts {
		s.Printf("  \x1b[1;33m%2d  \x1b[1;37m%s\x1b[0m\r\n", i+1, o.Text)
	}
	s.Print("\r\n\x1b[0;37m  Your choice \x1b[1;37m#\x1b[0;37m \x1b[1;36m> \x1b[1;37m")
	s.Flush()
	pick := strings.TrimSpace(s.ReadLine(5))
	if pick == "" {
		return
	}
	c, err := strconv.Atoi(pick)
	if err != nil || c < 1 || c > len(opts) {
		s.Notice("That isn't one of the choices.")
		return
	}

	if err := b.voting.Vote(pollID, opts[c-1].ID, user.Handle); err != nil {
		s.Notice("Could not record your vote: " + err.Error())
		return
	}
	s.Print("\x1b[1;32m  Vote cast. Here's how it stands:\x1b[0m\r\n")

	// Re-read for the fresh tally, then show results.
	_, opts2, err := b.voting.Poll(pollID)
	if err == nil {
		opts = opts2
	}
	b.votingResults(s, poll, opts)
}

// votingResults draws each option with a CP437 block bar scaled to the leading
// option, plus its count and percentage.
func (b *board) votingResults(s *term.Session, poll *voting.Poll, opts []voting.Option) {
	total := 0
	maxVotes := 0
	for _, o := range opts {
		total += o.Votes
		if o.Votes > maxVotes {
			maxVotes = o.Votes
		}
	}

	s.Print("\x1b[0m\r\n")
	s.Printf("\x1b[1;36m  %s \x1b[1;30m\xfa\x1b[0;37m results\x1b[0m\r\n", boardName)
	s.Printf("\x1b[1;37m  %s\x1b[0m\r\n", truncStr(poll.Question, 68))
	s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n\r\n")

	const barCols = 30
	for _, o := range opts {
		bar := 0
		if maxVotes > 0 {
			bar = o.Votes * barCols / maxVotes
		}
		pct := 0
		if total > 0 {
			pct = o.Votes * 100 / total
		}
		s.Printf("  \x1b[1;37m%-22s\x1b[0m\r\n", truncStr(o.Text, 22))
		s.Printf("    \x1b[1;35m%s\x1b[1;30m%s \x1b[0;37m%d \x1b[1;30m(%d%%)\x1b[0m\r\n",
			strings.Repeat("\xdb", bar),
			strings.Repeat("\xb0", barCols-bar),
			o.Votes, pct)
	}
	s.Printf("\r\n\x1b[1;30m  %d %s cast.\x1b[0m\r\n", total, plural(total, "vote", "votes"))
	s.Pause()
}

// votingNew collects a question and options and creates a poll attributed to
// the caller.
func (b *board) votingNew(s *term.Session, user *store.User) {
	s.Print("\x1b[0m\x1b[2J\x1b[H")
	s.Printf("\x1b[1;36m  %s \x1b[1;30m\xfa\x1b[0;37m new poll\x1b[0m\r\n", boardName)
	s.Print("\x1b[1;30m  Up to 8 options. Blank option finishes the list.\x1b[0m\r\n")
	s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n\r\n")

	s.Print("\x1b[0;37m  Question: \x1b[1;37m")
	s.Flush()
	question := strings.TrimSpace(s.ReadLine(120))
	if question == "" {
		s.Notice("No question -- poll cancelled.")
		return
	}

	var options []string
	for len(options) < 8 {
		s.Printf("\x1b[0;37m  Option %d (blank to finish): \x1b[1;37m", len(options)+1)
		s.Flush()
		opt := strings.TrimSpace(s.ReadLine(80))
		if opt == "" {
			break
		}
		options = append(options, opt)
	}

	if len(options) < 2 {
		s.Notice("A poll needs at least two options -- cancelled.")
		return
	}

	id, err := b.voting.Create(question, user.Handle, options)
	if err != nil {
		s.Notice("Could not create the poll: " + err.Error())
		return
	}
	s.Printf("\x1b[1;32m  Poll #%d is live. The booth is open.\x1b[0m\r\n", id)
	s.Pause()
}
