package store

import "testing"

// seedSearchBoards creates two boards and posts a few messages; returns the
// board ids as (gen, sysop).
func seedSearchBoards(t *testing.T, s *Store) (int64, int64) {
	t.Helper()
	gen, err := s.db.Exec(`INSERT INTO boards (tag, name) VALUES ('gen', 'General')`)
	if err != nil {
		t.Fatalf("insert board: %v", err)
	}
	genID, _ := gen.LastInsertId()
	sys, err := s.db.Exec(`INSERT INTO boards (tag, name, read_acs) VALUES ('sysop', 'Sysop', 's100')`)
	if err != nil {
		t.Fatalf("insert board: %v", err)
	}
	sysID, _ := sys.LastInsertId()

	for _, m := range []Message{
		{BoardID: genID, From: "nut", Subject: "Modem noise", Body: "the carrier tone at 3am"},
		{BoardID: genID, From: "phantom", Subject: "Ratio law", Body: "maintain your ratio or else"},
		{BoardID: sysID, From: "sysop", Subject: "secret plans", Body: "the carrier of our schemes"},
	} {
		mm := m
		if _, err := s.PostMessage(&mm); err != nil {
			t.Fatalf("PostMessage: %v", err)
		}
	}
	return genID, sysID
}

func TestSearchMessages(t *testing.T) {
	s := newTestStore(t)
	genID, sysID := seedSearchBoards(t, s)

	// Body match, scoped to the readable board only.
	got, err := s.SearchMessages("carrier", []int64{genID}, 0)
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(got) != 1 || got[0].Subject != "Modem noise" {
		t.Fatalf("carrier in [gen]: got %d results %v, want 1 (Modem noise)", len(got), got)
	}

	// The same term also lives in the sysop base; including it surfaces both.
	got, err = s.SearchMessages("carrier", []int64{genID, sysID}, 0)
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("carrier in [gen,sysop]: got %d, want 2", len(got))
	}

	// Author match is case-insensitive.
	got, _ = s.SearchMessages("PHANTOM", []int64{genID}, 0)
	if len(got) != 1 || got[0].From != "phantom" {
		t.Fatalf("author search: got %d %v, want 1 phantom", len(got), got)
	}

	// An empty readable set (no boards the caller can open) returns nothing,
	// even for a term that exists -- the ACS scoping is enforced here.
	if got, _ := s.SearchMessages("carrier", nil, 0); len(got) != 0 {
		t.Fatalf("empty id set: got %d, want 0", len(got))
	}

	// A blank query never matches everything.
	if got, _ := s.SearchMessages("   ", []int64{genID}, 0); len(got) != 0 {
		t.Fatalf("blank query: got %d, want 0", len(got))
	}
}

func TestSearchMessagesWildcardsAreLiteral(t *testing.T) {
	s := newTestStore(t)
	genID, _ := seedSearchBoards(t, s)
	m := Message{BoardID: genID, From: "nut", Subject: "100% cracked", Body: "no fakes"}
	if _, err := s.PostMessage(&m); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	// A bare "%" must not act as "match anything" -- it should match only the
	// message that literally contains a percent sign.
	got, err := s.SearchMessages("%", []int64{genID}, 0)
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(got) != 1 || got[0].Subject != "100% cracked" {
		t.Fatalf("literal %%: got %d %v, want 1 (100%% cracked)", len(got), got)
	}
}

func TestSearchFiles(t *testing.T) {
	s := newTestStore(t)
	a1, _ := s.db.Exec(`INSERT INTO file_areas (tag, name) VALUES ('warez', 'Warez')`)
	area1, _ := a1.LastInsertId()
	a2, _ := s.db.Exec(`INSERT INTO file_areas (tag, name, acs) VALUES ('elite', 'Elite', 's100')`)
	area2, _ := a2.LastInsertId()

	if _, err := s.AddFile(area1, "PKZIP204G.EXE", "the one and only packer", "nut", []byte("zip")); err != nil {
		t.Fatalf("AddFile: %v", err)
	}
	if _, err := s.AddFile(area2, "SECRET.ARJ", "packer notes, restricted", "sysop", []byte("arj")); err != nil {
		t.Fatalf("AddFile: %v", err)
	}
	// A pending upload must never surface in search.
	if _, err := s.AddPendingFile(area1, "PACKED.ZIP", "queued packer", "nut", []byte("pending")); err != nil {
		t.Fatalf("AddPendingFile: %v", err)
	}

	// "packer" is in all three, but only the approved file in the accessible
	// area comes back.
	got, err := s.SearchFiles("packer", []int64{area1}, 0)
	if err != nil {
		t.Fatalf("SearchFiles: %v", err)
	}
	if len(got) != 1 || got[0].Filename != "PKZIP204G.EXE" {
		t.Fatalf("packer in [warez]: got %d %v, want 1 (PKZIP204G.EXE)", len(got), got)
	}

	// Filename match across both accessible areas.
	got, _ = s.SearchFiles(".exe", []int64{area1, area2}, 0)
	if len(got) != 1 || got[0].Filename != "PKZIP204G.EXE" {
		t.Fatalf("extension search: got %d %v", len(got), got)
	}

	// Empty accessible set -> nothing.
	if got, _ := s.SearchFiles("packer", nil, 0); len(got) != 0 {
		t.Fatalf("empty id set: got %d, want 0", len(got))
	}
}
