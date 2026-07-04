package ftn

import (
	"strings"
	"testing"
	"time"
)

func TestAddressRoundTrip(t *testing.T) {
	cases := map[string]Address{
		"21:1/100":     {21, 1, 100, 0},
		"1:2320/105.7": {1, 2320, 105, 7},
	}
	for s, want := range cases {
		got, err := ParseAddress(s)
		if err != nil || got != want {
			t.Fatalf("ParseAddress(%q) = %+v, %v; want %+v", s, got, err, want)
		}
		if got.String() != s {
			t.Fatalf("String() = %q, want %q", got.String(), s)
		}
	}
	for _, bad := range []string{"", "1/100", "21:1", "x:y/z", "1:2/3.4.5x"} {
		if _, err := ParseAddress(bad); err == nil {
			t.Fatalf("ParseAddress(%q) accepted", bad)
		}
	}
}

func TestPacketRoundTrip(t *testing.T) {
	us, _ := ParseAddress("21:1/999")
	hub, _ := ParseAddress("21:1/100")
	posted := time.Date(2026, 2, 27, 21, 30, 5, 0, time.Local)

	out := []Message{
		{
			Area: "FSX_GEN", From: "phantom", To: "All", Subject: "hail fsxnet",
			Posted: posted, Body: "first line\n\nthird line",
			MsgID: NewMsgID(us, 42, posted),
		},
		{
			Area: "FSX_GEN", From: "nut", To: "phantom", Subject: "Re: hail fsxnet",
			Posted: posted.Add(time.Minute), Body: "phantom> first line\n\nagreed",
			MsgID:   NewMsgID(us, 43, posted.Add(time.Minute)),
			ReplyID: NewMsgID(us, 42, posted),
		},
	}

	pkt := WritePacket(us, hub, "s3cret", "Vendetta/X", "the wall remembers", out)
	got, password, err := ParsePacket(pkt)
	if err != nil {
		t.Fatalf("ParsePacket: %v", err)
	}
	if password != "s3cret" {
		t.Fatalf("password = %q", password)
	}
	if len(got) != 2 {
		t.Fatalf("parsed %d messages, want 2", len(got))
	}

	m := got[0]
	if m.Area != "FSX_GEN" || m.From != "phantom" || m.To != "All" || m.Subject != "hail fsxnet" {
		t.Fatalf("header round-trip: %+v", m)
	}
	if !strings.HasPrefix(m.Body, "first line\n\nthird line") {
		t.Fatalf("body round-trip: %q", m.Body)
	}
	// The tearline and origin travel as visible body; SEEN-BY and kludges don't.
	if !strings.Contains(m.Body, "--- Vendetta/X") || !strings.Contains(m.Body, "* Origin: the wall remembers (21:1/999)") {
		t.Fatalf("tearline/origin missing: %q", m.Body)
	}
	if strings.Contains(m.Body, "SEEN-BY") || strings.Contains(m.Body, "\x01") {
		t.Fatalf("control lines leaked into body: %q", m.Body)
	}
	if m.MsgID != out[0].MsgID {
		t.Fatalf("MsgID = %q, want %q", m.MsgID, out[0].MsgID)
	}
	if got[1].ReplyID != out[0].MsgID {
		t.Fatalf("ReplyID = %q, want %q", got[1].ReplyID, out[0].MsgID)
	}
	if !got[0].Posted.Equal(posted) {
		t.Fatalf("Posted = %v, want %v", got[0].Posted, posted)
	}
}

func TestParseSkipsNetmail(t *testing.T) {
	us, _ := ParseAddress("21:1/999")
	hub, _ := ParseAddress("21:1/100")
	pkt := WritePacket(us, hub, "", "VX", "o", []Message{
		{Area: "FSX_TST", From: "a", To: "b", Subject: "s", Posted: time.Now(), MsgID: "x 1"},
	})
	// Splice a netmail-looking message by clearing its AREA line: simplest is
	// to parse a hand-built packet where the text has no AREA prefix.
	msgs, _, err := ParsePacket(pkt)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("baseline parse: %d, %v", len(msgs), err)
	}
	if _, ok := parseEchomail("a", "b", "s", "27 Feb 26  21:30:05", "\x01MSGID: x 2\rjust netmail\r"); ok {
		t.Fatal("netmail (no AREA) treated as echomail")
	}
}

func TestNewMsgIDUnique(t *testing.T) {
	us, _ := ParseAddress("21:1/999")
	now := time.Now()
	a := NewMsgID(us, 1, now)
	b := NewMsgID(us, 2, now)
	if a == b {
		t.Fatalf("serials collide: %q", a)
	}
	if !strings.HasPrefix(a, "21:1/999 ") || len(strings.Fields(a)) != 2 {
		t.Fatalf("MSGID shape: %q", a)
	}
}
