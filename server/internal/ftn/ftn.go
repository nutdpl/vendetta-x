// Package ftn speaks FidoNet Technology Network echomail: the packet format
// (FTS-0001 type 2, with the FSC-0039 "type 2+" zone/point extensions) and
// the echomail conventions layered on it (FTS-0004: AREA tags, MSGID/REPLY
// kludges, tearline, origin line, SEEN-BY and PATH control lines).
//
// One implementation covers every FTN network -- FidoNet itself, fsxNet,
// AgoraNet, ArakNet, and the rest of the family -- because they differ only
// in zone numbers and uplinks, never in wire format. Pure logic, no I/O:
// the binkp package moves the bytes, server_ftn.go runs the exchange.
package ftn

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Address is a 4D FTN address: zone:net/node.point ("21:1/100" reads as
// zone 21, net 1, node 100, point 0).
type Address struct {
	Zone, Net, Node, Point int
}

// ParseAddress accepts zone:net/node[.point].
func ParseAddress(s string) (Address, error) {
	var a Address
	s = strings.TrimSpace(s)
	zoneRest := strings.SplitN(s, ":", 2)
	if len(zoneRest) != 2 {
		return a, fmt.Errorf("ftn: address %q needs zone:net/node", s)
	}
	netNode := strings.SplitN(zoneRest[1], "/", 2)
	if len(netNode) != 2 {
		return a, fmt.Errorf("ftn: address %q needs zone:net/node", s)
	}
	nodePoint := strings.SplitN(netNode[1], ".", 2)
	var err error
	if a.Zone, err = strconv.Atoi(zoneRest[0]); err != nil {
		return a, fmt.Errorf("ftn: bad zone in %q", s)
	}
	if a.Net, err = strconv.Atoi(netNode[0]); err != nil {
		return a, fmt.Errorf("ftn: bad net in %q", s)
	}
	if a.Node, err = strconv.Atoi(nodePoint[0]); err != nil {
		return a, fmt.Errorf("ftn: bad node in %q", s)
	}
	if len(nodePoint) == 2 {
		if a.Point, err = strconv.Atoi(nodePoint[1]); err != nil {
			return a, fmt.Errorf("ftn: bad point in %q", s)
		}
	}
	if a.Zone < 0 || a.Net < 0 || a.Node < 0 || a.Point < 0 ||
		a.Zone > 0xFFFF || a.Net > 0xFFFF || a.Node > 0xFFFF || a.Point > 0xFFFF {
		return a, fmt.Errorf("ftn: address %q out of range", s)
	}
	return a, nil
}

func (a Address) String() string {
	if a.Point != 0 {
		return fmt.Sprintf("%d:%d/%d.%d", a.Zone, a.Net, a.Node, a.Point)
	}
	return fmt.Sprintf("%d:%d/%d", a.Zone, a.Net, a.Node)
}

// Message is one echomail message, composed for export or parsed from an
// inbound packet.
type Message struct {
	Area    string // echo tag, e.g. FSX_GEN
	From    string // author handle
	To      string // recipient handle ("All" for echomail)
	Subject string
	Posted  time.Time
	Body    string // visible text, LF line endings

	// MsgID is the ^AMSGID kludge value ("addr serial"), the network-wide
	// duplicate key. ReplyID is the ^AREPLY value (the parent's MsgID).
	MsgID   string
	ReplyID string
}

// datetime is FTS-0001's fixed 19-char format, e.g. "27 Feb 26  21:30:05".
const datetime = "02 Jan 06  15:04:05"

// ---- packet writing ---------------------------------------------------------

// WritePacket builds a type 2+ packet from orig to dest carrying msgs as
// echomail. tearline names the software; originText goes on each message's
// "* Origin:" line (the board name); password is the packet password ("" ok).
func WritePacket(orig, dest Address, password, tearline, originText string, msgs []Message) []byte {
	var b bytes.Buffer
	now := time.Now()

	w16 := func(v int) { _ = binary.Write(&b, binary.LittleEndian, uint16(v)) }

	// FTS-0001 header, FSC-0039 fields filled in.
	w16(orig.Node)
	w16(dest.Node)
	w16(now.Year())
	w16(int(now.Month()) - 1)
	w16(now.Day())
	w16(now.Hour())
	w16(now.Minute())
	w16(now.Second())
	w16(0) // baud
	w16(2) // packet type
	w16(orig.Net)
	w16(dest.Net)
	b.WriteByte(0xFE) // prodCode low: 0xFE = no registered code
	b.WriteByte(0)    // serial / major revision
	var pw [8]byte
	copy(pw[:], password)
	b.Write(pw[:])
	w16(orig.Zone) // qOrigZone (FTS-0001 position)
	w16(dest.Zone) // qDestZone
	w16(0)         // auxNet
	b.WriteByte(0)
	b.WriteByte(1) // capValidation = byte-swapped capWord 0x0001
	b.WriteByte(0) // prodCode high
	b.WriteByte(0) // minor revision
	w16(1)         // capWord: type 2+
	w16(orig.Zone)
	w16(dest.Zone)
	w16(orig.Point)
	w16(dest.Point)
	b.Write([]byte{0, 0, 0, 0}) // prodData

	for i := range msgs {
		writeMessage(&b, orig, dest, tearline, originText, &msgs[i])
	}
	w16(0) // packet terminator
	return b.Bytes()
}

func writeMessage(b *bytes.Buffer, orig, dest Address, tearline, originText string, m *Message) {
	w16 := func(v int) { _ = binary.Write(b, binary.LittleEndian, uint16(v)) }
	w16(2) // message type
	w16(orig.Node)
	w16(dest.Node)
	w16(orig.Net)
	w16(dest.Net)
	w16(0x0100) // attribute: Local
	w16(0)      // cost

	b.WriteString(m.Posted.Format(datetime))
	b.WriteByte(0)
	writeZStr(b, m.To, 35)
	writeZStr(b, m.From, 35)
	writeZStr(b, m.Subject, 71)

	// Echomail text: AREA + kludges + body + tearline + origin + control.
	var t bytes.Buffer
	t.WriteString("AREA:" + strings.ToUpper(m.Area) + "\r")
	t.WriteString("\x01MSGID: " + m.MsgID + "\r")
	if m.ReplyID != "" {
		t.WriteString("\x01REPLY: " + m.ReplyID + "\r")
	}
	t.WriteString("\x01TZUTC: " + m.Posted.Format("-0700") + "\r")
	for _, ln := range strings.Split(strings.ReplaceAll(m.Body, "\r\n", "\n"), "\n") {
		t.WriteString(ln + "\r")
	}
	t.WriteString("--- " + tearline + "\r")
	t.WriteString(" * Origin: " + originText + " (" + orig.String() + ")\r")
	t.WriteString(fmt.Sprintf("SEEN-BY: %d/%d %d/%d\r", orig.Net, orig.Node, dest.Net, dest.Node))
	t.WriteString(fmt.Sprintf("\x01PATH: %d/%d\r", orig.Net, orig.Node))

	b.Write(t.Bytes())
	b.WriteByte(0)
}

func writeZStr(b *bytes.Buffer, s string, max int) {
	if len(s) > max {
		s = s[:max]
	}
	b.WriteString(s)
	b.WriteByte(0)
}

// NewMsgID builds a MSGID kludge value for a locally-posted message: our
// address plus an 8-hex-digit serial that never repeats for this board
// (seconds since epoch mixed with the message's own row id).
func NewMsgID(addr Address, localID int64, posted time.Time) string {
	serial := uint32(posted.Unix())*31 + uint32(localID)
	return fmt.Sprintf("%s %08x", addr.String(), serial)
}

// ---- packet parsing ---------------------------------------------------------

// ParsePacket reads a type 2 packet and returns its echomail messages.
// Non-echomail entries (no AREA line: netmail) are skipped. Password
// checking is the caller's business (the header password is returned).
func ParsePacket(data []byte) (msgs []Message, password string, err error) {
	if len(data) < 58 {
		return nil, "", fmt.Errorf("ftn: packet too short (%d bytes)", len(data))
	}
	if binary.LittleEndian.Uint16(data[18:20]) != 2 {
		return nil, "", fmt.Errorf("ftn: not a type-2 packet")
	}
	password = strings.TrimRight(string(bytes.TrimRight(data[26:34], "\x00")), " ")

	p := 58
	for {
		if p+2 > len(data) {
			return msgs, password, fmt.Errorf("ftn: truncated packet")
		}
		mtype := binary.LittleEndian.Uint16(data[p : p+2])
		if mtype == 0 {
			return msgs, password, nil // clean terminator
		}
		if mtype != 2 {
			return msgs, password, fmt.Errorf("ftn: unknown message type %d", mtype)
		}
		p += 14 // type + orig/dest node + orig/dest net + attr + cost

		if p+20 > len(data) {
			return msgs, password, fmt.Errorf("ftn: truncated message header")
		}
		dt := string(bytes.TrimRight(data[p:p+20], "\x00"))
		p += 20

		var to, from, subj, text string
		for _, dst := range []*string{&to, &from, &subj, &text} {
			nul := bytes.IndexByte(data[p:], 0)
			if nul < 0 {
				return msgs, password, fmt.Errorf("ftn: unterminated field")
			}
			*dst = string(data[p : p+nul])
			p += nul + 1
		}

		if m, ok := parseEchomail(from, to, subj, dt, text); ok {
			msgs = append(msgs, m)
		}
	}
}

// parseEchomail digs the echomail semantics out of a packed message's text.
// ok=false means netmail (no AREA tag) -- not ours to carry.
func parseEchomail(from, to, subj, dt, text string) (Message, bool) {
	m := Message{From: from, To: to, Subject: subj}
	if t, err := time.ParseInLocation(datetime, strings.TrimSpace(dt), time.Local); err == nil {
		m.Posted = t
	} else {
		m.Posted = time.Now()
	}

	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\r"), "\n", "\r"), "\r")
	var body []string
	for i, ln := range lines {
		switch {
		case i == 0 && strings.HasPrefix(ln, "AREA:"):
			m.Area = strings.ToUpper(strings.TrimSpace(strings.TrimPrefix(ln, "AREA:")))
		case strings.HasPrefix(ln, "\x01"):
			k := strings.TrimPrefix(ln, "\x01")
			if v, ok := strings.CutPrefix(k, "MSGID: "); ok {
				m.MsgID = strings.TrimSpace(v)
			} else if v, ok := strings.CutPrefix(k, "REPLY: "); ok {
				m.ReplyID = strings.TrimSpace(v)
			}
			// other kludges (PID, TZUTC, CHRS, ...) are dropped
		case strings.HasPrefix(ln, "SEEN-BY: "):
			// control info, not body
		default:
			body = append(body, ln)
		}
	}
	if m.Area == "" {
		return m, false // netmail
	}
	// Trim trailing blank lines the \r-splitting tends to leave.
	for len(body) > 0 && strings.TrimSpace(body[len(body)-1]) == "" {
		body = body[:len(body)-1]
	}
	m.Body = strings.Join(body, "\n")
	return m, true
}
