package qwk

import (
	"archive/zip"
	"bytes"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"
)

// unzip reads a ZIP byte slice into a name->bytes map for inspection.
func unzip(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	out := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", f.Name, err)
		}
		out[f.Name] = b
	}
	return out
}

func TestBuildSanity(t *testing.T) {
	p := Packet{
		BoardName: "Vendetta/X",
		Sysop:     "nut",
		Caller:    "phantom",
		Conferences: []Conference{
			{Number: 1, Name: "General"},
			{Number: 2, Name: "Warez Talk"},
		},
		Messages: []Message{
			{Conference: 1, To: "All", From: "nut", Subject: "Welcome", Body: "Hello there\nsecond line", Date: time.Now()},
			{Conference: 2, To: "All", From: "phantom", Subject: "First post", Body: "Short.", Date: time.Now(), Read: true},
		},
	}
	data, err := Build(p)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	files := unzip(t, data)
	control, ok := files["CONTROL.DAT"]
	if !ok {
		t.Fatal("missing CONTROL.DAT")
	}
	messages, ok := files["MESSAGES.DAT"]
	if !ok {
		t.Fatal("missing MESSAGES.DAT")
	}

	// CONTROL.DAT sanity: CRLF lines, board name, BBSID serial, message total,
	// and each conference number+name present.
	cs := string(control)
	if !strings.Contains(cs, "\r\n") {
		t.Error("CONTROL.DAT not CRLF terminated")
	}
	lines := strings.Split(cs, "\r\n")
	if lines[0] != "Vendetta/X" {
		t.Errorf("board name line = %q", lines[0])
	}
	if !strings.Contains(cs, ","+BBSID) {
		t.Errorf("CONTROL.DAT missing serial,%s line", BBSID)
	}
	// Line 9 is the "0" marker, line 10 the real message total, line 11 the
	// highest conference number.
	if lines[9] != "0" {
		t.Errorf("expected '0' marker line, got %q", lines[9])
	}
	if lines[10] != "2" {
		t.Errorf("total messages = %q, want 2", lines[10])
	}
	if lines[11] != "2" {
		t.Errorf("max conference = %q, want 2", lines[11])
	}
	if !strings.Contains(cs, "General") || !strings.Contains(cs, "Warez Talk") {
		t.Error("CONTROL.DAT missing conference names")
	}

	// MESSAGES.DAT must be a whole number of 128-byte blocks, starting with the
	// ident record.
	if len(messages)%blockSize != 0 {
		t.Errorf("MESSAGES.DAT len %d not a multiple of %d", len(messages), blockSize)
	}
	if len(messages) < blockSize {
		t.Fatal("MESSAGES.DAT has no ident block")
	}
	ident := messages[:blockSize]
	if !strings.HasPrefix(strings.TrimSpace(string(ident)), "Vendetta/X") {
		t.Errorf("ident block = %q", string(ident))
	}

	// First header block: status byte, active flag, conference number, blocks.
	hdr := messages[blockSize : 2*blockSize]
	if hdr[0] != ' ' {
		t.Errorf("msg0 status = %q, want ' ' (unread)", hdr[0])
	}
	if hdr[122] != 0xE1 {
		t.Errorf("msg0 active flag = %#x, want 0xE1", hdr[122])
	}
	if conf := getUint16LE(hdr, 123); conf != 1 {
		t.Errorf("msg0 conference = %d, want 1", conf)
	}
	if to := trimField(hdr[21:46]); to != "All" {
		t.Errorf("msg0 To = %q, want All", to)
	}
	if from := trimField(hdr[46:71]); from != "nut" {
		t.Errorf("msg0 From = %q, want nut", from)
	}
	if subj := trimField(hdr[71:96]); subj != "Welcome" {
		t.Errorf("msg0 Subject = %q, want Welcome", subj)
	}
	blocks := atoiField(hdr[116:122])
	if blocks < 2 {
		t.Errorf("msg0 block count = %d, want >= 2 (header + body)", blocks)
	}
	// The second message was marked read.
	bodyBlocks := blocks - 1
	secondHdrOff := 2*blockSize + bodyBlocks*blockSize
	hdr2 := messages[secondHdrOff : secondHdrOff+blockSize]
	if hdr2[0] != '-' {
		t.Errorf("msg1 status = %q, want '-' (read)", hdr2[0])
	}
}

// buildReplyZip hand-crafts a .REP packet using the same block writer Build
// uses, so ParseReply is exercised against real on-the-wire bytes.
func buildReplyZip(t *testing.T, msgs []Message) []byte {
	t.Helper()
	var data bytes.Buffer
	// Record 0: a BBSID ident block (readers put the BBSID here).
	data.Write(identBlock(BBSID))

	for i, m := range msgs {
		body := encodeBody(m.Body)
		bodyBlocks := (len(body) + blockSize - 1) / blockSize
		if bodyBlocks == 0 {
			bodyBlocks = 1
		}
		data.Write(messageHeader(m, i+1, bodyBlocks+1))
		padded := bodyBlocks * blockSize
		data.Write(body)
		data.Write(spaces(padded - len(body)))
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if err := writeZipFile(zw, BBSID+".MSG", data.Bytes()); err != nil {
		t.Fatalf("write msg: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func TestParseReplyRoundTrip(t *testing.T) {
	want := []Message{
		{Conference: 3, To: "nut", From: "phantom", Subject: "Re: Welcome",
			Body: "Thanks for the welcome.\nGlad to be here.\n\nLong-time caller."},
		{Conference: 7, To: "All", From: "phantom", Subject: "New topic",
			Body: "Single line reply."},
		{Conference: 1, To: "razor", From: "phantom", Subject: "Big body",
			Body: strings.Repeat("xyz line of text\n", 20)}, // spans many blocks
	}

	zipBytes := buildReplyZip(t, want)
	got, err := ParseReply(zipBytes)
	if err != nil {
		t.Fatalf("ParseReply: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d replies, want %d", len(got), len(want))
	}

	for i := range want {
		if got[i].Conference != want[i].Conference {
			t.Errorf("msg %d conference = %d, want %d", i, got[i].Conference, want[i].Conference)
		}
		if got[i].To != want[i].To {
			t.Errorf("msg %d To = %q, want %q", i, got[i].To, want[i].To)
		}
		if got[i].From != want[i].From {
			t.Errorf("msg %d From = %q, want %q", i, got[i].From, want[i].From)
		}
		if got[i].Subject != want[i].Subject {
			t.Errorf("msg %d Subject = %q, want %q", i, got[i].Subject, want[i].Subject)
		}
		// Body round-trips with newlines preserved (0xE3 decode). Trailing
		// block padding is trimmed, so compare against a right-trimmed want.
		wantBody := strings.TrimRight(want[i].Body, " \n")
		if got[i].Body != wantBody {
			t.Errorf("msg %d Body = %q, want %q", i, got[i].Body, wantBody)
		}
		// Multi-line bodies must keep their interior newlines.
		if strings.Contains(want[i].Body, "\n\n") && !strings.Contains(got[i].Body, "\n\n") {
			t.Errorf("msg %d lost blank-line separation", i)
		}
	}
}

// TestEncodeNewlineMarker verifies the 0xE3 convention is actually used on the
// wire (not a literal newline) inside body blocks.
func TestEncodeNewlineMarker(t *testing.T) {
	enc := encodeBody("a\nb")
	if bytes.IndexByte(enc, '\n') != -1 {
		t.Error("encoded body still contains a raw newline")
	}
	if bytes.IndexByte(enc, qwkNewline) == -1 {
		t.Errorf("encoded body missing 0x%X marker", qwkNewline)
	}
	if got := decodeBody(enc); got != "a\nb" {
		t.Errorf("decode round-trip = %q, want %q", got, "a\nb")
	}
}

// TestBuildThenParseConferences confirms Build's headers carry the conference
// numbers a reply parser would read back (offset 123 invariant).
func TestBuildThenParseConferences(t *testing.T) {
	p := Packet{
		Conferences: []Conference{{Number: 5, Name: "Five"}},
		Messages: []Message{
			{Conference: 5, To: "All", From: "nut", Subject: "s" + strconv.Itoa(5), Body: "body"},
		},
	}
	data, err := Build(p)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	files := unzip(t, data)
	msgs := files["MESSAGES.DAT"]
	hdr := msgs[blockSize : 2*blockSize]
	if conf := getUint16LE(hdr, 123); conf != 5 {
		t.Errorf("conference at offset 123 = %d, want 5", conf)
	}
}
