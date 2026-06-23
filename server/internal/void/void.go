package void

import (
	"database/sql"
	"time"
)

// Trader is one player's persistent ship + captain.
type Trader struct {
	Handle string // owning board account (primary key)
	Name   string // captain / ship name

	Sector   int
	Credits  int
	Turns    int // actions left today
	HoldsMax int
	Cargo    [NumCommodities]int // Ore / Org / Equ aboard
	Fighters int
	Shields  int

	Alignment int
	Exp       int
	Kills     int

	LastPlayed string // YYYY-MM-DD -> daily turn reset
}

// HoldsUsed / HoldsFree account for cargo space.
func (t *Trader) HoldsUsed() int {
	return t.Cargo[Ore] + t.Cargo[Org] + t.Cargo[Equ]
}
func (t *Trader) HoldsFree() int {
	f := t.HoldsMax - t.HoldsUsed()
	if f < 0 {
		return 0
	}
	return f
}

// NetWorth is the ranking score: credits + cargo value + ship assets.
func (t *Trader) NetWorth() int {
	w := t.Credits
	for c := 0; c < NumCommodities; c++ {
		w += t.Cargo[c] * basePrice[c]
	}
	w += t.HoldsMax * HoldPrice / 2
	w += t.Fighters * FighterPrice
	w += t.Shields * ShieldPrice
	return w
}

// Store is the Voidfarer data layer over the shared *sql.DB.
type Store struct{ db *sql.DB }

// New creates the schema, generates the galaxy on first run, and returns the
// Store.
func New(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	if err := s.seedGalaxy(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS void_sectors (
	id   INTEGER PRIMARY KEY,
	name TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS void_warps (
	frm INTEGER NOT NULL,
	dst INTEGER NOT NULL,
	PRIMARY KEY (frm, dst)
);
CREATE TABLE IF NOT EXISTS void_ports (
	sector INTEGER PRIMARY KEY,
	class  INTEGER NOT NULL,
	ore    INTEGER NOT NULL DEFAULT 0,
	org    INTEGER NOT NULL DEFAULT 0,
	equ    INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS void_traders (
	handle      TEXT PRIMARY KEY,
	name        TEXT NOT NULL DEFAULT '',
	sector      INTEGER NOT NULL DEFAULT 1,
	credits     INTEGER NOT NULL DEFAULT 0,
	turns       INTEGER NOT NULL DEFAULT 0,
	holds_max   INTEGER NOT NULL DEFAULT 0,
	ore         INTEGER NOT NULL DEFAULT 0,
	org         INTEGER NOT NULL DEFAULT 0,
	equ         INTEGER NOT NULL DEFAULT 0,
	fighters    INTEGER NOT NULL DEFAULT 0,
	shields     INTEGER NOT NULL DEFAULT 0,
	alignment   INTEGER NOT NULL DEFAULT 0,
	exp         INTEGER NOT NULL DEFAULT 0,
	kills       INTEGER NOT NULL DEFAULT 0,
	last_played TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS void_fighters (
	sector INTEGER NOT NULL,
	owner  TEXT NOT NULL,
	qty    INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (sector, owner)
);`
	_, err := s.db.Exec(schema)
	return err
}

// NewTrader rolls a fresh captain starting at Sol.
func NewTrader(handle, name string) *Trader {
	if name == "" {
		name = handle
	}
	return &Trader{
		Handle: handle, Name: name,
		Sector: StartSector, Credits: StartCredits,
		Turns: DailyTurns, HoldsMax: StartHolds,
		Fighters: 20, Shields: 50,
	}
}

const traderCols = `handle, name, sector, credits, turns, holds_max, ore, org, equ,
	fighters, shields, alignment, exp, kills, last_played`

func scanTrader(row interface{ Scan(...any) error }) (*Trader, error) {
	var t Trader
	if err := row.Scan(&t.Handle, &t.Name, &t.Sector, &t.Credits, &t.Turns,
		&t.HoldsMax, &t.Cargo[Ore], &t.Cargo[Org], &t.Cargo[Equ], &t.Fighters,
		&t.Shields, &t.Alignment, &t.Exp, &t.Kills, &t.LastPlayed); err != nil {
		return nil, err
	}
	return &t, nil
}

// Load returns the trader for handle (found=false if none yet).
func (s *Store) Load(handle string) (t *Trader, found bool, err error) {
	row := s.db.QueryRow(`SELECT `+traderCols+` FROM void_traders WHERE handle = ? COLLATE NOCASE`, handle)
	t, err = scanTrader(row)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return t, true, nil
}

// Save upserts a trader.
func (s *Store) Save(t *Trader) error {
	_, err := s.db.Exec(`INSERT INTO void_traders
		(handle, name, sector, credits, turns, holds_max, ore, org, equ, fighters,
		 shields, alignment, exp, kills, last_played)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(handle) DO UPDATE SET
		 name=excluded.name, sector=excluded.sector, credits=excluded.credits,
		 turns=excluded.turns, holds_max=excluded.holds_max, ore=excluded.ore,
		 org=excluded.org, equ=excluded.equ, fighters=excluded.fighters,
		 shields=excluded.shields, alignment=excluded.alignment, exp=excluded.exp,
		 kills=excluded.kills, last_played=excluded.last_played`,
		t.Handle, t.Name, t.Sector, t.Credits, t.Turns, t.HoldsMax,
		t.Cargo[Ore], t.Cargo[Org], t.Cargo[Equ], t.Fighters, t.Shields,
		t.Alignment, t.Exp, t.Kills, t.LastPlayed)
	return err
}

// ResetIfNewDay refills the daily turn allowance the first time a trader plays
// on a new calendar day. Returns true if a reset happened.
func (t *Trader) ResetIfNewDay() bool {
	today := time.Now().Format("2006-01-02")
	if t.LastPlayed == today {
		return false
	}
	t.LastPlayed = today
	t.Turns = DailyTurns
	return true
}

// TradersInSector returns other traders currently parked in a sector (the PvP
// targets), strongest first.
func (s *Store) TradersInSector(sector int, exclude string) ([]Trader, error) {
	rows, err := s.db.Query(`SELECT `+traderCols+` FROM void_traders
		WHERE sector = ? AND handle <> ? COLLATE NOCASE ORDER BY fighters DESC`, sector, exclude)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Trader
	for rows.Next() {
		t, err := scanTrader(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// Rankings returns the top traders by net worth (the leaderboard).
func (s *Store) Rankings(limit int) ([]Trader, error) {
	rows, err := s.db.Query(`SELECT ` + traderCols + ` FROM void_traders`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var all []Trader
	for rows.Next() {
		t, err := scanTrader(rows)
		if err != nil {
			return nil, err
		}
		all = append(all, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Sort by net worth desc (small N; in-memory is fine).
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].NetWorth() > all[i].NetWorth() {
				all[i], all[j] = all[j], all[i]
			}
		}
	}
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}
