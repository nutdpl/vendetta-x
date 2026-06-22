// Package qwk implements the classic QWK offline-mail packet format (and its
// .REP reply-packet counterpart) using only the standard library. A .QWK packet
// is a ZIP archive a caller downloads to read a board's messages offline; a .REP
// packet is the ZIP they upload with their offline replies.
//
// The format is faithful to the de-facto QWK spec used by Qmail/QWKE-era
// readers: a text CONTROL.DAT describing the board and its conferences, plus a
// MESSAGES.DAT of fixed 128-byte records (a header block followed by N body
// blocks per message). This file is pure logic -- no I/O beyond the in-memory
// ZIP bytes -- so it is trivially testable.
package qwk

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// BBSID is the short board identifier embedded in the packet (also the .NDX /
// .MSG basename). We name the packet VENDX.QWK and expect VENDX.MSG in replies.
const BBSID = "VENDX"

// blockSize is the fixed QWK record length. MESSAGES.DAT is a sequence of these:
// record 0 is an ident block, then each message is one header block plus its
// body blocks.
const blockSize = 128

// qwkNewline is the byte QWK substitutes for a line break inside a message body
// (the classic 0xE3 / "π"); readers convert it back to a newline.
const qwkNewline = 0xE3

// Conference is one readable area in the packet, numbered for the reader.
type Conference struct {
	Number uint16
	Name   string
}

// Message is one post carried in (or recovered from) a packet.
type Message struct {
	Conference              uint16
	To, From, Subject, Body string
	Date                    time.Time
	Read                    bool
}

// Packet is everything needed to build a .QWK download for one caller.
type Packet struct {
	BoardName, Sysop, Caller string
	Conferences              []Conference
	Messages                 []Message
}

// Build renders the packet as the bytes of a .QWK ZIP archive (CONTROL.DAT +
// MESSAGES.DAT). The conferences should be the same set the messages reference.
func Build(p Packet) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	control := buildControl(p)
	if err := writeZipFile(zw, "CONTROL.DAT", control); err != nil {
		return nil, err
	}

	messages := buildMessages(p)
	if err := writeZipFile(zw, "MESSAGES.DAT", messages); err != nil {
		return nil, err
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("qwk: close zip: %w", err)
	}
	return buf.Bytes(), nil
}

// buildControl renders CONTROL.DAT: CRLF text describing the board, the caller,
// the total message count, the highest conference number, and each conference's
// number+name, then the customary blank door/news trailer lines.
func buildControl(p Packet) []byte {
	now := time.Now()
	maxConf := uint16(0)
	for _, c := range p.Conferences {
		if c.Number > maxConf {
			maxConf = c.Number
		}
	}

	var b strings.Builder
	wl := func(s string) { b.WriteString(s); b.WriteString("\r\n") }

	wl(emptyOr(p.BoardName, "Vendetta/X"))           // board name
	wl("")                                           // location (blank ok)
	wl("")                                           // phone (blank ok)
	wl(emptyOr(p.Sysop, "Sysop"))                    // sysop
	wl("00000000," + BBSID)                          // <serial>,<BBSID>
	wl(now.Format("01-02-2006,15:04:05"))            // date,time
	wl(strings.ToUpper(emptyOr(p.Caller, "CALLER"))) // caller name
	wl("")                                           // blank
	wl("")                                           // blank
	wl("0")                                          // total-messages-this-call marker
	wl(strconv.Itoa(len(p.Messages)))                // total messages in packet
	wl(strconv.Itoa(int(maxConf)))                   // highest conference number

	for _, c := range p.Conferences {
		wl(strconv.Itoa(int(c.Number)))
		wl(c.Name)
	}

	// Door / news / goodbye filenames (blank trailers are accepted).
	wl("VENDX.NWS")
	wl("HELLO")
	wl("BBSNEWS")
	wl("GOODBYE")

	return []byte(b.String())
}

// buildMessages renders MESSAGES.DAT: an ident block (record 0) followed by, for
// each message, a header block and its body blocks.
func buildMessages(p Packet) []byte {
	var out bytes.Buffer
	out.Write(identBlock("Vendetta/X QWK packet  -  Produced by Vendetta/X"))

	for i, m := range p.Messages {
		body := encodeBody(m.Body)
		bodyBlocks := (len(body) + blockSize - 1) / blockSize
		if bodyBlocks == 0 {
			bodyBlocks = 1 // a message always carries at least one body block
		}
		// Block count INCLUDING the header block.
		totalBlocks := bodyBlocks + 1

		out.Write(messageHeader(m, i+1, totalBlocks))

		// Body, padded out to a whole number of 128-byte blocks with spaces.
		padded := bodyBlocks * blockSize
		out.Write(body)
		out.Write(spaces(padded - len(body)))
	}
	return out.Bytes()
}

// identBlock returns a 128-byte ident record (record 0). Any 128-byte ident is
// accepted by readers; we use a clear Vendetta/X string, space-padded.
func identBlock(ident string) []byte {
	return fieldBytes(ident, blockSize)
}

// messageHeader returns the 128-byte header block for one message. seq is the
// 1-based logical message number within the packet; totalBlocks is the block
// count INCLUDING this header.
func messageHeader(m Message, seq, totalBlocks int) []byte {
	h := spaces(blockSize)

	status := byte(' ') // public, unread
	if m.Read {
		status = '-' // public, read
	}
	h[0] = status

	date := m.Date
	if date.IsZero() {
		date = time.Now()
	}

	putField(h, 1, 7, strconv.Itoa(seq))           // message number
	putField(h, 8, 8, date.Format("01-02-06"))     // date MM-DD-YY
	putField(h, 16, 5, date.Format("15:04"))       // time HH:MM
	putField(h, 21, 25, m.To)                      // To
	putField(h, 46, 25, m.From)                    // From
	putField(h, 71, 25, m.Subject)                 // Subject
	putField(h, 96, 12, "")                        // password (blank)
	putField(h, 108, 8, "")                        // reference msg number (blank)
	putField(h, 116, 6, strconv.Itoa(totalBlocks)) // block count incl. header

	h[122] = 0xE1 // active flag

	putUint16LE(h, 123, m.Conference) // conference number
	putUint16LE(h, 125, uint16(seq))  // logical message number in packet
	h[127] = ' '                      // net tag

	return h
}

// encodeBody converts a message body to QWK on-the-wire form: newlines become
// the 0xE3 marker so the body is a single run of bytes within its blocks.
func encodeBody(body string) []byte {
	// Normalize CRLF/CR to LF first, then map each LF to the QWK marker.
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	b := []byte(body)
	for i := range b {
		if b[i] == '\n' {
			b[i] = qwkNewline
		}
	}
	return b
}

// decodeBody reverses encodeBody: 0xE3 markers become newlines and trailing
// block-padding spaces are trimmed.
func decodeBody(raw []byte) string {
	b := make([]byte, len(raw))
	copy(b, raw)
	for i := range b {
		if b[i] == qwkNewline {
			b[i] = '\n'
		}
	}
	// Block bodies are space-padded to a 128-byte boundary; trim that padding
	// (and any trailing newline introduced by it) without eating interior text.
	return strings.TrimRight(string(b), " \x00\n")
}

// ParseReply parses a .REP reply ZIP (containing VENDX.MSG in MESSAGES.DAT
// layout) and returns the caller's reply messages: conference number plus
// To/From/Subject/Body (with 0xE3 decoded back to newlines).
// maxReplyBytes caps the decompressed size of a reply packet's message member,
// guarding against zip bombs (the upload itself is also size-capped upstream).
const maxReplyBytes = 32 << 20 // 32 MiB

func ParseReply(zipBytes []byte) ([]Message, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, fmt.Errorf("qwk: open reply zip: %w", err)
	}

	var data []byte
	wantUpper := strings.ToUpper(BBSID + ".MSG")
	for _, f := range zr.File {
		if strings.EqualFold(f.Name, BBSID+".MSG") ||
			strings.ToUpper(baseName(f.Name)) == wantUpper {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("qwk: open %s: %w", f.Name, err)
			}
			// Bound the decompressed read so a zip bomb (a tiny member that
			// inflates to gigabytes) can't exhaust memory.
			data, err = io.ReadAll(io.LimitReader(rc, maxReplyBytes+1))
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("qwk: read %s: %w", f.Name, err)
			}
			if len(data) > maxReplyBytes {
				return nil, errors.New("qwk: reply packet member too large")
			}
			break
		}
	}
	if data == nil {
		return nil, errors.New("qwk: reply packet has no " + BBSID + ".MSG")
	}
	return parseMessages(data)
}

// parseMessages walks a MESSAGES.DAT byte stream: record 0 is the BBSID ident
// block, then each message is a header block followed by (blockCount-1) body
// blocks. Malformed/short records terminate parsing gracefully.
func parseMessages(data []byte) ([]Message, error) {
	if len(data) < blockSize {
		return nil, errors.New("qwk: message data too short")
	}
	// Skip record 0 (the ident block).
	pos := blockSize
	var out []Message

	for pos+blockSize <= len(data) {
		header := data[pos : pos+blockSize]
		pos += blockSize

		blocks := atoiField(header[116:122])
		if blocks < 1 {
			// Not a sane header; stop rather than misread the rest.
			break
		}
		bodyBlocks := blocks - 1

		bodyEnd := pos + bodyBlocks*blockSize
		if bodyEnd > len(data) {
			bodyEnd = len(data) // tolerate a truncated final block
		}
		bodyRaw := data[pos:bodyEnd]
		pos = bodyEnd

		out = append(out, Message{
			Conference: getUint16LE(header, 123),
			To:         trimField(header[21:46]),
			From:       trimField(header[46:71]),
			Subject:    trimField(header[71:96]),
			Body:       decodeBody(bodyRaw),
		})
	}
	return out, nil
}

// ---- low-level field helpers ----

// writeZipFile adds one stored file to the archive.
func writeZipFile(zw *zip.Writer, name string, data []byte) error {
	fw, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("qwk: create %s: %w", name, err)
	}
	if _, err := fw.Write(data); err != nil {
		return fmt.Errorf("qwk: write %s: %w", name, err)
	}
	return nil
}

// spaces returns an n-byte slice filled with ASCII spaces.
func spaces(n int) []byte {
	if n < 0 {
		n = 0
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return b
}

// fieldBytes returns s as exactly n bytes: truncated if too long, space-padded
// on the right if too short.
func fieldBytes(s string, n int) []byte {
	b := spaces(n)
	src := []byte(s)
	if len(src) > n {
		src = src[:n]
	}
	copy(b, src)
	return b
}

// putField writes s into dst[off:off+width], space-padded / truncated to width.
func putField(dst []byte, off, width int, s string) {
	copy(dst[off:off+width], fieldBytes(s, width))
}

// putUint16LE writes v little-endian at dst[off:off+2].
func putUint16LE(dst []byte, off int, v uint16) {
	dst[off] = byte(v)
	dst[off+1] = byte(v >> 8)
}

// getUint16LE reads a little-endian uint16 at src[off:off+2].
func getUint16LE(src []byte, off int) uint16 {
	if off+2 > len(src) {
		return 0
	}
	return uint16(src[off]) | uint16(src[off+1])<<8
}

// trimField returns a header text field with trailing/leading spaces and NULs
// removed.
func trimField(b []byte) string {
	return strings.Trim(string(b), " \x00")
}

// atoiField parses a space-padded ASCII integer field (block counts etc.).
func atoiField(b []byte) int {
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0
	}
	return n
}

// emptyOr returns s, or alt when s is blank.
func emptyOr(s, alt string) string {
	if strings.TrimSpace(s) == "" {
		return alt
	}
	return s
}

// baseName returns the final path element of a (possibly slashed) zip entry name.
func baseName(name string) string {
	if i := strings.LastIndexAny(name, "/\\"); i >= 0 {
		return name[i+1:]
	}
	return name
}
