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

// TestParseFullPacket runs the hub direction: Build a .QWK, then Parse it back
// and verify messages (with dates) and the conference list survive.
func TestParseFullPacket(t *testing.T) {
	when := time.Date(2026, 6, 30, 14, 45, 0, 0, time.Local)
	p := Packet{
		BoardName: "Hub of the North",
		Sysop:     "hubop",
		Caller:    "VENDX",
		Conferences: []Conference{
			{Number: 1, Name: "NET_GENERAL"},
			{Number: 4, Name: "NET_SYSOP"},
		},
		Messages: []Message{
			{Conference: 1, To: "All", From: "remote guy", Subject: "hi from afar",
				Body: "First networked post.\nSecond line.", Date: when},
			{Conference: 4, To: "All", From: "hubop", Subject: "netiquette",
				Body: "Be cool.", Date: when.Add(30 * time.Minute)},
		},
	}
	data, err := Build(p)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.BoardName != "Hub of the North" {
		t.Errorf("BoardName = %q", got.BoardName)
	}
	if len(got.Conferences) != 2 ||
		got.Conferences[0] != (Conference{1, "NET_GENERAL"}) ||
		got.Conferences[1] != (Conference{4, "NET_SYSOP"}) {
		t.Errorf("Conferences = %+v", got.Conferences)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(got.Messages))
	}
	m := got.Messages[0]
	if m.Conference != 1 || m.From != "remote guy" || m.Subject != "hi from afar" {
		t.Errorf("msg0 = %+v", m)
	}
	if m.Body != "First networked post.\nSecond line." {
		t.Errorf("msg0 body = %q", m.Body)
	}
	// The header only carries minute precision; both parse back exactly here.
	if !m.Date.Equal(when) {
		t.Errorf("msg0 date = %v, want %v", m.Date, when)
	}
	if !got.Messages[1].Date.Equal(when.Add(30 * time.Minute)) {
		t.Errorf("msg1 date = %v", got.Messages[1].Date)
	}
}

// TestParseWithoutControl: a packet missing CONTROL.DAT still yields messages.
func TestParseWithoutControl(t *testing.T) {
	var data bytes.Buffer
	data.Write(identBlock("SOMEHUB"))
	m := Message{Conference: 9, To: "All", From: "x", Subject: "s", Body: "b"}
	body := encodeBody(m.Body)
	data.Write(messageHeader(m, 1, 2))
	data.Write(body)
	data.Write(spaces(blockSize - len(body)))

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if err := writeZipFile(zw, "messages.dat", data.Bytes()); err != nil { // lower-case on purpose
		t.Fatalf("write: %v", err)
	}
	zw.Close()

	got, err := Parse(buf.Bytes())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(got.Messages) != 1 || got.Messages[0].Conference != 9 {
		t.Fatalf("messages = %+v", got.Messages)
	}
	if len(got.Conferences) != 0 || got.BoardName != "" {
		t.Errorf("expected empty control data, got %q %+v", got.BoardName, got.Conferences)
	}
}

// TestBuildReply checks the REP layout: <HUBID>.MSG member, hub id in record 0,
// conference in BOTH the ASCII message-number field and the binary field, and
// that our own ParseReply-style block walk recovers the messages.
func TestBuildReply(t *testing.T) {
	when := time.Date(2026, 7, 1, 9, 30, 0, 0, time.Local)
	msgs := []Message{
		{Conference: 2001, To: "All", From: "nut", Subject: "hello net",
			Body: "Greetings from Vendetta/X.\nCome visit.", Date: when},
		{Conference: 2002, To: "hubop", From: "razor", Subject: "question",
			Body: "How do I subscribe more echoes?", Date: when},
	}
	zipBytes, err := BuildReply("thehub", msgs)
	if err != nil {
		t.Fatalf("BuildReply: %v", err)
	}

	files := unzip(t, zipBytes)
	data, ok := files["THEHUB.MSG"]
	if !ok {
		t.Fatalf("missing THEHUB.MSG; members: %v", keys(files))
	}
	if len(data)%blockSize != 0 {
		t.Fatalf("length %d not block-aligned", len(data))
	}
	if id := strings.TrimSpace(string(data[:blockSize])); id != "THEHUB" {
		t.Errorf("record 0 ident = %q, want THEHUB", id)
	}

	hdr := data[blockSize : 2*blockSize]
	if n := atoiField(hdr[1:8]); n != 2001 {
		t.Errorf("ASCII message-number field = %d, want conference 2001", n)
	}
	if conf := getUint16LE(hdr, 123); conf != 2001 {
		t.Errorf("binary conference = %d, want 2001", conf)
	}

	parsed, err := parseMessages(data)
	if err != nil {
		t.Fatalf("parseMessages: %v", err)
	}
	if len(parsed) != 2 {
		t.Fatalf("parsed %d messages, want 2", len(parsed))
	}
	if parsed[0].From != "nut" || parsed[0].Body != "Greetings from Vendetta/X.\nCome visit." {
		t.Errorf("msg0 = %+v", parsed[0])
	}
	if parsed[1].Conference != 2002 || !parsed[1].Date.Equal(when) {
		t.Errorf("msg1 conference/date = %d %v", parsed[1].Conference, parsed[1].Date)
	}

	if _, err := BuildReply("  ", nil); err == nil {
		t.Error("BuildReply with blank hub id should error")
	}
}

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
