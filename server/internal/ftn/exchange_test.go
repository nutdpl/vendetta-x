package ftn

import (
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"vendetta-x/server/internal/store"
)

// fakeHub speaks just enough server-side BinkP to drive Exchange: handshake,
// receive the board's packet, then answer with a packet of its own built
// AROUND what it received -- an echo-back copy of the board's message (the
// loop test), a fresh reply from a remote user (the import + threading
// test), and traffic in an echo the board doesn't carry (the skip test).
type fakeHub struct {
	ln       net.Listener
	password string
	received []Message
	done     chan struct{}
	err      error
}

func startExchangeHub(t *testing.T, password string) *fakeHub {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	h := &fakeHub{ln: ln, password: password, done: make(chan struct{})}
	go h.serve()
	t.Cleanup(func() { ln.Close() })
	return h
}

func (h *fakeHub) serve() {
	defer close(h.done)
	conn, err := h.ln.Accept()
	if err != nil {
		h.err = err
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))

	wcmd := func(c byte, arg string) {
		p := append([]byte{c}, arg...)
		var hd [2]byte
		binary.BigEndian.PutUint16(hd[:], uint16(len(p))|0x8000)
		conn.Write(hd[:])
		conn.Write(p)
	}
	wdata := func(b []byte) {
		var hd [2]byte
		binary.BigEndian.PutUint16(hd[:], uint16(len(b)))
		conn.Write(hd[:])
		conn.Write(b)
	}
	read := func() (byte, []byte, bool) {
		var hd [2]byte
		if _, err := io.ReadFull(conn, hd[:]); err != nil {
			return 0, nil, false
		}
		v := binary.BigEndian.Uint16(hd[:])
		buf := make([]byte, v&0x7FFF)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return 0, nil, false
		}
		if v&0x8000 != 0 {
			return buf[0], buf[1:], true
		}
		return 0xFF, buf, true
	}

	wcmd(0, "SYS hub")
	wcmd(1, "21:1/100")

	// Handshake + receive the board's files until its EOB.
	var pktData []byte
	var curSize int64
	inFile := false
recv:
	for {
		c, payload, ok := read()
		if !ok {
			h.err = io.ErrUnexpectedEOF
			return
		}
		switch c {
		case 2: // M_PWD
			if string(payload) != h.password {
				wcmd(5, "bad password")
				return
			}
			wcmd(4, "secure")
		case 3: // M_FILE
			f := strings.Fields(string(payload))
			curSize, _ = strconv.ParseInt(f[1], 10, 64)
			pktData = nil
			inFile = true
			if curSize == 0 {
				inFile = false
			}
		case 0xFF:
			if inFile {
				pktData = append(pktData, payload...)
				if int64(len(pktData)) >= curSize {
					wcmd(8, "got") // M_GOT
					inFile = false
				}
			}
		case 10: // M_EOB
			break recv
		}
	}

	if len(pktData) > 0 {
		msgs, _, err := ParsePacket(pktData)
		if err != nil {
			h.err = err
			return
		}
		h.received = msgs
	}

	// Build the response packet around what arrived.
	hub, _ := ParseAddress("21:1/100")
	us, _ := ParseAddress("21:1/999")
	now := time.Now().Truncate(time.Second)
	var out []Message
	if len(h.received) > 0 {
		orig := h.received[0]
		// 1. the board's own message echoed back (must be dropped by MSGID)
		out = append(out, orig)
		// 2. a fresh remote reply to it (must import, threaded)
		out = append(out, Message{
			Area: orig.Area, From: "remote rider", To: orig.From,
			Subject: "Re: " + orig.Subject, Posted: now,
			Body:  orig.From + "> " + strings.SplitN(orig.Body, "\n", 2)[0] + "\n\ngreetings from the hub",
			MsgID: "21:1/100 cafe0001", ReplyID: orig.MsgID,
		})
	}
	// 3. traffic in an echo the board doesn't carry (must be skipped)
	out = append(out, Message{
		Area: "FSX_NOPE", From: "nobody", To: "All", Subject: "elsewhere",
		Posted: now, Body: "not carried", MsgID: "21:1/100 cafe0002",
	})
	resp := WritePacket(hub, us, h.password, "hub", "the hub", out)

	wcmd(3, "0000resp.pkt "+strconv.Itoa(len(resp))+" "+strconv.FormatInt(now.Unix(), 10)+" 0")
	wdata(resp)
	wcmd(10, "")
	for {
		c, _, ok := read()
		if !ok || c == 8 || c == 10 { // M_GOT / their EOB re-read
			return
		}
	}
}

func TestExchangeEndToEnd(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	defer st.Close()
	if err := st.Seed(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	fs, err := NewStore(st.DB())
	if err != nil {
		t.Fatalf("ftn store: %v", err)
	}

	hub := startExchangeHub(t, "s3cret")

	link := Link{
		Name: "fsxNet", Host: hub.ln.Addr().String(),
		OurAddr: "21:1/999", HubAddr: "21:1/100",
		Password: "s3cret", Enabled: true,
	}
	if err := fs.SaveLink(&link); err != nil {
		t.Fatalf("SaveLink: %v", err)
	}
	boards, _ := st.Boards()
	genTag := boards[0].Tag
	if err := fs.SetEchoes(link.ID, map[string]string{"FSX_GEN": genTag}); err != nil {
		t.Fatalf("SetEchoes: %v", err)
	}

	// Catch the high-water up past the seed content, then post fresh mail.
	if msgs, _ := st.Messages(boards[0].ID, 1); len(msgs) > 0 {
		echoes, _ := fs.Echoes(link.ID)
		_ = fs.AdvanceExport(echoes[0].ID, msgs[0].ID)
	}
	localID, err := st.PostMessage(&store.Message{
		BoardID: boards[0].ID, From: "phantom", To: "All",
		Subject: "hail the network", Body: "first post to fsxnet", Posted: time.Now(),
	})
	if err != nil {
		t.Fatalf("post: %v", err)
	}

	sum, err := Exchange(st, fs)
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	<-hub.done
	if hub.err != nil {
		t.Fatalf("hub: %v", hub.err)
	}

	// Outbound: exactly our fresh local post, properly dressed.
	if len(hub.received) != 1 || hub.received[0].Subject != "hail the network" {
		t.Fatalf("hub received %+v", hub.received)
	}
	if hub.received[0].Area != "FSX_GEN" || !strings.HasPrefix(hub.received[0].MsgID, "21:1/999 ") {
		t.Fatalf("export dressing: %+v", hub.received[0])
	}

	// Summary: 1 out, 1 in (the reply), 2 skipped (echo-back + unmapped echo).
	if sum.Exported != 1 || sum.Imported != 1 || sum.Skipped != 2 {
		t.Fatalf("summary = %+v", sum)
	}

	// The import landed threaded onto our local message, origin-tagged.
	msgs, _ := st.Messages(boards[0].ID, 5)
	var reply *store.Message
	for i := range msgs {
		if msgs[i].From == "remote rider" {
			reply = &msgs[i]
		}
	}
	if reply == nil {
		t.Fatal("imported reply missing")
	}
	if reply.Origin != "fsxNet" {
		t.Fatalf("origin = %q", reply.Origin)
	}
	if reply.ReplyTo != localID {
		t.Fatalf("threading: ReplyTo = %d, want %d", reply.ReplyTo, localID)
	}

	// Second exchange: nothing new to export (high-water advanced, and the
	// imported reply has a non-blank Origin so it can never echo back out).
	hub2 := startExchangeHub(t, "s3cret")
	l2, _ := fs.LinkByID(link.ID)
	l2.Host = hub2.ln.Addr().String()
	if err := fs.SaveLink(l2); err != nil {
		t.Fatalf("relink: %v", err)
	}
	sum2, err := Exchange(st, fs)
	if err != nil {
		t.Fatalf("Exchange 2: %v", err)
	}
	if sum2.Exported != 0 {
		t.Fatalf("second exchange re-exported %d", sum2.Exported)
	}
	// The hub's canned unmapped-echo message dedupes by MSGID this time too.
	if sum2.Imported != 0 {
		t.Fatalf("second exchange re-imported %d", sum2.Imported)
	}
}
