package voting

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// newTestStore opens an in-memory SQLite database and builds a voting Store
// (migrate + seed) over it.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	st, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return st
}

// TestSeed checks the booth comes up seeded with exactly one sample poll that
// has options.
func TestSeed(t *testing.T) {
	st := newTestStore(t)
	polls, err := st.Polls()
	if err != nil {
		t.Fatalf("Polls: %v", err)
	}
	if len(polls) != 1 {
		t.Fatalf("seed: want 1 poll, got %d", len(polls))
	}
	_, opts, err := st.Poll(polls[0].ID)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(opts) < 2 {
		t.Fatalf("seed: want >=2 options, got %d", len(opts))
	}

	// New is idempotent: a second build over the same DB does not re-seed.
	if err := st.seed(); err != nil {
		t.Fatalf("seed again: %v", err)
	}
	polls, _ = st.Polls()
	if len(polls) != 1 {
		t.Fatalf("seed not idempotent: got %d polls", len(polls))
	}
}

// TestCreateAndPoll checks Create persists a poll and its options, and Poll
// reads them back.
func TestCreateAndPoll(t *testing.T) {
	st := newTestStore(t)
	id, err := st.Create("best editor?", "razor", []string{"vi", "", "emacs", "ed"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	p, opts, err := st.Poll(id)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if p == nil {
		t.Fatalf("Poll: got nil for id %d", id)
	}
	if p.Question != "best editor?" || p.Author != "razor" {
		t.Fatalf("Poll: wrong fields: %+v", p)
	}
	// The blank option is ignored -> 3 options.
	if len(opts) != 3 {
		t.Fatalf("Create: want 3 options (blank ignored), got %d", len(opts))
	}
	for _, o := range opts {
		if o.Votes != 0 {
			t.Fatalf("fresh option %q has %d votes", o.Text, o.Votes)
		}
	}
}

// TestCreateRequiresTwoOptions checks Create rejects a poll with fewer than two
// non-blank options.
func TestCreateRequiresTwoOptions(t *testing.T) {
	st := newTestStore(t)
	if _, err := st.Create("solo?", "acid", []string{"only", "", ""}); err == nil {
		t.Fatalf("Create: expected error with one non-blank option, got nil")
	}
	if _, err := st.Create("none?", "acid", nil); err == nil {
		t.Fatalf("Create: expected error with no options, got nil")
	}
}

// TestVoteAndResults checks a vote lands on the right option and tallies/totals
// are correct.
func TestVoteAndResults(t *testing.T) {
	st := newTestStore(t)
	id, err := st.Create("pick one", "sysop", []string{"alpha", "beta", "gamma"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, opts, err := st.Poll(id)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	beta := opts[1].ID

	if err := st.Vote(id, beta, "razor"); err != nil {
		t.Fatalf("Vote: %v", err)
	}
	if err := st.Vote(id, beta, "acid"); err != nil {
		t.Fatalf("Vote: %v", err)
	}
	if err := st.Vote(id, opts[0].ID, "phiber"); err != nil {
		t.Fatalf("Vote: %v", err)
	}

	res, total, err := st.Results(id)
	if err != nil {
		t.Fatalf("Results: %v", err)
	}
	if total != 3 {
		t.Fatalf("Results: want total 3, got %d", total)
	}
	got := map[string]int{}
	for _, o := range res {
		got[o.Text] = o.Votes
	}
	if got["alpha"] != 1 || got["beta"] != 2 || got["gamma"] != 0 {
		t.Fatalf("Results: wrong tallies: %+v", got)
	}
}

// TestDoubleVoteRejected checks a second vote by the same voter is rejected and
// leaves the tally unchanged, and HasVoted reflects the vote.
func TestDoubleVoteRejected(t *testing.T) {
	st := newTestStore(t)
	id, err := st.Create("again?", "sysop", []string{"yes", "no"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, opts, _ := st.Poll(id)
	yes, no := opts[0].ID, opts[1].ID

	if voted, _ := st.HasVoted(id, "razor"); voted {
		t.Fatalf("HasVoted: want false before voting")
	}
	if err := st.Vote(id, yes, "razor"); err != nil {
		t.Fatalf("Vote: %v", err)
	}
	if voted, _ := st.HasVoted(id, "razor"); !voted {
		t.Fatalf("HasVoted: want true after voting")
	}

	// A second vote by the same voter must be rejected, tally unchanged.
	if err := st.Vote(id, no, "razor"); err != ErrAlreadyVoted {
		t.Fatalf("Vote second time: want ErrAlreadyVoted, got %v", err)
	}
	_, total, _ := st.Results(id)
	if total != 1 {
		t.Fatalf("double vote changed total: got %d, want 1", total)
	}
}

// TestForeignOptionRejected checks voting with an optionID that belongs to a
// different poll is rejected.
func TestForeignOptionRejected(t *testing.T) {
	st := newTestStore(t)
	a, _ := st.Create("poll A", "sysop", []string{"a1", "a2"})
	b, _ := st.Create("poll B", "sysop", []string{"b1", "b2"})

	_, bopts, _ := st.Poll(b)
	foreign := bopts[0].ID

	if err := st.Vote(a, foreign, "razor"); err != ErrBadOption {
		t.Fatalf("Vote foreign option: want ErrBadOption, got %v", err)
	}
	// Nothing recorded on poll A.
	if voted, _ := st.HasVoted(a, "razor"); voted {
		t.Fatalf("HasVoted: foreign vote should not have recorded")
	}
	_, total, _ := st.Results(a)
	if total != 0 {
		t.Fatalf("foreign vote leaked into tally: total %d", total)
	}
}

// TestSetClosed checks SetClosed toggles the poll's Closed flag in both
// directions, reflected by Poll.
func TestSetClosed(t *testing.T) {
	st := newTestStore(t)
	id, err := st.Create("close me?", "sysop", []string{"yes", "no"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	p, _, _ := st.Poll(id)
	if p == nil || p.Closed {
		t.Fatalf("fresh poll should be open: %+v", p)
	}

	if err := st.SetClosed(id, true); err != nil {
		t.Fatalf("SetClosed(true): %v", err)
	}
	p, _, _ = st.Poll(id)
	if p == nil || !p.Closed {
		t.Fatalf("poll should be closed after SetClosed(true): %+v", p)
	}

	if err := st.SetClosed(id, false); err != nil {
		t.Fatalf("SetClosed(false): %v", err)
	}
	p, _, _ = st.Poll(id)
	if p == nil || p.Closed {
		t.Fatalf("poll should be open after SetClosed(false): %+v", p)
	}
}

// TestDelete checks Delete removes the poll, its options, and its votes so that
// Poll returns nil and Results comes back empty with a zero total.
func TestDelete(t *testing.T) {
	st := newTestStore(t)
	id, err := st.Create("delete me?", "sysop", []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, opts, _ := st.Poll(id)
	if err := st.Vote(id, opts[0].ID, "razor"); err != nil {
		t.Fatalf("Vote: %v", err)
	}

	if err := st.Delete(id); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	p, gotOpts, err := st.Poll(id)
	if err != nil {
		t.Fatalf("Poll after delete: %v", err)
	}
	if p != nil || gotOpts != nil {
		t.Fatalf("Poll after delete: want nil,nil; got %+v %+v", p, gotOpts)
	}

	res, total, err := st.Results(id)
	if err != nil {
		t.Fatalf("Results after delete: %v", err)
	}
	if len(res) != 0 || total != 0 {
		t.Fatalf("Results after delete: want empty/0; got %d options, total %d", len(res), total)
	}

	// Vote rows are gone too: the deleted voter no longer counts as having voted.
	if voted, _ := st.HasVoted(id, "razor"); voted {
		t.Fatalf("HasVoted after delete: vote row should be gone")
	}
}

// TestPollMissing checks Poll returns nil for an unknown id.
func TestPollMissing(t *testing.T) {
	st := newTestStore(t)
	p, opts, err := st.Poll(999999)
	if err != nil {
		t.Fatalf("Poll(missing): %v", err)
	}
	if p != nil || opts != nil {
		t.Fatalf("Poll(missing): want nil,nil; got %+v %+v", p, opts)
	}
}
