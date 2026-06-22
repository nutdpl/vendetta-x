package editor

import (
	"strings"
	"testing"
)

// event is a scripted keystroke.
type event struct {
	k Key
	r rune
}

// fakeConsole returns pre-loaded events from ReadKey (KeyEOF when exhausted)
// and records all Write output for assertions.
type fakeConsole struct {
	events []event
	idx    int
	out    strings.Builder
	flush  int
}

func (c *fakeConsole) Write(s string) { c.out.WriteString(s) }

func (c *fakeConsole) ReadKey() (Key, rune) {
	if c.idx >= len(c.events) {
		return KeyEOF, 0
	}
	ev := c.events[c.idx]
	c.idx++
	return ev.k, ev.r
}

func (c *fakeConsole) Flush() { c.flush++ }

// runes builds a slice of KeyRune events for the given string.
func runes(s string) []event {
	ev := make([]event, 0, len(s))
	for _, r := range s {
		ev = append(ev, event{KeyRune, r})
	}
	return ev
}

func script(parts ...[]event) []event {
	var out []event
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func TestTypeThenSave(t *testing.T) {
	c := &fakeConsole{events: script(runes("hello"), []event{{KeySave, 0}})}
	e := New(c, 1, 1, 20, 5, nil)
	lines, saved := e.Run()
	if !saved {
		t.Fatalf("expected saved=true")
	}
	if len(lines) != 1 || lines[0] != "hello" {
		t.Fatalf("got %#v, want [hello]", lines)
	}
}

func TestEnterSplitsLine(t *testing.T) {
	// type "abcd", move left twice, Enter -> "ab","cd"
	c := &fakeConsole{events: script(
		runes("abcd"),
		[]event{{KeyLeft, 0}, {KeyLeft, 0}, {KeyEnter, 0}, {KeySave, 0}},
	)}
	e := New(c, 1, 1, 20, 5, nil)
	lines, saved := e.Run()
	if !saved {
		t.Fatal("expected saved")
	}
	if len(lines) != 2 || lines[0] != "ab" || lines[1] != "cd" {
		t.Fatalf("got %#v, want [ab cd]", lines)
	}
}

func TestBackspaceWithinLine(t *testing.T) {
	c := &fakeConsole{events: script(runes("hello"), []event{{KeyBackspace, 0}, {KeySave, 0}})}
	e := New(c, 1, 1, 20, 5, nil)
	lines, _ := e.Run()
	if lines[0] != "hell" {
		t.Fatalf("got %q, want hell", lines[0])
	}
}

func TestBackspaceJoinsLines(t *testing.T) {
	// pre-loaded two lines; Home then Backspace at col 0 of line 2 joins.
	c := &fakeConsole{events: []event{{KeyDown, 0}, {KeyHome, 0}, {KeyBackspace, 0}, {KeySave, 0}}}
	e := New(c, 1, 1, 20, 5, []string{"foo", "bar"})
	lines, _ := e.Run()
	if len(lines) != 1 || lines[0] != "foobar" {
		t.Fatalf("got %#v, want [foobar]", lines)
	}
}

func TestBackspaceAtBufferStartNoop(t *testing.T) {
	c := &fakeConsole{events: []event{{KeyBackspace, 0}, {KeySave, 0}}}
	e := New(c, 1, 1, 20, 5, []string{"abc"})
	lines, _ := e.Run()
	if len(lines) != 1 || lines[0] != "abc" {
		t.Fatalf("got %#v, want [abc]", lines)
	}
}

func TestDeleteJoinsNextLine(t *testing.T) {
	// cursor at end of line 1 (KeyEnd), Delete joins line 2.
	c := &fakeConsole{events: []event{{KeyEnd, 0}, {KeyDelete, 0}, {KeySave, 0}}}
	e := New(c, 1, 1, 20, 5, []string{"foo", "bar"})
	lines, _ := e.Run()
	if len(lines) != 1 || lines[0] != "foobar" {
		t.Fatalf("got %#v, want [foobar]", lines)
	}
}

func TestArrowNavWithColumnClamping(t *testing.T) {
	// lines: "long line", "x". Put cursor at end of line0, Down should clamp
	// col to len("x")=1, then type "Y" -> "xY".
	c := &fakeConsole{events: script(
		[]event{{KeyEnd, 0}, {KeyDown, 0}},
		runes("Y"),
		[]event{{KeySave, 0}},
	)}
	e := New(c, 1, 1, 20, 5, []string{"long line", "x"})
	lines, _ := e.Run()
	if lines[1] != "xY" {
		t.Fatalf("got %q, want xY", lines[1])
	}
}

func TestLeftRightWrapAcrossLines(t *testing.T) {
	// At col0 of line1, Left wraps to end of line0; type "Z" appends to line0.
	c := &fakeConsole{events: script(
		[]event{{KeyDown, 0}, {KeyHome, 0}, {KeyLeft, 0}},
		runes("Z"),
		[]event{{KeySave, 0}},
	)}
	e := New(c, 1, 1, 20, 5, []string{"ab", "cd"})
	lines, _ := e.Run()
	if lines[0] != "abZ" {
		t.Fatalf("got %q, want abZ", lines[0])
	}
	// Right at end of line0 should wrap to start of line1.
	c2 := &fakeConsole{events: script(
		[]event{{KeyEnd, 0}, {KeyRight, 0}},
		runes("Q"),
		[]event{{KeySave, 0}},
	)}
	e2 := New(c2, 1, 1, 20, 5, []string{"ab", "cd"})
	lines2, _ := e2.Run()
	if lines2[1] != "Qcd" {
		t.Fatalf("got %q, want Qcd", lines2[1])
	}
}

func TestHomeEnd(t *testing.T) {
	// type "world", Home, type ">", End, type "<" -> ">world<"
	c := &fakeConsole{events: script(
		runes("world"),
		[]event{{KeyHome, 0}},
		runes(">"),
		[]event{{KeyEnd, 0}},
		runes("<"),
		[]event{{KeySave, 0}},
	)}
	e := New(c, 1, 1, 30, 5, nil)
	lines, _ := e.Run()
	if lines[0] != ">world<" {
		t.Fatalf("got %q, want >world<", lines[0])
	}
}

func TestAbortReturnsNilFalse(t *testing.T) {
	c := &fakeConsole{events: script(runes("junk"), []event{{KeyAbort, 0}})}
	e := New(c, 1, 1, 20, 5, nil)
	lines, saved := e.Run()
	if saved || lines != nil {
		t.Fatalf("got lines=%#v saved=%v, want nil,false", lines, saved)
	}
}

func TestEOFReturnsNilFalse(t *testing.T) {
	// No KeySave/KeyAbort; ReadKey eventually returns KeyEOF.
	c := &fakeConsole{events: runes("typed but never saved")}
	e := New(c, 1, 1, 20, 5, nil)
	lines, saved := e.Run()
	if saved || lines != nil {
		t.Fatalf("got lines=%#v saved=%v, want nil,false", lines, saved)
	}
}

func TestEditPreloadedMultiLine(t *testing.T) {
	// Buffer with interior blank line; move down to line2, append.
	c := &fakeConsole{events: script(
		[]event{{KeyDown, 0}, {KeyDown, 0}, {KeyEnd, 0}},
		runes("!"),
		[]event{{KeySave, 0}},
	)}
	e := New(c, 1, 1, 40, 10, []string{"first", "", "third"})
	lines, _ := e.Run()
	want := []string{"first", "", "third!"}
	if len(lines) != 3 {
		t.Fatalf("got %#v", lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("line %d: got %q want %q", i, lines[i], want[i])
		}
	}
}

func TestDrawsCursorPositioning(t *testing.T) {
	c := &fakeConsole{events: script(runes("hi"), []event{{KeySave, 0}})}
	e := New(c, 3, 5, 20, 4, nil)
	e.Run()
	out := c.out.String()
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected CSI escapes in output, got %q", out)
	}
	// origin-positioning sequence for the top-left window cell.
	if !strings.Contains(out, "\x1b[3;5H") {
		t.Fatalf("expected origin position escape \\x1b[3;5H in output")
	}
	if c.flush == 0 {
		t.Fatalf("expected Flush to be called during draws")
	}
}

func TestHorizontalScrollNoPanicLongLine(t *testing.T) {
	// Type more than width runes; must not panic and must preserve text.
	c := &fakeConsole{events: script(runes("abcdefghijklmnop"), []event{{KeySave, 0}})}
	e := New(c, 1, 1, 5, 3, nil) // width 5, line longer than width
	lines, _ := e.Run()
	if lines[0] != "abcdefghijklmnop" {
		t.Fatalf("got %q", lines[0])
	}
}
