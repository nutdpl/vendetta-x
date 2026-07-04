package store

import (
	"database/sql"
	"fmt"
)

// Read pointers (the classic qscan): per user, per board, the highest message
// id the caller has seen. "New" everywhere on the board means "id above your
// pointer" -- the reader advances it, the new-scan resumes from it, and the
// pickers/logon surface the counts.

// LastRead returns the user's read pointer for a board (0 = never read it).
func (s *Store) LastRead(userID, boardID int64) (int64, error) {
	var id int64
	err := s.db.QueryRow(
		`SELECT last_msg_id FROM lastread WHERE user_id = ? AND board_id = ?`,
		userID, boardID).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("store: last read: %w", err)
	}
	return id, nil
}

// SetLastRead advances the user's read pointer for a board. It is monotonic:
// a stale or out-of-order update can never move the pointer backwards, so
// re-reading an old message doesn't resurrect everything after it as "new".
func (s *Store) SetLastRead(userID, boardID, msgID int64) error {
	_, err := s.db.Exec(
		`INSERT INTO lastread (user_id, board_id, last_msg_id) VALUES (?, ?, ?)
		 ON CONFLICT(user_id, board_id) DO UPDATE
		 SET last_msg_id = MAX(last_msg_id, excluded.last_msg_id)`,
		userID, boardID, msgID)
	if err != nil {
		return fmt.Errorf("store: set last read: %w", err)
	}
	return nil
}

// UnreadCounts returns, for every board with any, the number of messages newer
// than the user's read pointer. Boards absent from the map have nothing new.
// The caller applies its own ACS filtering -- this is a pure count.
func (s *Store) UnreadCounts(userID int64) (map[int64]int, error) {
	rows, err := s.db.Query(
		`SELECT m.board_id, COUNT(*)
		 FROM messages m
		 LEFT JOIN lastread lr ON lr.board_id = m.board_id AND lr.user_id = ?
		 WHERE m.id > COALESCE(lr.last_msg_id, 0)
		 GROUP BY m.board_id`, userID)
	if err != nil {
		return nil, fmt.Errorf("store: unread counts: %w", err)
	}
	defer rows.Close()
	counts := map[int64]int{}
	for rows.Next() {
		var boardID int64
		var n int
		if err := rows.Scan(&boardID, &n); err != nil {
			return nil, fmt.Errorf("store: unread counts scan: %w", err)
		}
		counts[boardID] = n
	}
	return counts, rows.Err()
}

// MessagesAfter returns a board's messages with id > afterID, oldest first --
// the new-scan feed (unlike LocalMessagesAfter it includes network imports;
// a caller doesn't care where a new message came from).
func (s *Store) MessagesAfter(boardID, afterID int64) ([]Message, error) {
	rows, err := s.db.Query(
		`SELECT `+msgCols+`
		 FROM messages WHERE board_id = ? AND id > ?
		 ORDER BY id`, boardID, afterID)
	if err != nil {
		return nil, fmt.Errorf("store: messages after %d: %w", afterID, err)
	}
	return scanMessages(rows)
}
