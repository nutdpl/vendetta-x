package store

import (
	"fmt"
	"strings"
)

// Personal message scan: public posts a caller is addressed by name in the
// `To` field (replies to their posts, or anything sent to their handle) --
// the classic "you have messages waiting" that isn't private mail. Restricted
// to the boards the caller can read; "All" (the broadcast recipient) never
// counts as personal.

// MessagesToHandle returns messages in boardIDs whose recipient is handle
// (case-insensitive, excluding the "All" broadcast), newest first, capped at
// limit (<=0 means all). An empty handle or id set yields nothing.
func (s *Store) MessagesToHandle(handle string, boardIDs []int64, limit int) ([]Message, error) {
	handle = strings.TrimSpace(handle)
	if handle == "" || len(boardIDs) == 0 {
		return nil, nil
	}
	args := []any{handle}
	in := idPlaceholders(boardIDs, &args)
	q := `SELECT ` + msgCols + ` FROM messages
	      WHERE to_who = ? COLLATE NOCASE AND to_who <> 'All' COLLATE NOCASE
	        AND board_id IN (` + in + `)
	      ORDER BY posted DESC, id DESC`
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: messages to handle: %w", err)
	}
	return scanMessages(rows)
}

// UnreadToHandle counts messages addressed to handle in boardIDs that sit above
// the user's read pointer -- the "N addressed to you" figure for the logon
// digest. Mirrors UnreadCounts' lastread join.
func (s *Store) UnreadToHandle(userID int64, handle string, boardIDs []int64) (int, error) {
	handle = strings.TrimSpace(handle)
	if handle == "" || len(boardIDs) == 0 {
		return 0, nil
	}
	args := []any{userID, handle}
	in := idPlaceholders(boardIDs, &args)
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM messages m
		 LEFT JOIN lastread lr ON lr.board_id = m.board_id AND lr.user_id = ?
		 WHERE m.to_who = ? COLLATE NOCASE AND m.to_who <> 'All' COLLATE NOCASE
		   AND m.board_id IN (`+in+`)
		   AND m.id > COALESCE(lr.last_msg_id, 0)`, args...).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: unread to handle: %w", err)
	}
	return n, nil
}
