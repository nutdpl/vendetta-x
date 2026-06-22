package web

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"vendetta-x/server/internal/voting"
)

// pollCard pairs a poll with its total vote count and whether the current user
// has already voted, so the index can show status at a glance.
type pollCard struct {
	Poll  voting.Poll
	Total int
	Voted bool
}

// votingList renders the booth index: every poll, newest first, each linking to
// its detail page. Owned by the voting feature.
func (s *server) votingList(w http.ResponseWriter, r *http.Request) {
	base := s.base(r, "voting booth", "voting")

	polls, err := s.voting.Polls()
	if err != nil {
		log.Printf("web: voting Polls: %v", err)
	}

	cards := make([]pollCard, 0, len(polls))
	for _, p := range polls {
		c := pollCard{Poll: p}
		if _, total, err := s.voting.Results(p.ID); err == nil {
			c.Total = total
		}
		if base.User != nil {
			c.Voted, _ = s.voting.HasVoted(p.ID, base.User.Handle)
		}
		cards = append(cards, c)
	}

	s.render(w, "voting", struct {
		pageData
		Cards []pollCard
	}{base, cards})
}

// optionResult is one option with its tally and percentage, for the results
// view.
type optionResult struct {
	Option voting.Option
	Pct    int
}

// votingShowPage is the data for the single-poll view: either a vote form (when
// the caller may still vote) or the results.
type votingShowPage struct {
	pageData
	ID      int64
	Poll    *voting.Poll
	Options []voting.Option // for the vote form
	Results []optionResult  // for the results view
	Total   int
	Max     int
	CanVote bool
}

// votingShow renders one poll: a vote form for a logged-in caller who hasn't
// voted on an open poll, otherwise the results with c-bar bars + percentages.
func (s *server) votingShow(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		http.Redirect(w, r, "/voting", http.StatusSeeOther)
		return
	}

	base := s.base(r, "voting booth", "voting")
	poll, opts, err := s.voting.Poll(id)
	if err != nil {
		log.Printf("web: voting Poll: %v", err)
	}
	if poll == nil {
		http.Redirect(w, r, "/voting", http.StatusSeeOther)
		return
	}
	base.Title = poll.Question

	page := votingShowPage{pageData: base, ID: id, Poll: poll}

	voted := false
	if base.User != nil {
		voted, _ = s.voting.HasVoted(id, base.User.Handle)
	}
	page.CanVote = base.User != nil && !voted && !poll.Closed

	for _, o := range opts {
		page.Total += o.Votes
		if o.Votes > page.Max {
			page.Max = o.Votes
		}
	}

	if page.CanVote {
		page.Options = opts
	} else {
		for _, o := range opts {
			page.Results = append(page.Results, optionResult{
				Option: o,
				Pct:    pct(o.Votes, page.Total),
			})
		}
	}

	s.render(w, "voting_show", page)
}

// votingVote records the caller's choice and 303s back to the poll. It requires
// a logged-in user; a bad/closed poll or duplicate vote just bounces back.
func (s *server) votingVote(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		http.Redirect(w, r, "/voting", http.StatusSeeOther)
		return
	}
	dest := "/voting/" + strconv.FormatInt(id, 10)

	u := s.currentUser(r)
	if u == nil {
		http.Redirect(w, r, "/login?next="+dest, http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, dest, http.StatusSeeOther)
		return
	}
	optionID, ok := parseID(r.FormValue("option_id"))
	if !ok {
		http.Redirect(w, r, dest, http.StatusSeeOther)
		return
	}

	// Vote enforces option-belongs-to-poll and one-vote-per-voter itself; a
	// soft error (already voted, bad option) just returns the caller to the
	// (now results) page.
	if err := s.voting.Vote(id, optionID, u.Handle); err != nil &&
		err != voting.ErrAlreadyVoted && err != voting.ErrBadOption {
		log.Printf("web: voting Vote: %v", err)
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

// votingNewForm renders the create-a-poll form. Login required.
func (s *server) votingNewForm(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		http.Redirect(w, r, "/login?next=/voting/new", http.StatusSeeOther)
		return
	}
	s.render(w, "voting_new", struct {
		pageData
		Slots []int // option input slots to render
	}{s.base(r, "new poll", "voting"), []int{1, 2, 3, 4, 5, 6}})
}

// votingCreate collects the question and non-empty options, creates the poll,
// and 303s to it. Login required; an invalid form returns to the new-poll page.
func (s *server) votingCreate(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		http.Redirect(w, r, "/login?next=/voting/new", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/voting/new", http.StatusSeeOther)
		return
	}

	question := strings.TrimSpace(r.FormValue("question"))
	var options []string
	for _, v := range r.Form["option"] {
		if t := strings.TrimSpace(v); t != "" {
			options = append(options, t)
		}
	}

	if question == "" || len(options) < 2 {
		http.Redirect(w, r, "/voting/new", http.StatusSeeOther)
		return
	}

	id, err := s.voting.Create(question, u.Handle, options)
	if err != nil {
		log.Printf("web: voting Create: %v", err)
		http.Redirect(w, r, "/voting/new", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/voting/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}
