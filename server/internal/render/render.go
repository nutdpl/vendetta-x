// Package render is the Go port of the board's pipe-code template renderer
// (see core/render.c). It turns a .pp/.ans template into an ANSI byte stream:
// |XX colour codes, |XY data tokens, |{r,c,key,label} lightbar markers, and
// cursor/screen codes. This file is the CONTRACT; the body implements the real
// pipe-code grammar, ported byte-for-byte from core/render.c.
package render

import (
	"fmt"
	"io"
	"os"
)

// Marker is one |{row,col,key,label} lightbar option found in the art.
type Marker struct {
	Row, Col int
	Key      byte
	Label    string
}

// Ctx carries the live data the renderer splices into the template.
type Ctx struct {
	// Tokens maps a two-letter code to its value, e.g. "BN" (board name),
	// "UH" (user handle), "VR" (version). Looked up for |XY codes.
	Tokens map[string]string
	// OnMarker, if non-nil, is called for every |{...} lightbar marker as the
	// art renders (so a menu engine can collect option positions).
	OnMarker func(Marker)
}

// DOS palette index -> ANSI SGR base code (RGB bit order differs from DOS).
var ansiFG = [8]int{30, 34, 32, 36, 31, 35, 33, 37}
var ansiBG = [8]int{40, 44, 42, 46, 41, 45, 43, 47}

// Default theme: slots 1..9 -> DOS color index.
var theme = [9]int{8, 7, 15, 14, 11, 9, 12, 1, 0}

// src is a one-byte-pushback byte source over an in-memory slice, mirroring
// render.c's pp_src (string variant; the Go API only renders from memory).
type src struct {
	b  []byte
	i  int
	pb int // pushed-back byte, or -1
}

const eof = -1

func (s *src) next() int {
	if s.pb != -1 {
		c := s.pb
		s.pb = -1
		return c
	}
	if s.i < len(s.b) {
		c := int(s.b[s.i])
		s.i++
		return c
	}
	return eof
}

func (s *src) back(c int) { s.pb = c }

// renderer holds output + the newline/attr state that render.c kept in globals
// and the pp_ctx.
type renderer struct {
	w     io.Writer
	ctx   *Ctx
	err   error
	last  int // newline state for LF->CRLF (render.c g_last)
	curFG int
	curBG int
}

func isDigit(c int) bool { return c >= '0' && c <= '9' }

// putc writes a single byte, recording the first write error.
func (r *renderer) putc(c byte) {
	if r.err != nil {
		return
	}
	_, err := r.w.Write([]byte{c})
	if err != nil {
		r.err = err
	}
}

// puts writes a string verbatim.
func (r *renderer) puts(s string) {
	if r.err != nil {
		return
	}
	_, err := io.WriteString(r.w, s)
	if err != nil {
		r.err = err
	}
}

// emitByte emits one art byte, turning a bare LF into CRLF but never doubling \r.
func (r *renderer) emitByte(c int) {
	if c == '\n' && r.last != '\r' {
		r.putc('\r')
	}
	r.putc(byte(c))
	r.last = c
}

func (r *renderer) emitStr(s string) {
	for i := 0; i < len(s); i++ {
		r.emitByte(int(s[i]))
	}
}

// emitAttr emits the current SGR attribute, matching render.c emit_attr.
func (r *renderer) emitAttr() {
	bold := 0
	if r.curFG >= 8 {
		bold = 1
	}
	r.puts(fmt.Sprintf("\x1b[%d;%d;%dm", bold, ansiFG[r.curFG&7], ansiBG[r.curBG&7]))
	r.last = 'm'
}

// emitCtrl emits a control/escape sequence verbatim (no LF->CRLF rewriting).
func (r *renderer) emitCtrl(seq string) {
	r.puts(seq)
	r.last = 'm'
}

// readNum reads a non-negative decimal from src; returns def if no digits.
func readNum(s *src, def int) int {
	d := s.next()
	if !isDigit(d) {
		s.back(d)
		return def
	}
	n := 0
	for isDigit(d) {
		n = n*10 + (d - '0')
		d = s.next()
	}
	s.back(d)
	return n
}

// emitValue emits a token value, applying an optional width modifier that
// follows it in the source: \[<>^]?NN. Ported from render.c emit_value.
func (r *renderer) emitValue(s *src, val string) {
	c := s.next()
	align := '<'
	width := -1

	if c == '\\' {
		d := s.next()
		if d == '<' || d == '>' || d == '^' {
			align = rune(d)
			d = s.next()
		}
		if isDigit(d) {
			width = 0
			for isDigit(d) {
				width = width*10 + (d - '0')
				d = s.next()
			}
			s.back(d)
		} else {
			s.back(d)
			r.emitByte('\\')
			if align != '<' {
				r.emitByte(int(align))
			}
		}
	} else {
		s.back(c)
	}

	if width < 0 {
		r.emitStr(val)
		return
	}

	n := len(val)
	if n >= width {
		for i := 0; i < width; i++ {
			r.emitByte(int(val[i])) // truncate
		}
		return
	}
	pad := width - n
	left, right := 0, 0
	switch align {
	case '>':
		left = pad
	case '^':
		left = pad / 2
		right = pad - left
	default:
		right = pad
	}
	for i := 0; i < left; i++ {
		r.emitByte(' ')
	}
	r.emitStr(val)
	for i := 0; i < right; i++ {
		r.emitByte(' ')
	}
}

// handleBar parses a lightbar marker body "R,C,K,Label" (braces stripped):
// draw Label at (R,C) and register it via OnMarker. Ported from handle_bar.
func (r *renderer) handleBar(spec string) {
	row, col, key := 0, 0, 0
	i := 0
	n := len(spec)

	for i < n && spec[i] == ' ' {
		i++
	}
	for i < n && isDigit(int(spec[i])) {
		if row < 1000 {
			row = row*10 + int(spec[i]-'0')
		}
		i++
	}
	if i < n && spec[i] == ',' {
		i++
	}
	for i < n && spec[i] == ' ' {
		i++
	}
	for i < n && isDigit(int(spec[i])) {
		if col < 1000 {
			col = col*10 + int(spec[i]-'0')
		}
		i++
	}
	if i < n && spec[i] == ',' {
		i++
	}
	for i < n && spec[i] == ' ' {
		i++
	}
	if i < n && spec[i] != 0 {
		key = int(spec[i])
		i++
	}
	if i < n && spec[i] == ',' {
		i++
	}
	for i < n && spec[i] == ' ' {
		i++
	}

	if row < 1 {
		row = 1
	} else if row > 999 {
		row = 999
	}
	if col < 1 {
		col = 1
	} else if col > 999 {
		col = 999
	}
	label := spec[i:]
	r.emitCtrl(fmt.Sprintf("\x1b[%d;%dH", row, col))
	r.emitStr(label)
	if r.ctx != nil && r.ctx.OnMarker != nil {
		r.ctx.OnMarker(Marker{Row: row, Col: col, Key: byte(key), Label: label})
	}
}

// tokenValue resolves a two-char data token to its value, or ("", false).
// All tokens (built-in and custom) resolve through ctx.Tokens here; render.c's
// built-ins are expected to be supplied by the caller via Tokens.
func (r *renderer) tokenValue(a, b int) (string, bool) {
	if r.ctx == nil || r.ctx.Tokens == nil {
		return "", false
	}
	v, ok := r.ctx.Tokens[string([]byte{byte(a), byte(b)})]
	return v, ok
}

// handleCode handles one '|' code; the leading '|' has already been consumed.
func (r *renderer) handleCode(s *src) {
	a := s.next()
	if a == eof {
		r.emitByte('|')
		return
	}
	b := s.next()
	if b == eof {
		r.emitByte('|')
		r.emitByte(a)
		return
	}

	// Cursor positioning: |[X## -> column (CHA), |[Y## -> row (VPA), 1-based.
	if a == '[' && (b == 'X' || b == 'x' || b == 'Y' || b == 'y') {
		n := readNum(s, 1)
		if n < 1 {
			n = 1
		}
		final := byte('d')
		if b == 'X' || b == 'x' {
			final = 'G'
		}
		r.emitCtrl(fmt.Sprintf("\x1b[%d%c", n, final))
		return
	}
	// Screen control: |CL clear+home, |CE clear to EOL. Intercepted before
	// token lookup so a data token can never shadow them.
	if a == 'C' && b == 'L' {
		r.emitCtrl("\x1b[2J\x1b[H")
		return
	}
	if a == 'C' && b == 'E' {
		r.emitCtrl("\x1b[K")
		return
	}

	// Lightbar option marker: |{R,C,K,Label} -- collect body up to '}'.
	if a == '{' {
		const maxBody = 95 // render.c m[96], one byte reserved for NUL
		body := make([]byte, 0, maxBody)
		d := b
		for d != eof && d != '}' && len(body) < maxBody {
			body = append(body, byte(d))
			d = s.next()
		}
		if d == '}' {
			r.handleBar(string(body))
			return
		}
		// malformed: emit literally
		r.emitByte('|')
		r.emitByte('{')
		for i := 0; i < len(body); i++ {
			r.emitByte(int(body[i]))
		}
		return
	}

	if isDigit(a) && isDigit(b) {
		n := (a-'0')*10 + (b - '0')
		if n <= 15 {
			r.curFG = n
			r.emitAttr()
			return
		}
		if n <= 23 {
			r.curBG = n - 16
			r.emitAttr()
			return
		}
	} else if a == 'T' && b >= '1' && b <= '9' {
		r.curFG = theme[b-'1']
		r.emitAttr()
		return
	} else {
		if val, ok := r.tokenValue(a, b); ok {
			r.emitValue(s, val)
			return
		}
	}

	// unrecognized: emit literally so a stray pipe never eats characters
	r.emitByte('|')
	r.emitByte(a)
	r.emitByte(b)
}

// Render parses template bytes src and writes the resulting ANSI to w.
func Render(w io.Writer, source []byte, ctx *Ctx) error {
	r := &renderer{
		w:     w,
		ctx:   ctx,
		last:  0,
		curFG: 7,
		curBG: 0,
	}
	s := &src{b: source, pb: -1}
	for {
		ch := s.next()
		if ch == eof {
			break
		}
		if ch == '|' {
			r.handleCode(s)
		} else {
			r.emitByte(ch)
		}
		if r.err != nil {
			return r.err
		}
	}
	return r.err
}

// RenderFile renders a .pp/.ans file by path.
func RenderFile(w io.Writer, path string, ctx *Ctx) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return Render(w, b, ctx)
}
