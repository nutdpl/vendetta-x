package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestArtNeverPaintsColumn80 audits every art file the board can serve for
// lines that reach 80 visible columns. A line that paints column 80 AND ends
// in a newline double-spaces on every ANSI.SYS-family renderer (TheDraw, DOS
// ANSI.SYS, SyncTERM's ANSI-BBS/cterm): those wrap the cursor the moment
// column 80 is written, so the newline that follows opens a blank row under
// the line -- stretching wordmarks to double height and shearing everything
// below. Modern terminals defer the wrap, which is how this shipped broken
// once already: every PNG preview and vt-emulator check looked fine while
// SyncTERM (and TheDraw, viewing the same bytes) showed the art shredded.
// The art pipeline caps emission at 79 columns (legacy/tools/dither.py
// MAX_EMIT_COLS); this test keeps regenerated or hand-edited art honest.
func TestArtNeverPaintsColumn80(t *testing.T) {
	escapes := regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
	markers := regexp.MustCompile(`\|\{[^}]*\}`) // |{r,c,key,label}: positions absolutely
	cursor := regexp.MustCompile(`\|\[[XY][0-9]+`)
	pipes := regexp.MustCompile(`\|..`) // |XX color/token codes

	entries, err := os.ReadDir("../art")
	if err != nil {
		t.Fatalf("read art dir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".pp") && !strings.HasSuffix(name, ".ans") {
			continue
		}
		data, err := os.ReadFile(filepath.Join("../art", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for i, line := range bytes.Split(data, []byte("\n")) {
			line = bytes.TrimSuffix(line, []byte("\r"))
			line = escapes.ReplaceAll(line, nil)
			line = markers.ReplaceAll(line, nil)
			line = cursor.ReplaceAll(line, nil)
			line = pipes.ReplaceAll(line, nil)
			if len(line) >= 80 {
				t.Errorf("%s:%d paints %d columns; art must stop at 79 "+
					"(column 80 + newline double-spaces on ANSI.SYS/cterm renderers)",
					name, i+1, len(line))
			}
		}
	}
}
