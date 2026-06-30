package render

import (
	"bytes"
	"testing"
)

// r renders a template string with a fresh ctx whose Tokens mirror the C test's
// default pp_ctx (handle = "nut", version = "0.1", board = "Vendetta/X").
func r(t *testing.T, tpl string) string {
	t.Helper()
	var buf bytes.Buffer
	ctx := &Ctx{Tokens: map[string]string{
		"UH": "nut",
		"BN": "Vendetta/X",
		"VR": "0.1",
	}}
	if err := Render(&buf, []byte(tpl), ctx); err != nil {
		t.Fatalf("Render(%q) error: %v", tpl, err)
	}
	return buf.String()
}

func eq(t *testing.T, name, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q want %q", name, got, want)
	}
}

func TestPassthroughCRLF(t *testing.T) {
	eq(t, "passthrough + LF->CRLF", r(t, "hi\n"), "hi\r\n")
}

func TestColor(t *testing.T) {
	// |15 -> bold white on black
	eq(t, "color |15", r(t, "|15"), "\x1b[1;37;40m")
	// |07 -> non-bold white on black
	eq(t, "color |07", r(t, "|07"), "\x1b[0;37;40m")
	// |16 -> background blue (index 0 after -16 ... 16-16=0 -> bg 40); use 17 (blue)
	eq(t, "bg |17", r(t, "|17"), "\x1b[0;37;44m")
}

func TestToken(t *testing.T) {
	eq(t, "token |UH", r(t, "|UH"), "nut")
}

func TestTokenWidth(t *testing.T) {
	// right-justify to 5
	eq(t, "token |UH\\>5", r(t, "|UH\\>5"), "  nut")
	// left-justify (default) to 5
	eq(t, "token |UH\\5", r(t, "|UH\\5"), "nut  ")
	// center to 5
	eq(t, "token |UH\\^5", r(t, "|UH\\^5"), " nut ")
	// truncate to 2
	eq(t, "token |UH\\2", r(t, "|UH\\2"), "nu")
}

func TestCursorCol(t *testing.T) {
	eq(t, "|[X10 -> ESC[10G", r(t, "|[X10"), "\x1b[10G")
}

func TestCursorRow(t *testing.T) {
	eq(t, "|[Y5 -> ESC[5d", r(t, "|[Y5"), "\x1b[5d")
}

func TestCursorDefault(t *testing.T) {
	eq(t, "|[X -> ESC[1G default", r(t, "|[X"), "\x1b[1G")
}

func TestClearScreen(t *testing.T) {
	eq(t, "|CL -> ESC[2J ESC[H", r(t, "|CL"), "\x1b[2J\x1b[H")
}

func TestClearEOL(t *testing.T) {
	eq(t, "|CE -> ESC[K", r(t, "|CE"), "\x1b[K")
}

func TestCtrlThenLF(t *testing.T) {
	eq(t, "|CE then LF -> CRLF", r(t, "|CE\n"), "\x1b[K\r\n")
}

func TestUnknownLiteral(t *testing.T) {
	eq(t, "unknown |ZZ literal", r(t, "|ZZ"), "|ZZ")
}

func TestLiteralPipe(t *testing.T) {
	// render.c has no special || escape: "||" => a='|', b=EOF -> emit '|','|'.
	eq(t, "double pipe at EOF", r(t, "||"), "||")
	// A lone trailing '|' (a=EOF) emits a single '|'.
	eq(t, "trailing pipe", r(t, "x|"), "x|")
}

func TestCompose(t *testing.T) {
	eq(t, "compose |[Y2|[X5 text", r(t, "|[Y2|[X5Name:"), "\x1b[2d\x1b[5GName:")
}

func TestMalformedMarker(t *testing.T) {
	eq(t, "malformed |{ literal", r(t, "|{2,5,F,Files"), "|{2,5,F,Files")
}

func TestMarkersDrawAndRegister(t *testing.T) {
	var buf bytes.Buffer
	var got []Marker
	ctx := &Ctx{OnMarker: func(m Marker) { got = append(got, m) }}
	if err := Render(&buf, []byte("hdr|{2,5,F,Files}mid|{3,5,M,Mail}"), ctx); err != nil {
		t.Fatal(err)
	}
	eq(t, "markers draw labels at positions", buf.String(),
		"hdr\x1b[2;5HFilesmid\x1b[3;5HMail")
	if len(got) != 2 {
		t.Fatalf("registered %d options, want 2", len(got))
	}
	if got[0] != (Marker{Row: 2, Col: 5, Key: 'F', Label: "Files"}) {
		t.Errorf("option 1 = %+v want {2 5 'F' Files}", got[0])
	}
	if got[1] != (Marker{Row: 3, Col: 5, Key: 'M', Label: "Mail"}) {
		t.Errorf("option 2 = %+v want {3 5 'M' Mail}", got[1])
	}
}

func TestMarkerLabelSpacesCommas(t *testing.T) {
	var buf bytes.Buffer
	var got []Marker
	ctx := &Ctx{OnMarker: func(m Marker) { got = append(got, m) }}
	if err := Render(&buf, []byte("|{10,20,G,Goodbye, caller}"), ctx); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Label != "Goodbye, caller" {
		t.Fatalf("label keeps spaces/commas: got %+v", got)
	}
}

func TestMarkerClamp(t *testing.T) {
	var buf bytes.Buffer
	var got []Marker
	ctx := &Ctx{OnMarker: func(m Marker) { got = append(got, m) }}
	if err := Render(&buf, []byte("|{99999,0,K,X}"), ctx); err != nil {
		t.Fatal(err)
	}
	// row huge -> clamped to 999; col 0 -> clamped to 1
	eq(t, "clamp huge row / zero col", buf.String(), "\x1b[999;1HX")
	if len(got) != 1 || got[0].Row != 999 || got[0].Col != 1 {
		t.Fatalf("clamp marker: got %+v", got)
	}
}

func TestTheme(t *testing.T) {
	// |T3 -> theme[2] = 15 -> bold white on black
	eq(t, "|T3 theme", r(t, "|T3"), "\x1b[1;37;40m")
	// |T1 -> theme[0] = 8 -> bold black (gray)
	eq(t, "|T1 theme", r(t, "|T1"), "\x1b[1;30;40m")
}

func TestRenderFileWelcome(t *testing.T) {
	var buf bytes.Buffer
	ctx := &Ctx{Tokens: map[string]string{
		"UH": "nut",
		"BN": "Vendetta/X",
		"VR": "0.1",
		"UL": "Earth",
		"UC": "42",
	}}
	// art/welcome.pp exercises the same UH/BN/VR/UL/UC tokens sysinfo.pp used
	// to (before it was dropped as dead art -- nothing ever rendered it; the
	// real sysop/system-info screens are hand-printed Go, not a .pp file).
	if err := RenderFile(&buf, "../../../art/welcome.pp", ctx); err != nil {
		t.Fatalf("RenderFile error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"nut", "0.1", "Earth", "42"} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Errorf("welcome output missing %q", want)
		}
	}
	// LF should have been normalized to CRLF.
	if bytes.Contains([]byte(out), []byte("\n")) && !bytes.Contains([]byte(out), []byte("\r\n")) {
		t.Errorf("welcome output not CRLF-normalized")
	}
}
