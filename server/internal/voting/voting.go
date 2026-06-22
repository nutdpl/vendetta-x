// Package voting is Vendetta/X's Voting Booth: polls with options and one vote
// per user. It owns its own SQLite tables over the shared database handle.
// This file is the CONTRACT; New creates the tables and methods are stubs.
package voting

import (
	"database/sql"
	"errors"
	"time"

	"vendetta-x/server/internal/sanitize"
)

// Poll is one voting-booth question.
type Poll struct {
	ID       int64
	Question string
	Author   string
	Created  time.Time
	Closed   bool
}

// Option is one choice on a poll, with its current vote tally.
type Option struct {
	ID     int64
	PollID int64
	Text   string
	Votes  int
}

// Store is the voting data layer over the shared *sql.DB.
type Store struct {
	db *sql.DB
}

// ErrAlreadyVoted is returned by Vote when the voter has already voted on the
// poll (the one-vote-per-voter-per-poll rule).
var ErrAlreadyVoted = errors.New("voting: already voted on this poll")

// ErrBadOption is returned by Vote when optionID does not belong to pollID.
var ErrBadOption = errors.New("voting: option does not belong to poll")

// New returns a Store, creating the polls/options/votes tables if absent and
// seeding one sample poll into an empty booth (idempotent).
func New(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	if err := s.seed(); err != nil {
		return nil, err
	}
	return s, nil
}

// migrate creates the three tables: polls, poll_options, and poll_votes with a
// UNIQUE(poll_id, voter) so a user votes at most once per poll.
func (s *Store) migrate() error {
	const polls = `CREATE TABLE IF NOT EXISTS polls (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		question TEXT NOT NULL DEFAULT '',
		author TEXT NOT NULL DEFAULT '',
		created INTEGER NOT NULL DEFAULT 0,
		closed INTEGER NOT NULL DEFAULT 0
	);`
	const options = `CREATE TABLE IF NOT EXISTS poll_options (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		poll_id INTEGER NOT NULL,
		text TEXT NOT NULL DEFAULT ''
	);`
	const votes = `CREATE TABLE IF NOT EXISTS poll_votes (
		poll_id INTEGER NOT NULL,
		option_id INTEGER NOT NULL,
		voter TEXT NOT NULL,
		UNIQUE(poll_id, voter)
	);`
	for _, schema := range []string{polls, options, votes} {
		if _, err := s.db.Exec(schema); err != nil {
			return err
		}
	}
	return nil
}

// seed inserts a sample poll if the booth is empty (idempotent).
func (s *Store) seed() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM polls`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err := s.Create(
		"What's the truest face of the scene?",
		"phiber",
		[]string{
			"ANSI art and the demoscene",
			"0day couriers and FXP boards",
			"Phreaking and the blue box era",
			"It's the people, always was",
		})
	return err
}

// Polls returns all polls, newest first.
func (s *Store) Polls() ([]Poll, error) {
	rows, err := s.db.Query(
		`SELECT id, question, author, created, closed FROM polls
		 ORDER BY created DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Poll
	for rows.Next() {
		p, err := scanPoll(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// Poll returns one poll with its options (Votes populated), or nil if missing.
func (s *Store) Poll(id int64) (*Poll, []Option, error) {
	row := s.db.QueryRow(
		`SELECT id, question, author, created, closed FROM polls WHERE id = ?`, id)
	p, err := scanPoll(row)
	if err == sql.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	opts, _, err := s.Results(id)
	if err != nil {
		return nil, nil, err
	}
	return p, opts, nil
}

// Create adds a poll with the given options, returning the new poll id. Blank
// option strings are ignored; at least two options are required.
func (s *Store) Create(question, author string, options []string) (int64, error) {
	question = sanitize.Line(question)
	author = sanitize.Line(author)
	clean := make([]string, 0, len(options))
	for _, o := range options {
		if o = sanitize.Line(o); o != "" {
			clean = append(clean, o)
		}
	}
	if len(clean) < 2 {
		return 0, errors.New("voting: a poll needs at least two options")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`INSERT INTO polls (question, author, created, closed) VALUES (?, ?, ?, 0)`,
		question, author, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	pollID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for _, text := range clean {
		if _, err := tx.Exec(
			`INSERT INTO poll_options (poll_id, text) VALUES (?, ?)`, pollID, text); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return pollID, nil
}

// SetClosed sets the closed flag on a poll (stored as 1/0).
func (s *Store) SetClosed(id int64, closed bool) error {
	v := 0
	if closed {
		v = 1
	}
	_, err := s.db.Exec(`UPDATE polls SET closed = ? WHERE id = ?`, v, id)
	return err
}

// Delete removes a poll along with its options and votes in one transaction.
func (s *Store) Delete(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM poll_votes WHERE poll_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM poll_options WHERE poll_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM polls WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// Vote records voter's choice of optionID on pollID. optionID must belong to
// pollID; a second vote by the same voter returns ErrAlreadyVoted.
func (s *Store) Vote(pollID, optionID int64, voter string) error {
	// optionID must belong to pollID.
	var n int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM poll_options WHERE id = ? AND poll_id = ?`,
		optionID, pollID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrBadOption
	}

	// One vote per voter per poll: check first for a clean error, and rely on
	// the UNIQUE(poll_id, voter) constraint as the backstop against races.
	voted, err := s.HasVoted(pollID, voter)
	if err != nil {
		return err
	}
	if voted {
		return ErrAlreadyVoted
	}

	if _, err := s.db.Exec(
		`INSERT INTO poll_votes (poll_id, option_id, voter) VALUES (?, ?, ?)`,
		pollID, optionID, voter); err != nil {
		// A UNIQUE violation here means a concurrent vote landed first.
		return ErrAlreadyVoted
	}
	return nil
}

// HasVoted reports whether voter has already voted on pollID.
func (s *Store) HasVoted(pollID int64, voter string) (bool, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM poll_votes WHERE poll_id = ? AND voter = ?`,
		pollID, voter).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// Results returns a poll's options with vote tallies and the total vote count.
func (s *Store) Results(pollID int64) ([]Option, int, error) {
	rows, err := s.db.Query(
		`SELECT o.id, o.poll_id, o.text,
		        (SELECT COUNT(*) FROM poll_votes v WHERE v.option_id = o.id) AS votes
		 FROM poll_options o
		 WHERE o.poll_id = ?
		 ORDER BY o.id`, pollID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var (
		out   []Option
		total int
	)
	for rows.Next() {
		var o Option
		if err := rows.Scan(&o.ID, &o.PollID, &o.Text, &o.Votes); err != nil {
			return nil, 0, err
		}
		total += o.Votes
		out = append(out, o)
	}
	return out, total, rows.Err()
}

// scanner is the shared interface of *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...interface{}) error
}

// scanPoll decodes one polls row (id, question, author, created unix, closed)
// into a Poll, converting the Unix timestamp into a time.Time.
func scanPoll(r scanner) (*Poll, error) {
	var (
		p       Poll
		created int64
		closed  int
	)
	if err := r.Scan(&p.ID, &p.Question, &p.Author, &created, &closed); err != nil {
		return nil, err
	}
	if created != 0 {
		p.Created = time.Unix(created, 0)
	}
	p.Closed = closed != 0
	return &p, nil
}
