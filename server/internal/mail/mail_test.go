package mail

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// newTestStore opens an in-memory SQLite db and builds a migrated Store.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	st, err := New(db)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	return st
}

func TestSendInbox(t *testing.T) {
	st := newTestStore(t)
	if err := st.Send("alice", "bob", "hi", "hello bob"); err != nil {
		t.Fatalf("send: %v", err)
	}
	in, err := st.Inbox("bob")
	if err != nil {
		t.Fatalf("inbox: %v", err)
	}
	if len(in) != 1 {
		t.Fatalf("inbox len = %d, want 1", len(in))
	}
	m := in[0]
	if m.From != "alice" || m.To != "bob" || m.Subject != "hi" || m.Body != "hello bob" {
		t.Fatalf("unexpected message: %+v", m)
	}
	if m.Read {
		t.Fatalf("new message should be unread")
	}
	if m.Sent.IsZero() {
		t.Fatalf("Sent should be stamped")
	}
}

func TestInboxNewestFirst(t *testing.T) {
	st := newTestStore(t)
	for _, s := range []string{"first", "second", "third"} {
		if err := st.Send("alice", "bob", s, ""); err != nil {
			t.Fatalf("send: %v", err)
		}
	}
	in, err := st.Inbox("bob")
	if err != nil {
		t.Fatalf("inbox: %v", err)
	}
	if len(in) != 3 || in[0].Subject != "third" || in[2].Subject != "first" {
		t.Fatalf("not newest-first: %+v", in)
	}
}

func TestUnreadCountAndMarkRead(t *testing.T) {
	st := newTestStore(t)
	st.Send("alice", "bob", "1", "")
	st.Send("alice", "bob", "2", "")
	n, err := st.UnreadCount("bob")
	if err != nil {
		t.Fatalf("unread: %v", err)
	}
	if n != 2 {
		t.Fatalf("unread = %d, want 2", n)
	}

	in, _ := st.Inbox("bob")
	if err := st.MarkRead(in[0].ID); err != nil {
		t.Fatalf("markread: %v", err)
	}
	n, _ = st.UnreadCount("bob")
	if n != 1 {
		t.Fatalf("unread after markread = %d, want 1", n)
	}
	m, _ := st.Get(in[0].ID)
	if m == nil || !m.Read {
		t.Fatalf("message should be read after MarkRead: %+v", m)
	}
}

func TestOutbox(t *testing.T) {
	st := newTestStore(t)
	st.Send("alice", "bob", "to bob", "")
	st.Send("alice", "carol", "to carol", "")
	st.Send("dave", "alice", "to alice", "")

	out, err := st.Outbox("alice")
	if err != nil {
		t.Fatalf("outbox: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("outbox len = %d, want 2", len(out))
	}
	for _, m := range out {
		if m.From != "alice" {
			t.Fatalf("outbox should only have alice's mail: %+v", m)
		}
	}
}

func TestDeleteOwnership(t *testing.T) {
	st := newTestStore(t)
	st.Send("alice", "bob", "secret", "")
	in, _ := st.Inbox("bob")
	id := in[0].ID

	// A stranger cannot delete it.
	if err := st.Delete(id, "eve"); err != nil {
		t.Fatalf("delete (stranger) returned error: %v", err)
	}
	if m, _ := st.Get(id); m == nil {
		t.Fatalf("stranger should not be able to delete the message")
	}

	// The recipient can delete it.
	if err := st.Delete(id, "bob"); err != nil {
		t.Fatalf("delete (recipient): %v", err)
	}
	if m, _ := st.Get(id); m != nil {
		t.Fatalf("recipient should have deleted the message")
	}
}

func TestDeleteBySender(t *testing.T) {
	st := newTestStore(t)
	st.Send("alice", "bob", "msg", "")
	out, _ := st.Outbox("alice")
	id := out[0].ID
	if err := st.Delete(id, "alice"); err != nil {
		t.Fatalf("delete (sender): %v", err)
	}
	if m, _ := st.Get(id); m != nil {
		t.Fatalf("sender should be able to delete the message")
	}
}

func TestCaseInsensitiveHandles(t *testing.T) {
	st := newTestStore(t)
	st.Send("Alice", "Bob", "hi", "")

	in, err := st.Inbox("bob")
	if err != nil {
		t.Fatalf("inbox: %v", err)
	}
	if len(in) != 1 {
		t.Fatalf("case-insensitive inbox len = %d, want 1", len(in))
	}
	n, _ := st.UnreadCount("BOB")
	if n != 1 {
		t.Fatalf("case-insensitive unread = %d, want 1", n)
	}
	out, _ := st.Outbox("ALICE")
	if len(out) != 1 {
		t.Fatalf("case-insensitive outbox len = %d, want 1", len(out))
	}

	// Delete with a differently-cased handle still works for the owner.
	if err := st.Delete(in[0].ID, "bOb"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if m, _ := st.Get(in[0].ID); m != nil {
		t.Fatalf("case-insensitive owner delete should succeed")
	}
}
