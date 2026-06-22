package door

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	s, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestStoreRoundTrip(t *testing.T) {
	s := newTestStore(t)

	// Empty to start (no seeding).
	if ds, err := s.List(); err != nil || len(ds) != 0 {
		t.Fatalf("List empty: %v len=%d", err, len(ds))
	}

	id, err := s.Add(&Door{
		Name:        "Legend of the Red Dragon",
		Description: "Classic LORD",
		Command:     "dosbox -conf /opt/lord/dosbox.conf -exit",
		WorkDir:     "/opt/lord",
		DropType:    "DOOR.SYS",
		Enabled:     true,
	})
	if err != nil || id == 0 {
		t.Fatalf("Add: %v id=%d", err, id)
	}

	// A second, disabled door (sorts before the first by name).
	id2, err := s.Add(&Door{Name: "Barren Realms", Command: "brez", Enabled: false})
	if err != nil {
		t.Fatalf("Add 2: %v", err)
	}

	all, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List len = %d, want 2", len(all))
	}
	// Ordered by name COLLATE NOCASE: "Barren Realms" before "Legend...".
	if all[0].Name != "Barren Realms" || all[1].Name != "Legend of the Red Dragon" {
		t.Fatalf("List order wrong: %q, %q", all[0].Name, all[1].Name)
	}

	en, err := s.Enabled()
	if err != nil {
		t.Fatalf("Enabled: %v", err)
	}
	if len(en) != 1 || en[0].ID != id {
		t.Fatalf("Enabled = %+v, want only id %d", en, id)
	}

	got, err := s.Get(id)
	if err != nil || got == nil {
		t.Fatalf("Get: %v %v", err, got)
	}
	if got.Name != "Legend of the Red Dragon" || got.WorkDir != "/opt/lord" ||
		got.DropType != "DOOR.SYS" || !got.Enabled {
		t.Fatalf("Get mismatch: %+v", got)
	}

	// Missing id -> nil,nil.
	if g, err := s.Get(99999); err != nil || g != nil {
		t.Fatalf("Get missing = %v,%v want nil,nil", g, err)
	}

	got.Name = "LORD"
	got.Enabled = false
	if err := s.Update(got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	re, _ := s.Get(id)
	if re.Name != "LORD" || re.Enabled {
		t.Fatalf("Update not applied: %+v", re)
	}
	if en, _ := s.Enabled(); len(en) != 0 {
		t.Fatalf("Enabled after disable = %d, want 0", len(en))
	}

	if err := s.Delete(id2); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if g, _ := s.Get(id2); g != nil {
		t.Fatalf("door %d not deleted", id2)
	}
	if all, _ := s.List(); len(all) != 1 {
		t.Fatalf("List after delete = %d, want 1", len(all))
	}
}

func TestWriteDropFileDoorSys(t *testing.T) {
	dir := t.TempDir()
	d := Door{Name: "LORD", WorkDir: dir, DropType: "DOOR.SYS", DOSPath: "C:\\LORD"}
	c := Caller{Node: 1, Handle: "Acidburn", RealName: "Kate Libby", SL: 100, MinutesLeft: 45, Emulation: 1, Baud: 38400}

	path, err := d.WriteDropFile(c, System{Name: "Vendetta/X", Sysop: "nut"})
	if err != nil {
		t.Fatalf("WriteDropFile: %v", err)
	}
	if filepath.Base(path) != "DOOR.SYS" {
		t.Fatalf("path base = %q, want DOOR.SYS", filepath.Base(path))
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read drop: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "Acidburn") {
		t.Fatalf("DOOR.SYS missing handle:\n%s", text)
	}
	if !strings.Contains(text, "45") {
		t.Fatalf("DOOR.SYS missing minutes left:\n%s", text)
	}
	if !strings.HasPrefix(text, "COM0:") {
		t.Fatalf("DOOR.SYS line 1 = not COM0:\n%s", text)
	}
	// Identity comes from settings, the DOS path from the door config -- no
	// hardcoded "C:\BBS\GEN" or fixed sysop name.
	if strings.Contains(text, "C:\\BBS\\GEN") {
		t.Fatalf("DOOR.SYS still has the hardcoded path:\n%s", text)
	}
	if !strings.Contains(text, "C:\\LORD") {
		t.Fatalf("DOOR.SYS missing configured DOS path:\n%s", text)
	}
	if !strings.Contains(text, "nut") {
		t.Fatalf("DOOR.SYS missing configured sysop name:\n%s", text)
	}
}

func TestWriteDropFileDorinfo(t *testing.T) {
	dir := t.TempDir()
	d := Door{Name: "TW2002", WorkDir: dir, DropType: "DORINFO1.DEF"}
	c := Caller{Node: 1, Handle: "ZeroCool", RealName: "Dade Murphy", SL: 50, MinutesLeft: 30, Emulation: 1, Baud: 38400}

	path, err := d.WriteDropFile(c, System{Name: "Acme BBS", Sysop: "Joe Admin"})
	if err != nil {
		t.Fatalf("WriteDropFile: %v", err)
	}
	if filepath.Base(path) != "DORINFO1.DEF" {
		t.Fatalf("path base = %q, want DORINFO1.DEF", filepath.Base(path))
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read drop: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "ZeroCool") {
		t.Fatalf("DORINFO1.DEF missing handle:\n%s", text)
	}
	if !strings.Contains(text, "30") {
		t.Fatalf("DORINFO1.DEF missing minutes left:\n%s", text)
	}
	// System and sysop names come from settings, not a baked-in "Vendetta/X".
	if !strings.HasPrefix(text, "Acme BBS\r\n") {
		t.Fatalf("DORINFO1.DEF line 1 should be the configured system name:\n%s", text)
	}
	if !strings.Contains(text, "Joe") || !strings.Contains(text, "Admin") {
		t.Fatalf("DORINFO1.DEF missing configured sysop name:\n%s", text)
	}
}
