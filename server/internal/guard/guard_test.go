package guard

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	s, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestIPAndCIDRBans(t *testing.T) {
	s := newTestStore(t)
	if err := s.Add(KindIP, "203.0.113.7", "flooder", 0); err != nil {
		t.Fatalf("Add ip: %v", err)
	}
	if err := s.Add(KindCIDR, "198.51.100.0/24", "bot range", 0); err != nil {
		t.Fatalf("Add cidr: %v", err)
	}

	if reason, blocked := s.BlockedIP("203.0.113.7"); !blocked || reason != "flooder" {
		t.Fatalf("exact IP: %q %v", reason, blocked)
	}
	if _, blocked := s.BlockedIP("203.0.113.8"); blocked {
		t.Fatal("neighbor IP wrongly blocked")
	}
	if _, blocked := s.BlockedIP("198.51.100.200"); !blocked {
		t.Fatal("CIDR member not blocked")
	}
	// The console is sacred: loopback can never be locked out.
	if err := s.Add(KindCIDR, "127.0.0.0/8", "oops", 0); err != nil {
		t.Fatalf("Add loopback cidr: %v", err)
	}
	if _, blocked := s.BlockedIP("127.0.0.1"); blocked {
		t.Fatal("loopback must never be blocked")
	}
}

func TestHandleTrashcan(t *testing.T) {
	s := newTestStore(t)
	if err := s.Add(KindHandle, "SysOp", "reserved", 0); err != nil {
		t.Fatalf("Add handle: %v", err)
	}
	if _, blocked := s.BlockedHandle("SYSOP2000"); !blocked {
		t.Fatal("substring/case-insensitive match failed")
	}
	if _, blocked := s.BlockedHandle("phantom"); blocked {
		t.Fatal("clean handle wrongly blocked")
	}
}

func TestExpiryAndValidation(t *testing.T) {
	s := newTestStore(t)
	if err := s.Add(KindIP, "not-an-ip", "x", 0); err == nil {
		t.Fatal("bad IP accepted")
	}
	if err := s.Add(KindCIDR, "203.0.113.7", "x", 0); err == nil {
		t.Fatal("bare IP accepted as CIDR")
	}

	// An expired ban stops enforcing but stays listed until deleted.
	if err := s.Add(KindIP, "203.0.113.9", "old news", -1); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// days<0 is treated as permanent by Add (expires only when days>0), so
	// force-expire it directly for the test.
	if _, err := s.db.Exec(`UPDATE bans SET expires = 1 WHERE value = '203.0.113.9'`); err != nil {
		t.Fatalf("force expire: %v", err)
	}
	if _, blocked := s.BlockedIP("203.0.113.9"); blocked {
		t.Fatal("expired ban still enforcing")
	}
	bans, _ := s.List()
	if len(bans) != 1 {
		t.Fatalf("expired ban missing from list: %d", len(bans))
	}
	if err := s.Delete(bans[0].ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if bans, _ = s.List(); len(bans) != 0 {
		t.Fatal("delete left the ban behind")
	}
}
