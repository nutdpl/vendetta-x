// Package editor is the board's full-screen ANSI message editor: the thing
// callers actually remember about a BBS. It draws a multi-line editing window,
// moves a cursor, inserts and deletes text, soft-wraps at the window width, and
// returns the finished lines when the user saves (or nothing if they abort).
//
// It talks to the terminal through the Console interface so it has no telnet
// dependency and can be driven by a scripted Console in tests.
//
// This file is the CONTRACT. New + Run stub out until implemented.
//
// Overflow policy: this implementation uses HORIZONTAL SCROLLING within a line
// (not soft-wrap). A buffer line may grow arbitrarily long; only the slice that
// fits the window width starting at a horizontal offset (colOff) is drawn. The
// horizontal offset follows the cursor so the cursor is always visible.
package editor

import "strings"

// Key is a decoded keystroke handed to the editor by the Console.
type Key int

const (
	KeyRune      Key = iota // a printable rune (r is valid)
	KeyEnter                // newline / split line
	KeyBackspace            // delete left
	KeyDelete               // delete under cursor
	KeyLeft
	KeyRight
	KeyUp
	KeyDown
	KeyHome
	KeyEnd
	KeySave  // finish and keep the text (e.g. Ctrl-Z)
	KeyAbort // discard (e.g. Esc twice / Ctrl-C)
	KeyEOF   // carrier lost
)

// Console is the terminal the editor draws on and reads keys from. Write
// receives raw bytes (ANSI escapes + text); coordinates are 1-based and the
// editor positions the cursor with standard CSI sequences relative to the
// origin it was told to draw at.
type Console interface {
	Write(s string)       // emit raw ANSI/text
	ReadKey() (Key, rune) // next keystroke; rune meaningful only for KeyRune
	Flush()               // flush buffered output to the wire
}

// Editor is a bounded multi-line text editor.
type Editor struct {
	con    Console
	lines  [][]rune // text buffer, one []rune per line; always >= 1 line
	row    int      // cursor line index into lines
	col    int      // cursor rune index into lines[row]
	rowOff int      // index of the buffer line drawn at the top of the window
	colOff int      // horizontal scroll offset within the current line

	originRow int // 1-based screen row of the window's top-left cell
	originCol int // 1-based screen col of the window's top-left cell
	width     int // window width in columns
	height    int // window height in rows
}

// New creates an editor whose editing window is width columns by height rows,
// drawn starting at screen cell (originRow, originCol) (1-based), pre-loaded
// with lines (may be nil for an empty buffer).
func New(c Console, originRow, originCol, width, height int, lines []string) *Editor {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	buf := make([][]rune, 0, len(lines))
	for _, l := range lines {
		buf = append(buf, []rune(l))
	}
	if len(buf) == 0 {
		buf = append(buf, []rune{})
	}
	return &Editor{
		con:       c,
		lines:     buf,
		originRow: originRow,
		originCol: originCol,
		width:     width,
		height:    height,
	}
}

// CursorEnd moves the cursor to the end of the buffer -- where a reply
// composer wants to open when the buffer is prefilled with quoted text.
// draw()'s scroll() brings the window along on the first paint.
func (e *Editor) CursorEnd() {
	e.row = len(e.lines) - 1
	e.col = len(e.lines[e.row])
}

// Run draws the editor and drives the edit loop until the user saves or aborts.
// On save it returns the buffer's lines and saved=true; on abort or carrier
// loss it returns nil, false.
func (e *Editor) Run() (lines []string, saved bool) {
	for {
		e.draw()
		k, r := e.con.ReadKey()
		switch k {
		case KeyRune:
			e.insertRune(r)
		case KeyEnter:
			e.splitLine()
		case KeyBackspace:
			e.backspace()
		case KeyDelete:
			e.deleteUnder()
		case KeyLeft:
			e.moveLeft()
		case KeyRight:
			e.moveRight()
		case KeyUp:
			e.moveUp()
		case KeyDown:
			e.moveDown()
		case KeyHome:
			e.col = 0
		case KeyEnd:
			e.col = len(e.lines[e.row])
		case KeySave:
			return e.snapshot(), true
		case KeyAbort, KeyEOF:
			return nil, false
		default:
			// ignore unknown keys
		}
	}
}

// snapshot converts the buffer to []string, trimming trailing spaces per line
// but preserving interior blank lines and line count.
func (e *Editor) snapshot() []string {
	out := make([]string, len(e.lines))
	for i, l := range e.lines {
		out[i] = strings.TrimRight(string(l), " ")
	}
	return out
}

// Buffer bounds. O(1) checks on each keystroke keep one session's editor from
// being pumped full of megabytes by a fast scripted client; comfortably above
// any real message.
const (
	maxEditorLines   = 500
	maxEditorLineLen = 1000
)

func (e *Editor) insertRune(r rune) {
	line := e.lines[e.row]
	if len(line) >= maxEditorLineLen {
		return // line is full; drop the keystroke
	}
	if e.col > len(line) {
		e.col = len(line)
	}
	nl := make([]rune, 0, len(line)+1)
	nl = append(nl, line[:e.col]...)
	nl = append(nl, r)
	nl = append(nl, line[e.col:]...)
	e.lines[e.row] = nl
	e.col++
}

func (e *Editor) splitLine() {
	if len(e.lines) >= maxEditorLines {
		return // buffer is at its line cap; Enter is a no-op
	}
	line := e.lines[e.row]
	if e.col > len(line) {
		e.col = len(line)
	}
	left := make([]rune, e.col)
	copy(left, line[:e.col])
	right := make([]rune, len(line)-e.col)
	copy(right, line[e.col:])

	e.lines[e.row] = left
	// insert right as a new line after row
	e.lines = append(e.lines, nil)
	copy(e.lines[e.row+2:], e.lines[e.row+1:])
	e.lines[e.row+1] = right

	e.row++
	e.col = 0
}

func (e *Editor) backspace() {
	if e.col > 0 {
		line := e.lines[e.row]
		nl := make([]rune, 0, len(line)-1)
		nl = append(nl, line[:e.col-1]...)
		nl = append(nl, line[e.col:]...)
		e.lines[e.row] = nl
		e.col--
		return
	}
	if e.row == 0 {
		return // no-op at buffer start
	}
	// join with previous line
	prev := e.lines[e.row-1]
	join := len(prev)
	merged := make([]rune, 0, len(prev)+len(e.lines[e.row]))
	merged = append(merged, prev...)
	merged = append(merged, e.lines[e.row]...)
	e.lines[e.row-1] = merged
	e.lines = append(e.lines[:e.row], e.lines[e.row+1:]...)
	e.row--
	e.col = join
}

func (e *Editor) deleteUnder() {
	line := e.lines[e.row]
	if e.col < len(line) {
		nl := make([]rune, 0, len(line)-1)
		nl = append(nl, line[:e.col]...)
		nl = append(nl, line[e.col+1:]...)
		e.lines[e.row] = nl
		return
	}
	// at line end: join next line up
	if e.row == len(e.lines)-1 {
		return // no-op at buffer end
	}
	merged := make([]rune, 0, len(line)+len(e.lines[e.row+1]))
	merged = append(merged, line...)
	merged = append(merged, e.lines[e.row+1]...)
	e.lines[e.row] = merged
	e.lines = append(e.lines[:e.row+1], e.lines[e.row+2:]...)
}

func (e *Editor) moveLeft() {
	if e.col > 0 {
		e.col--
		return
	}
	if e.row > 0 {
		e.row--
		e.col = len(e.lines[e.row])
	}
}

func (e *Editor) moveRight() {
	if e.col < len(e.lines[e.row]) {
		e.col++
		return
	}
	if e.row < len(e.lines)-1 {
		e.row++
		e.col = 0
	}
}

func (e *Editor) moveUp() {
	if e.row > 0 {
		e.row--
		if e.col > len(e.lines[e.row]) {
			e.col = len(e.lines[e.row])
		}
	}
}

func (e *Editor) moveDown() {
	if e.row < len(e.lines)-1 {
		e.row++
		if e.col > len(e.lines[e.row]) {
			e.col = len(e.lines[e.row])
		}
	}
}

// scroll adjusts rowOff and colOff so the cursor is inside the window.
func (e *Editor) scroll() {
	if e.row < e.rowOff {
		e.rowOff = e.row
	}
	if e.row >= e.rowOff+e.height {
		e.rowOff = e.row - e.height + 1
	}
	if e.rowOff < 0 {
		e.rowOff = 0
	}
	if e.col < e.colOff {
		e.colOff = e.col
	}
	if e.col >= e.colOff+e.width {
		e.colOff = e.col - e.width + 1
	}
	if e.colOff < 0 {
		e.colOff = 0
	}
}

// draw repaints the whole window and places the hardware cursor.
func (e *Editor) draw() {
	e.scroll()
	for vr := 0; vr < e.height; vr++ {
		screenRow := e.originRow + vr
		// position to the start of this window row
		e.con.Write(csiPos(screenRow, e.originCol))
		// clear the window-width region by writing spaces
		bufIdx := e.rowOff + vr
		var visible string
		if bufIdx < len(e.lines) {
			line := e.lines[bufIdx]
			if e.colOff < len(line) {
				end := e.colOff + e.width
				if end > len(line) {
					end = len(line)
				}
				visible = string(line[e.colOff:end])
			}
		}
		// pad to full width so stale content is overwritten
		if len(visible) < e.width {
			visible += strings.Repeat(" ", e.width-len([]rune(visible)))
		}
		e.con.Write(visible)
	}
	// place hardware cursor at the buffer cursor's screen position
	curScreenRow := e.originRow + (e.row - e.rowOff)
	curScreenCol := e.originCol + (e.col - e.colOff)
	e.con.Write(csiPos(curScreenRow, curScreenCol))
	e.con.Flush()
}

// csiPos returns the CSI cursor-position sequence for a 1-based row/col.
func csiPos(row, col int) string {
	return "\x1b[" + itoa(row) + ";" + itoa(col) + "H"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
