package store

import (
	"fmt"
	"time"
)

// Board statistics for the sysop dashboard: cheap aggregates over the data
// already on disk (timestamps, counts, sizes). All read-only.

// Totals is the headline count of everything on the board.
type Totals struct {
	Users       int
	Posts       int
	Files       int
	StoredBytes int64 // total bytes of approved file content
}

// Totals returns the headline counts.
func (s *Store) Totals() (Totals, error) {
	var t Totals
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&t.Users); err != nil {
		return t, fmt.Errorf("store: totals users: %w", err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&t.Posts); err != nil {
		return t, fmt.Errorf("store: totals posts: %w", err)
	}
	if err := s.db.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(size), 0) FROM files WHERE approved != 0`).Scan(&t.Files, &t.StoredBytes); err != nil {
		return t, fmt.Errorf("store: totals files: %w", err)
	}
	return t, nil
}

// DayStat is one day's activity for the 30-day sparkline.
type DayStat struct {
	Day     string // "YYYY-MM-DD" (local)
	Posts   int
	Uploads int
	Signups int
}

// DailyActivity returns per-day counts of new posts, uploads, and signups over
// the last `days` days, oldest first and zero-filled so the series is
// continuous for a sparkline. days<=0 defaults to 30.
func (s *Store) DailyActivity(days int) ([]DayStat, error) {
	if days <= 0 {
		days = 30
	}
	out := make([]DayStat, days)
	idx := make(map[string]*DayStat, days)
	now := time.Now()
	for i := 0; i < days; i++ {
		d := now.AddDate(0, 0, -(days - 1 - i)).Format("2006-01-02")
		out[i] = DayStat{Day: d}
		idx[d] = &out[i]
	}
	// Each source stores its timestamp as unix seconds; bucket by local day.
	sources := []struct {
		query string
		set   func(*DayStat, int)
	}{
		{`SELECT date(posted,'unixepoch','localtime') d, COUNT(*) FROM messages WHERE posted > 0 GROUP BY d`,
			func(ds *DayStat, n int) { ds.Posts = n }},
		{`SELECT date(uploaded,'unixepoch','localtime') d, COUNT(*) FROM files WHERE approved != 0 AND uploaded > 0 GROUP BY d`,
			func(ds *DayStat, n int) { ds.Uploads = n }},
		{`SELECT date(first_call,'unixepoch','localtime') d, COUNT(*) FROM users WHERE first_call > 0 GROUP BY d`,
			func(ds *DayStat, n int) { ds.Signups = n }},
	}
	for _, src := range sources {
		rows, err := s.db.Query(src.query)
		if err != nil {
			return nil, fmt.Errorf("store: daily activity: %w", err)
		}
		for rows.Next() {
			var day string
			var n int
			if err := rows.Scan(&day, &n); err != nil {
				rows.Close()
				return nil, fmt.Errorf("store: daily activity scan: %w", err)
			}
			if ds := idx[day]; ds != nil {
				src.set(ds, n)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("store: daily activity rows: %w", err)
		}
		rows.Close()
	}
	return out, nil
}

// BoardCount pairs a board name with its message count, for the "busiest
// bases" ranking.
type BoardCount struct {
	Name  string
	Count int
}

// TopBoards returns the boards with the most messages, busiest first, capped at
// n (n<=0 means all).
func (s *Store) TopBoards(n int) ([]BoardCount, error) {
	q := `SELECT b.name, COUNT(m.id) c
	      FROM boards b LEFT JOIN messages m ON m.board_id = b.id
	      GROUP BY b.id ORDER BY c DESC, b.name`
	if n > 0 {
		q += fmt.Sprintf(" LIMIT %d", n)
	}
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("store: top boards: %w", err)
	}
	defer rows.Close()
	var out []BoardCount
	for rows.Next() {
		var bc BoardCount
		if err := rows.Scan(&bc.Name, &bc.Count); err != nil {
			return nil, fmt.Errorf("store: top boards scan: %w", err)
		}
		out = append(out, bc)
	}
	return out, rows.Err()
}
