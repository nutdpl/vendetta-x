package store

import (
	"fmt"
	"strings"
)

// Board-wide search: substring matching over the message and file corpora,
// restricted to the set of boards/areas the caller may read. Access control
// stays with the caller (it already holds the ACS evaluator and the user's
// subject); the store just takes the resolved id set and filters to it, so a
// search can never surface content from a base the caller can't open.

// likeEscape escapes a user query for use inside a LIKE pattern with
// `ESCAPE '\'`, so the wildcards a caller might type (`%`, `_`) and the escape
// char itself are matched literally instead of acting as wildcards.
func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// idPlaceholders builds the "?,?,?" list and appends the ids to args for an
// `IN (...)` clause.
func idPlaceholders(ids []int64, args *[]any) string {
	ph := make([]string, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		*args = append(*args, id)
	}
	return strings.Join(ph, ",")
}

// SearchMessages returns messages whose subject, body, or author contains the
// query (case-insensitive substring), restricted to boardIDs -- the caller's
// ACS-readable set -- newest first. An empty query or an empty id set yields no
// rows (a caller with nothing to search reads nothing). limit<=0 means all.
func (s *Store) SearchMessages(query string, boardIDs []int64, limit int) ([]Message, error) {
	query = strings.TrimSpace(query)
	if query == "" || len(boardIDs) == 0 {
		return nil, nil
	}
	pat := "%" + likeEscape(query) + "%"
	args := []any{pat, pat, pat}
	in := idPlaceholders(boardIDs, &args)
	q := `SELECT ` + msgCols + ` FROM messages
	      WHERE (subject LIKE ? ESCAPE '\' OR body LIKE ? ESCAPE '\' OR from_who LIKE ? ESCAPE '\')
	        AND board_id IN (` + in + `)
	      ORDER BY posted DESC, id DESC`
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: search messages: %w", err)
	}
	return scanMessages(rows)
}

// SearchFiles returns approved files whose filename, description, or uploader
// contains the query (case-insensitive substring), restricted to areaIDs --
// the caller's ACS-accessible set -- newest upload first. Queue-held uploads
// stay invisible. An empty query or an empty id set yields no rows. limit<=0
// means all.
func (s *Store) SearchFiles(query string, areaIDs []int64, limit int) ([]FileEntry, error) {
	query = strings.TrimSpace(query)
	if query == "" || len(areaIDs) == 0 {
		return nil, nil
	}
	pat := "%" + likeEscape(query) + "%"
	args := []any{pat, pat, pat}
	in := idPlaceholders(areaIDs, &args)
	q := `SELECT ` + fileCols + ` FROM files
	      WHERE approved != 0
	        AND (filename LIKE ? ESCAPE '\' OR descr LIKE ? ESCAPE '\' OR uploader LIKE ? ESCAPE '\')
	        AND area_id IN (` + in + `)
	      ORDER BY uploaded DESC, id DESC`
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: search files: %w", err)
	}
	return scanFileRows(rows, "search files")
}
