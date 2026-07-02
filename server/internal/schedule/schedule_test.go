package schedule

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCRUD(t *testing.T) {
	s, err := New(openTestDB(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	id, err := s.Add(&Event{Name: "Nightly purge", Action: "messages.purge_old", TimeOfDay: "03:00", Enabled: true, Interval: 45})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, err := s.Get(id)
	if err != nil || got == nil {
		t.Fatalf("Get: %v, %v", got, err)
	}
	if got.Name != "Nightly purge" || got.Action != "messages.purge_old" || got.TimeOfDay != "03:00" || !got.Enabled || got.Interval != 45 {
		t.Fatalf("Get returned wrong event: %+v", got)
	}
	if !got.LastRun.IsZero() {
		t.Fatalf("new event should have zero LastRun, got %v", got.LastRun)
	}

	got.Name = "Nightly purge (renamed)"
	got.Enabled = false
	if err := s.Update(got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	reread, err := s.Get(id)
	if err != nil || reread.Name != "Nightly purge (renamed)" || reread.Enabled {
		t.Fatalf("Update didn't stick: %+v, err=%v", reread, err)
	}

	now := time.Now()
	if err := s.MarkRun(id, now); err != nil {
		t.Fatalf("MarkRun: %v", err)
	}
	reread, _ = s.Get(id)
	if reread.LastRun.Unix() != now.Unix() {
		t.Fatalf("MarkRun didn't stick: got %v want %v", reread.LastRun, now)
	}

	all, err := s.List()
	if err != nil || len(all) != 1 {
		t.Fatalf("List: %v, %+v", err, all)
	}

	if err := s.Delete(id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if gone, _ := s.Get(id); gone != nil {
		t.Fatalf("event still present after Delete: %+v", gone)
	}
}

func TestGetMissing(t *testing.T) {
	s, err := New(openTestDB(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := s.Get(999)
	if err != nil || got != nil {
		t.Fatalf("Get(missing) = %+v, %v, want nil, nil", got, err)
	}
}

func at(hh, mm int) time.Time {
	return time.Date(2026, 7, 1, hh, mm, 0, 0, time.UTC)
}

func TestDueAt(t *testing.T) {
	cases := []struct {
		name string
		e    Event
		now  time.Time
		want bool
	}{
		{
			name: "disabled never due",
			e:    Event{Enabled: false, TimeOfDay: "03:00"},
			now:  at(4, 0),
			want: false,
		},
		{
			name: "before time of day",
			e:    Event{Enabled: true, TimeOfDay: "03:00"},
			now:  at(2, 59),
			want: false,
		},
		{
			name: "at time of day, never run",
			e:    Event{Enabled: true, TimeOfDay: "03:00"},
			now:  at(3, 0),
			want: true,
		},
		{
			name: "after time of day, never run",
			e:    Event{Enabled: true, TimeOfDay: "03:00"},
			now:  at(9, 30),
			want: true,
		},
		{
			name: "already ran today, at due moment",
			e:    Event{Enabled: true, TimeOfDay: "03:00", LastRun: at(3, 0)},
			now:  at(3, 5),
			want: false,
		},
		{
			name: "already ran today, later",
			e:    Event{Enabled: true, TimeOfDay: "03:00", LastRun: at(3, 1)},
			now:  at(20, 0),
			want: false,
		},
		{
			name: "ran yesterday, due again today",
			e:    Event{Enabled: true, TimeOfDay: "03:00", LastRun: at(3, 1).Add(-24 * time.Hour)},
			now:  at(3, 0),
			want: true,
		},
		{
			name: "malformed time of day",
			e:    Event{Enabled: true, TimeOfDay: "not-a-time"},
			now:  at(12, 0),
			want: false,
		},
		{
			name: "out of range hour",
			e:    Event{Enabled: true, TimeOfDay: "25:00"},
			now:  at(12, 0),
			want: false,
		},
		{
			name: "interval, never run: due immediately",
			e:    Event{Enabled: true, Interval: 60},
			now:  at(0, 1),
			want: true,
		},
		{
			name: "interval not yet elapsed",
			e:    Event{Enabled: true, Interval: 60, LastRun: at(10, 0)},
			now:  at(10, 59),
			want: false,
		},
		{
			name: "interval elapsed exactly",
			e:    Event{Enabled: true, Interval: 60, LastRun: at(10, 0)},
			now:  at(11, 0),
			want: true,
		},
		{
			name: "interval wins over a bogus time of day",
			e:    Event{Enabled: true, Interval: 30, TimeOfDay: "garbage", LastRun: at(9, 0)},
			now:  at(9, 45),
			want: true,
		},
		{
			name: "interval disabled",
			e:    Event{Enabled: false, Interval: 5},
			now:  at(12, 0),
			want: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.e.DueAt(c.now); got != c.want {
				t.Errorf("DueAt() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestCatalogKeysAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range Catalog {
		if seen[a.Key] {
			t.Errorf("duplicate catalog key %q", a.Key)
		}
		seen[a.Key] = true
		if a.Label == "" || a.Desc == "" {
			t.Errorf("catalog entry %q missing label/desc", a.Key)
		}
	}
}
