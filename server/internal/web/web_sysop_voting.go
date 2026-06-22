package web

import (
	"log"
	"net/http"

	"vendetta-x/server/internal/voting"
)

// votingRow pairs a poll with its total vote count for the sysop list.
type votingRow struct {
	Poll  voting.Poll
	Votes int
}

// sysopVoting lists every poll with its vote total so a sysop can close/reopen
// or delete it. Router gates this with s.admin.
func (s *server) sysopVoting(w http.ResponseWriter, r *http.Request) {
	base := s.base(r, "voting booth", "sysop")

	polls, err := s.voting.Polls()
	if err != nil {
		log.Printf("web: sysop voting Polls: %v", err)
	}

	rows := make([]votingRow, 0, len(polls))
	for _, p := range polls {
		row := votingRow{Poll: p}
		if _, total, err := s.voting.Results(p.ID); err == nil {
			row.Votes = total
		}
		rows = append(rows, row)
	}

	s.render(w, "sysop_voting", struct {
		pageData
		Polls []votingRow
	}{base, rows})
}

// sysopVotingClose toggles a poll's closed state and 303s back to the list. An
// optional form value "closed" ("1"/"0") sets an explicit target state.
func (s *server) sysopVotingClose(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		http.Redirect(w, r, "/sysop/voting", http.StatusSeeOther)
		return
	}

	poll, _, err := s.voting.Poll(id)
	if err != nil {
		log.Printf("web: sysop voting Poll: %v", err)
	}
	if poll == nil {
		http.Redirect(w, r, "/sysop/voting", http.StatusSeeOther)
		return
	}

	closed := !poll.Closed
	if err := r.ParseForm(); err == nil {
		switch r.FormValue("closed") {
		case "1":
			closed = true
		case "0":
			closed = false
		}
	}

	if err := s.voting.SetClosed(id, closed); err != nil {
		log.Printf("web: sysop voting SetClosed: %v", err)
	}
	http.Redirect(w, r, "/sysop/voting", http.StatusSeeOther)
}

// sysopVotingDelete deletes a poll (and its options/votes) and 303s back.
func (s *server) sysopVotingDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		http.Redirect(w, r, "/sysop/voting", http.StatusSeeOther)
		return
	}
	if err := s.voting.Delete(id); err != nil {
		log.Printf("web: sysop voting Delete: %v", err)
	}
	http.Redirect(w, r, "/sysop/voting", http.StatusSeeOther)
}
