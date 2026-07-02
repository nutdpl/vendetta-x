package qwknet

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"vendetta-x/server/internal/qwk"
	"vendetta-x/server/internal/store"
)

// fakeHub is an in-process FTP server playing the hub role over real TCP:
// it serves THEHUB.QWK from a byte slice and records whatever REP we upload.
type fakeHub struct {
	ln    net.Listener
	mu    sync.Mutex
	files map[string][]byte
}

func newFakeHub(t *testing.T) *fakeHub {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	h := &fakeHub{ln: ln, files: map[string][]byte{}}
	go h.serve()
	t.Cleanup(func() { ln.Close() })
	return h
}

func (h *fakeHub) put(name string, b []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.files[name] = b
}

func (h *fakeHub) get(name string) ([]byte, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	b, ok := h.files[name]
	return b, ok
}

func (h *fakeHub) serve() {
	for {
		conn, err := h.ln.Accept()
		if err != nil {
			return
		}
		go h.session(conn)
	}
}

func (h *fakeHub) session(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	reply := func(s string) { fmt.Fprintf(conn, "%s\r\n", s) }
	reply("220 hub ready")

	var dataLn net.Listener
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		verb, arg, _ := strings.Cut(strings.TrimRight(line, "\r\n"), " ")
		switch strings.ToUpper(verb) {
		case "USER":
			reply("331 pass?")
		case "PASS":
			reply("230 in")
		case "TYPE":
			reply("200 ok")
		case "PASV":
			if dataLn != nil {
				dataLn.Close()
			}
			dataLn, _ = net.Listen("tcp", "127.0.0.1:0")
			port := dataLn.Addr().(*net.TCPAddr).Port
			reply(fmt.Sprintf("227 (127,0,0,1,%d,%d)", port>>8, port&0xFF))
		case "RETR":
			body, ok := h.get(arg)
			if !ok {
				reply("550 not found")
				continue
			}
			reply("150 sending")
			dc, _ := dataLn.Accept()
			dc.Write(body)
			dc.Close()
			reply("226 done")
		case "STOR":
			reply("150 send it")
			dc, _ := dataLn.Accept()
			body, _ := io.ReadAll(dc)
			dc.Close()
			h.put(arg, body)
			reply("226 stored")
		case "DELE":
			h.mu.Lock()
			_, existed := h.files[arg]
			delete(h.files, arg)
			h.mu.Unlock()
			if existed {
				reply("250 deleted")
			} else {
				reply("550 not found")
			}
		case "QUIT":
			reply("221 bye")
			return
		default:
			reply("502 nope")
		}
	}
}

// newNetBoard builds a store with one networked board and a full qwknet
// configuration pointing at the fake hub.
func newNetBoard(t *testing.T, hub *fakeHub) (*store.Store, int64) {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	boardID, err := st.AddBoard(&store.Board{Tag: "netgen", Name: "Net General"})
	if err != nil {
		t.Fatalf("AddBoard: %v", err)
	}

	set := func(k, v string) {
		if err := st.SetSetting(k, v); err != nil {
			t.Fatalf("SetSetting(%s): %v", k, err)
		}
	}
	set(KeyEnabled, "1")
	set(KeyHost, hub.ln.Addr().String())
	set(KeyUser, "vendx")
	set(KeyPass, "pw")
	set(KeyHubID, "THEHUB")
	set(KeyNetName, "TESTNET")
	set(KeyConfMap, "# test network\n2001 = netgen\n")
	return st, boardID
}

// hubPacket builds the QWK packet the hub will serve.
func hubPacket(t *testing.T, msgs []qwk.Message) []byte {
	t.Helper()
	data, err := qwk.Build(qwk.Packet{
		BoardName:   "The Hub",
		Sysop:       "hubop",
		Caller:      "VENDX",
		Conferences: []qwk.Conference{{Number: 2001, Name: "NET_GENERAL"}},
		Messages:    msgs,
	})
	if err != nil {
		t.Fatalf("hub Build: %v", err)
	}
	return data
}

func TestExchangeRoundTrip(t *testing.T) {
	hub := newFakeHub(t)
	st, boardID := newNetBoard(t, hub)

	// Local traffic to export: one local post, one previously-imported post
	// that must NOT loop back out.
	when := time.Date(2026, 7, 1, 8, 0, 0, 0, time.Local)
	if _, err := st.PostMessage(&store.Message{
		BoardID: boardID, From: "nut", To: "All", Subject: "hello net",
		Body: "greetings from vendetta/x", Posted: when,
	}); err != nil {
		t.Fatalf("post local: %v", err)
	}
	if _, err := st.PostMessage(&store.Message{
		BoardID: boardID, From: "old remote", To: "All", Subject: "already here",
		Body: "imported last week", Posted: when, Origin: "TESTNET",
	}); err != nil {
		t.Fatalf("post imported: %v", err)
	}

	// Hub-side traffic to import.
	remoteDate := time.Date(2026, 6, 30, 21, 15, 0, 0, time.Local)
	hub.put("THEHUB.QWK", hubPacket(t, []qwk.Message{
		{Conference: 2001, To: "All", From: "remote gal", Subject: "news from the net",
			Body: "line one\nline two", Date: remoteDate},
		{Conference: 9999, To: "All", From: "nobody", Subject: "unmapped",
			Body: "should be skipped", Date: remoteDate},
	}))

	sum, err := Exchange(st)
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if sum.Exported != 1 || sum.Imported != 1 || sum.Skipped != 1 {
		t.Fatalf("summary = %+v, want exported 1, imported 1, skipped 1", sum)
	}

	// The uploaded REP must exist, name the hub, and carry ONLY the local post.
	rep, ok := hub.get("THEHUB.REP")
	if !ok {
		t.Fatal("hub never received THEHUB.REP")
	}
	// Parse it the way a hub would (it's MESSAGES.DAT layout inside a zip).
	parsed, err := qwk.ParseReplyFor("THEHUB", rep)
	if err != nil {
		t.Fatalf("parse uploaded REP: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("REP carries %d messages, want 1 (imported post must not loop back)", len(parsed))
	}
	if parsed[0].From != "nut" || parsed[0].Conference != 2001 ||
		parsed[0].Body != "greetings from vendetta/x" {
		t.Fatalf("REP msg = %+v", parsed[0])
	}

	// The imported message must be on the board with author/date/origin intact.
	msgs, _ := st.Messages(boardID, 0)
	var imported *store.Message
	for i := range msgs {
		if msgs[i].From == "remote gal" {
			imported = &msgs[i]
		}
	}
	if imported == nil {
		t.Fatal("network message was not imported")
	}
	if imported.Origin != "TESTNET" {
		t.Errorf("imported origin = %q, want TESTNET", imported.Origin)
	}
	if !imported.Posted.Equal(remoteDate) {
		t.Errorf("imported date = %v, want %v", imported.Posted, remoteDate)
	}
	if imported.Body != "line one\nline two" {
		t.Errorf("imported body = %q", imported.Body)
	}

	// The consumed packet must be deleted from the hub.
	if _, ok := hub.get("THEHUB.QWK"); ok {
		t.Error("THEHUB.QWK still on hub after import")
	}

	// Status was recorded.
	if s := LastStatus(st); !strings.Contains(s, "exported 1") {
		t.Errorf("last status = %q", s)
	}

	// ---- second exchange: nothing new anywhere ----
	hub.put("THEHUB.QWK", hubPacket(t, []qwk.Message{
		{Conference: 2001, To: "All", From: "remote gal", Subject: "news from the net",
			Body: "line one\nline two", Date: remoteDate}, // exact dupe
	}))
	sum2, err := Exchange(st)
	if err != nil {
		t.Fatalf("second Exchange: %v", err)
	}
	if sum2.Exported != 0 {
		t.Errorf("second exchange re-exported %d messages (high-water mark broken)", sum2.Exported)
	}
	if sum2.Imported != 0 || sum2.Skipped != 1 {
		t.Errorf("second exchange = %+v, want dupe skipped", sum2)
	}
	if _, ok := hub.get("THEHUB.REP"); ok && sum2.Exported == 0 {
		// First REP was consumed conceptually; a zero-export exchange must not
		// upload an empty packet over it. (The file map still holds the first
		// REP; assert it wasn't replaced by an empty one.)
		rep2, _ := hub.get("THEHUB.REP")
		if p, err := qwk.ParseReplyFor("THEHUB", rep2); err != nil || len(p) != 1 {
			t.Errorf("REP was overwritten on a zero-export exchange")
		}
	}
}

func TestExchangeDisabled(t *testing.T) {
	hub := newFakeHub(t)
	st, _ := newNetBoard(t, hub)
	st.SetSetting(KeyEnabled, "0")
	if _, err := Exchange(st); err == nil {
		t.Fatal("disabled exchange should error")
	}
}

func TestConfMapParsing(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	boardID, _ := st.AddBoard(&store.Board{Tag: "gen", Name: "General"})

	set := func(v string) { st.SetSetting(KeyConfMap, v) }

	set("# comment\n\n1 = gen\n")
	m, err := ConfMap(st)
	if err != nil || len(m) != 1 || m[1] != boardID {
		t.Fatalf("ConfMap = %v, %v", m, err)
	}

	set("1 = nosuchtag")
	if _, err := ConfMap(st); err == nil {
		t.Fatal("unknown tag accepted")
	}
	set("notanumber = gen")
	if _, err := ConfMap(st); err == nil {
		t.Fatal("bad conference number accepted")
	}
	set("garbage line")
	if _, err := ConfMap(st); err == nil {
		t.Fatal("unparseable line accepted")
	}
}
