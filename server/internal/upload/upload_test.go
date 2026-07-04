package upload

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func zipWith(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func TestDescribeUsesFileIDDiz(t *testing.T) {
	z := zipWith(t, map[string]string{
		"README.TXT":  "ignore me",
		"file_id.diz": "  VENDETTA/X UTILS v1.2\r\n\r\n  the finest tools\r\n  on any board\r\n",
	})
	got := Describe(z, "typed description")
	want := "VENDETTA/X UTILS v1.2 the finest tools on any board"
	if got != want {
		t.Fatalf("Describe = %q, want %q", got, want)
	}
}

func TestDescribeNestedAndCaseInsensitive(t *testing.T) {
	z := zipWith(t, map[string]string{"REL/FILE_ID.DIZ": "nested diz"})
	if got := Describe(z, "x"); got != "nested diz" {
		t.Fatalf("nested = %q", got)
	}
}

func TestDescribeFallsBack(t *testing.T) {
	if got := Describe([]byte("just plain bytes"), "  typed  "); got != "typed" {
		t.Fatalf("non-zip fallback = %q", got)
	}
	z := zipWith(t, map[string]string{"a.txt": "no diz here"})
	if got := Describe(z, ""); got != "(no description)" {
		t.Fatalf("zip-without-diz empty fallback = %q", got)
	}
}

func TestDescribeClipsHostileDiz(t *testing.T) {
	long := strings.Repeat(strings.Repeat("A", 100)+"\n", 40) + "\x1b[31mred\n"
	z := zipWith(t, map[string]string{"FILE_ID.DIZ": long})
	got := Describe(z, "x")
	if strings.Contains(got, "\x1b") {
		t.Fatalf("escape byte survived: %q", got)
	}
	// 10 lines x 45 cols + 9 joiner spaces is the ceiling.
	if len(got) > 10*45+9 {
		t.Fatalf("diz not clipped: %d bytes", len(got))
	}
}
