// Package lord is "Red Dragon" -- a native, persistent LORD-style BBS door
// (Legend of the Red Dragon clone). It renders directly over a term.Session and
// keeps each warrior in its own SQLite table over the shared database, so it
// plugs into the board like any other native door (no emulator, no drop files).
//
// This file holds the character model and its persistence; content.go holds the
// game data + combat math; game.go runs the town loop and the forest; the town
// services (shops, bank, healer, inn, player-vs-player) live in their own files.
package lord

import (
	"database/sql"
	"time"
)

// Character is one player's persistent warrior.
type Character struct {
	Handle string // owning board account (primary key)
	Name   string // in-game name (defaults to the handle)

	Level int
	Exp   int

	HP, MaxHP int
	Attack    int // base attack stat (weapon power is added on top)
	Defense   int // base defense stat (armor power is added on top)

	Gold int // gold on hand (lost on death; steal-able in PvP)
	Bank int // gold in the bank (safe)
	Gems int

	Charm   int
	WeaponN int // index into Weapons
	ArmorN  int // index into Armors

	ForestFights int // forest turns left today
	PlayerFights int // PvP turns left today

	Alive       bool
	Married     bool
	DragonKills int
	Kills       int // monsters + players slain
	Deaths      int

	LastPlayed string // YYYY-MM-DD, drives the once-a-day reset
}

// EffAttack / EffDefense fold in the equipped weapon/armor power.
func (c *Character) EffAttack() int  { return c.Attack + WeaponPower(c.WeaponN) }
func (c *Character) EffDefense() int { return c.Defense + ArmorPower(c.ArmorN) }

// Store is the Red Dragon data layer over the shared *sql.DB.
type Store struct{ db *sql.DB }

// New returns a Store, creating the lord_chars table if absent.
func New(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	const schema = `CREATE TABLE IF NOT EXISTS lord_chars (
		handle        TEXT PRIMARY KEY,
		name          TEXT NOT NULL DEFAULT '',
		level         INTEGER NOT NULL DEFAULT 1,
		exp           INTEGER NOT NULL DEFAULT 0,
		hp            INTEGER NOT NULL DEFAULT 0,
		maxhp         INTEGER NOT NULL DEFAULT 0,
		attack        INTEGER NOT NULL DEFAULT 0,
		defense       INTEGER NOT NULL DEFAULT 0,
		gold          INTEGER NOT NULL DEFAULT 0,
		bank          INTEGER NOT NULL DEFAULT 0,
		gems          INTEGER NOT NULL DEFAULT 0,
		charm         INTEGER NOT NULL DEFAULT 0,
		weapon        INTEGER NOT NULL DEFAULT 0,
		armor         INTEGER NOT NULL DEFAULT 0,
		forest_fights INTEGER NOT NULL DEFAULT 0,
		player_fights INTEGER NOT NULL DEFAULT 0,
		alive         INTEGER NOT NULL DEFAULT 1,
		married       INTEGER NOT NULL DEFAULT 0,
		dragon_kills  INTEGER NOT NULL DEFAULT 0,
		kills         INTEGER NOT NULL DEFAULT 0,
		deaths        INTEGER NOT NULL DEFAULT 0,
		last_played   TEXT NOT NULL DEFAULT ''
	);`
	if _, err := s.db.Exec(schema); err != nil {
		return nil, err
	}
	return s, nil
}

// NewCharacter rolls a fresh level-1 warrior for handle.
func NewCharacter(handle, name string) *Character {
	if name == "" {
		name = handle
	}
	c := &Character{
		Handle: handle, Name: name,
		Level: 1, Exp: 0,
		MaxHP: 20, HP: 20,
		Attack: 6, Defense: 2,
		Gold: 0, Bank: 0,
		ForestFights: DailyForestFights, PlayerFights: DailyPlayerFights,
		Alive: true,
	}
	return c
}

// scanCols is the column order shared by Load/Top/Others and Save.
const scanCols = `handle, name, level, exp, hp, maxhp, attack, defense, gold, bank,
	gems, charm, weapon, armor, forest_fights, player_fights, alive, married,
	dragon_kills, kills, deaths, last_played`

func scanChar(row interface{ Scan(...any) error }) (*Character, error) {
	var c Character
	var alive, married int
	if err := row.Scan(&c.Handle, &c.Name, &c.Level, &c.Exp, &c.HP, &c.MaxHP,
		&c.Attack, &c.Defense, &c.Gold, &c.Bank, &c.Gems, &c.Charm, &c.WeaponN,
		&c.ArmorN, &c.ForestFights, &c.PlayerFights, &alive, &married,
		&c.DragonKills, &c.Kills, &c.Deaths, &c.LastPlayed); err != nil {
		return nil, err
	}
	c.Alive = alive != 0
	c.Married = married != 0
	return &c, nil
}

// Load returns the warrior for handle (found=false if none yet).
func (s *Store) Load(handle string) (c *Character, found bool, err error) {
	row := s.db.QueryRow(`SELECT `+scanCols+` FROM lord_chars WHERE handle = ? COLLATE NOCASE`, handle)
	c, err = scanChar(row)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return c, true, nil
}

// Save upserts a warrior.
func (s *Store) Save(c *Character) error {
	_, err := s.db.Exec(`INSERT INTO lord_chars
		(handle, name, level, exp, hp, maxhp, attack, defense, gold, bank, gems,
		 charm, weapon, armor, forest_fights, player_fights, alive, married,
		 dragon_kills, kills, deaths, last_played)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(handle) DO UPDATE SET
		 name=excluded.name, level=excluded.level, exp=excluded.exp, hp=excluded.hp,
		 maxhp=excluded.maxhp, attack=excluded.attack, defense=excluded.defense,
		 gold=excluded.gold, bank=excluded.bank, gems=excluded.gems, charm=excluded.charm,
		 weapon=excluded.weapon, armor=excluded.armor, forest_fights=excluded.forest_fights,
		 player_fights=excluded.player_fights, alive=excluded.alive, married=excluded.married,
		 dragon_kills=excluded.dragon_kills, kills=excluded.kills, deaths=excluded.deaths,
		 last_played=excluded.last_played`,
		c.Handle, c.Name, c.Level, c.Exp, c.HP, c.MaxHP, c.Attack, c.Defense,
		c.Gold, c.Bank, c.Gems, c.Charm, c.WeaponN, c.ArmorN, c.ForestFights,
		c.PlayerFights, b2i(c.Alive), b2i(c.Married), c.DragonKills, c.Kills,
		c.Deaths, c.LastPlayed)
	return err
}

// Others returns up to n other warriors (not handle), strongest first -- the
// pool the player can challenge and the leaderboard reads from.
func (s *Store) Others(handle string, n int) ([]Character, error) {
	rows, err := s.db.Query(`SELECT `+scanCols+` FROM lord_chars
		WHERE handle <> ? COLLATE NOCASE
		ORDER BY level DESC, exp DESC LIMIT ?`, handle, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Character
	for rows.Next() {
		c, err := scanChar(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// ResetIfNewDay applies the once-a-day reset (forest/player turns refilled, HP
// restored, revived) the first time a warrior plays on a new calendar day.
// Returns true if a reset happened.
func (c *Character) ResetIfNewDay() bool {
	today := time.Now().Format("2006-01-02")
	if c.LastPlayed == today {
		return false
	}
	c.LastPlayed = today
	c.ForestFights = DailyForestFights
	c.PlayerFights = DailyPlayerFights
	c.Alive = true
	c.HP = c.MaxHP
	return true
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
