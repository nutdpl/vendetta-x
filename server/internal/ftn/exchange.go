package ftn

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"vendetta-x/server/internal/binkp"
	"vendetta-x/server/internal/store"
)

// The exchange: the board as a node in FidoNet-technology networks (fsxNet,
// FidoNet, AgoraNet, ArakNet, ... -- as many at once as the sysop joins).
// Per enabled uplink: bundle locally-posted messages from mapped boards into
// a type 2+ packet, run a BinkP session with the hub (send ours, collect
// theirs), and import the inbound echomail with MSGID dedupe and reply
// threading. Fired by the scheduler's ftn.exchange action or the panel's
// run-now button; exchangeMu serializes the two.

var exchangeMu sync.Mutex

// Summary reports what one exchange pass did across all links.
type Summary struct {
	Links    int
	Exported int
	Imported int
	Skipped  int
	Errors   []string
}

func (s Summary) String() string {
	out := fmt.Sprintf("%d link(s): exported %d, imported %d, skipped %d",
		s.Links, s.Exported, s.Imported, s.Skipped)
	if len(s.Errors) > 0 {
		out += "; errors: " + strings.Join(s.Errors, " | ")
	}
	return out
}

// Exchange polls every enabled uplink. Individual link failures are
// collected, not fatal: one dead hub must not stop the others' mail.
func Exchange(st *store.Store, fs *Store) (Summary, error) {
	exchangeMu.Lock()
	defer exchangeMu.Unlock()

	var sum Summary
	links, err := fs.Links()
	if err != nil {
		return sum, err
	}
	for i := range links {
		if !links[i].Enabled {
			continue
		}
		sum.Links++
		exp, imp, skip, err := exchangeLink(st, fs, &links[i])
		sum.Exported += exp
		sum.Imported += imp
		sum.Skipped += skip
		if err != nil {
			sum.Errors = append(sum.Errors, links[i].Name+": "+err.Error())
		}
	}
	_ = st.SetSetting("ftn.last_status",
		time.Now().Format("2006-01-02 15:04")+" -- "+sum.String())
	return sum, nil
}

func exchangeLink(st *store.Store, fs *Store, link *Link) (exported, imported, skipped int, err error) {
	ourAddr, err := ParseAddress(link.OurAddr)
	if err != nil {
		return 0, 0, 0, err
	}
	hubAddr, err := ParseAddress(link.HubAddr)
	if err != nil {
		return 0, 0, 0, err
	}
	echoes, err := fs.Echoes(link.ID)
	if err != nil {
		return 0, 0, 0, err
	}

	boards, err := st.Boards()
	if err != nil {
		return 0, 0, 0, err
	}
	boardByTag := map[string]*store.Board{}
	for i := range boards {
		boardByTag[boards[i].Tag] = &boards[i]
	}
	boardName := st.Setting("board.name", "Vendetta/X")

	// ---- export: locally-posted messages above each echo's high-water mark.
	var outMsgs []Message
	pendingMark := map[int64]int64{} // echoID -> highest exported local msg id
	pendingSeen := map[string]int64{}
	for _, e := range echoes {
		bd := boardByTag[e.BoardTag]
		if bd == nil {
			continue
		}
		msgs, err := st.LocalMessagesAfter(bd.ID, e.LastExport)
		if err != nil {
			return 0, 0, 0, err
		}
		for _, m := range msgs {
			fm := Message{
				Area: e.Tag, From: m.From, To: m.To, Subject: m.Subject,
				Posted: m.Posted, Body: m.Body,
				MsgID: NewMsgID(ourAddr, m.ID, m.Posted),
			}
			// A reply to another local message carries its parent's
			// (deterministic) MSGID so threading survives the network.
			if m.ReplyTo != 0 {
				if parent, err := st.MessageByID(m.ReplyTo); err == nil && parent != nil && parent.Origin == "" {
					fm.ReplyID = NewMsgID(ourAddr, parent.ID, parent.Posted)
				}
			}
			outMsgs = append(outMsgs, fm)
			pendingSeen[fm.MsgID] = m.ID
			if m.ID > pendingMark[e.ID] {
				pendingMark[e.ID] = m.ID
			}
		}
	}

	var outbound []binkp.File
	if len(outMsgs) > 0 {
		pkt := WritePacket(ourAddr, hubAddr, link.Password, "Vendetta/X", boardName, outMsgs)
		outbound = append(outbound, binkp.File{
			Name: fmt.Sprintf("%08x.pkt", uint32(time.Now().Unix())),
			Data: pkt, Time: time.Now(),
		})
	}

	// ---- the BinkP session.
	inbound, err := binkp.Session(binkp.Config{
		HostPort: link.Host,
		OurAddr:  link.OurAddr,
		Password: link.Password,
		SysName:  boardName,
		Sysop:    st.Setting("board.sysop", "SysOp"),
	}, outbound)
	if err != nil {
		return 0, 0, 0, err
	}

	// The hub took our packet: advance the marks and remember our MSGIDs so
	// the hub echoing our own posts back never re-imports them.
	exported = len(outMsgs)
	for echoID, mark := range pendingMark {
		if err := fs.AdvanceExport(echoID, mark); err != nil {
			return exported, 0, 0, err
		}
	}
	for msgid, localID := range pendingSeen {
		_ = fs.MarkSeen(msgid, localID)
	}

	// ---- import: every inbound file is a packet or a zip bundle of packets.
	echoBoard := map[string]*store.Board{}
	for _, e := range echoes {
		if bd := boardByTag[e.BoardTag]; bd != nil {
			echoBoard[strings.ToUpper(e.Tag)] = bd
		}
	}
	for _, f := range inbound {
		for _, pkt := range unbundle(f) {
			imp, skip := importPacket(st, fs, link, pkt, echoBoard)
			imported += imp
			skipped += skip
		}
	}
	return exported, imported, skipped, nil
}

// unbundle returns the packets inside an inbound file: the file itself when
// it's a bare .pkt, its members when it's a zip-compressed mail bundle
// (what binkd-era hubs send), nothing when it's some other artifact.
func unbundle(f binkp.File) [][]byte {
	if bytes.HasPrefix(f.Data, []byte("PK\x03\x04")) {
		zr, err := zip.NewReader(bytes.NewReader(f.Data), int64(len(f.Data)))
		if err != nil {
			log.Printf("ftn: unreadable bundle %s: %v", f.Name, err)
			return nil
		}
		var pkts [][]byte
		for _, zf := range zr.File {
			rc, err := zf.Open()
			if err != nil {
				continue
			}
			data, err := io.ReadAll(io.LimitReader(rc, 32<<20))
			rc.Close()
			if err == nil && len(data) > 58 {
				pkts = append(pkts, data)
			}
		}
		return pkts
	}
	if len(f.Data) > 58 {
		return [][]byte{f.Data}
	}
	return nil
}

// importPacket posts a packet's echomail into the mapped boards.
func importPacket(st *store.Store, fs *Store, link *Link, pkt []byte, echoBoard map[string]*store.Board) (imported, skipped int) {
	msgs, _, err := ParsePacket(pkt)
	if err != nil {
		log.Printf("ftn %s: bad packet: %v", link.Name, err)
		return 0, 0
	}
	for _, m := range msgs {
		bd := echoBoard[m.Area]
		if bd == nil {
			skipped++ // echo we don't carry
			continue
		}
		if seen, _, err := fs.Seen(m.MsgID); err != nil || seen {
			skipped++
			continue
		}
		// MSGID-less messages fall back to the content dupe check.
		if m.MsgID == "" {
			if dup, err := st.HasMessage(bd.ID, m.From, m.Subject, m.Posted); err != nil || dup {
				skipped++
				continue
			}
		}
		var replyTo int64
		if m.ReplyID != "" {
			if seen, localID, err := fs.Seen(m.ReplyID); err == nil && seen {
				replyTo = localID
			}
		}
		id, err := st.PostMessage(&store.Message{
			BoardID: bd.ID,
			From:    m.From, To: m.To, Subject: m.Subject, Body: m.Body,
			Posted: m.Posted, Origin: link.Name, ReplyTo: replyTo,
		})
		if err != nil {
			log.Printf("ftn %s: import post: %v", link.Name, err)
			skipped++
			continue
		}
		_ = fs.MarkSeen(m.MsgID, id)
		imported++
	}
	return imported, skipped
}
