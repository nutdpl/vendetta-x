package void

import (
	"database/sql"
	"math/rand"
	"sort"
)

// Sector is one location in the galaxy: a name and the sectors it warps to.
type Sector struct {
	ID    int
	Name  string
	Warps []int
}

// Port is a trading station in a sector. Class 1..8 are the BBS/SSB trade
// classes (see content.go); class 0 is StarDock, class 9 is Terra (Sol).
type Port struct {
	Sector int
	Class  int
	Stock  [NumCommodities]int
}

// StarDock / Terra sector placement (inside Federation space).
const (
	StarDockSector = 5
	TerraSector    = 1
)

// IsStarDock / IsTerra classify the special ports.
func (p *Port) IsStarDock() bool { return p != nil && p.Class == 0 }
func (p *Port) IsTerra() bool    { return p != nil && p.Class == 9 }

// seedGalaxy generates the shared universe once, into the sectors/warps/ports
// tables. It is idempotent: if any sector exists it does nothing.
func (s *Store) seedGalaxy() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM void_sectors`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	rng := rand.New(rand.NewSource(20260623)) // fixed -> a stable galaxy

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for i := 1; i <= TotalSectors; i++ {
		name := "Uncharted"
		if i == TerraSector {
			name = "Sol"
		}
		if _, err := tx.Exec(`INSERT INTO void_sectors (id, name) VALUES (?, ?)`, i, name); err != nil {
			return err
		}
	}

	// Warps: a connected backbone (a shuffled path through every sector) so the
	// galaxy is fully reachable, then a few random extra two-way warps each.
	order := rng.Perm(TotalSectors) // 0..N-1
	link := func(a, b int) error {
		if a == b {
			return nil
		}
		_, err := tx.Exec(`INSERT OR IGNORE INTO void_warps (frm, dst) VALUES (?, ?), (?, ?)`, a, b, b, a)
		return err
	}
	for i := 0; i+1 < len(order); i++ {
		if err := link(order[i]+1, order[i+1]+1); err != nil {
			return err
		}
	}
	for i := 1; i <= TotalSectors; i++ {
		extra := 1 + rng.Intn(3) // 1..3 extra warps
		for j := 0; j < extra; j++ {
			if err := link(i, 1+rng.Intn(TotalSectors)); err != nil {
				return err
			}
		}
	}

	// Special ports.
	if err := insertPort(tx, TerraSector, 9); err != nil {
		return err
	}
	if err := insertPort(tx, StarDockSector, 0); err != nil {
		return err
	}
	// Trading ports: ~40% of the remaining sectors get a class 1..8 port.
	for i := 2; i <= TotalSectors; i++ {
		if i == StarDockSector {
			continue
		}
		if rng.Intn(100) < 40 {
			if err := insertPort(tx, i, 1+rng.Intn(8)); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// insertPort creates a port and stocks it: goods it SELLS start full (cheap to
// buy), goods it BUYS start empty (it pays well) -- instant arbitrage.
func insertPort(tx *sql.Tx, sector, class int) error {
	var st [NumCommodities]int
	for c := 0; c < NumCommodities; c++ {
		if PortSells(class, c) {
			st[c] = MaxStock
		} else {
			st[c] = 0
		}
	}
	_, err := tx.Exec(`INSERT INTO void_ports (sector, class, ore, org, equ) VALUES (?, ?, ?, ?, ?)`,
		sector, class, st[Ore], st[Org], st[Equ])
	return err
}

// Sector returns a sector with its (sorted) warps, or nil if out of range.
func (s *Store) Sector(id int) (*Sector, error) {
	var sec Sector
	err := s.db.QueryRow(`SELECT id, name FROM void_sectors WHERE id = ?`, id).Scan(&sec.ID, &sec.Name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT dst FROM void_warps WHERE frm = ? ORDER BY dst`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var d int
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		sec.Warps = append(sec.Warps, d)
	}
	sort.Ints(sec.Warps)
	return &sec, rows.Err()
}

// WarpExists reports whether you can move directly from -> to.
func (s *Store) WarpExists(from, to int) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM void_warps WHERE frm = ? AND dst = ?`, from, to).Scan(&n)
	return n > 0, err
}

// PortAt returns the port in a sector, or nil if the sector has none.
func (s *Store) PortAt(sector int) (*Port, error) {
	var p Port
	err := s.db.QueryRow(`SELECT sector, class, ore, org, equ FROM void_ports WHERE sector = ?`, sector).
		Scan(&p.Sector, &p.Class, &p.Stock[Ore], &p.Stock[Org], &p.Stock[Equ])
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// SavePort persists a port's (possibly traded-down) stock.
func (s *Store) SavePort(p *Port) error {
	_, err := s.db.Exec(`UPDATE void_ports SET ore = ?, org = ?, equ = ? WHERE sector = ?`,
		p.Stock[Ore], p.Stock[Org], p.Stock[Equ], p.Sector)
	return err
}

// ---- deployed sector fighters (territorial control / tolls) ----------------

// SectorFighters is one trader's fighters parked in a sector.
type SectorFighters struct {
	Owner string
	Qty   int
}

// FightersAt returns every trader's fighters deployed in a sector.
func (s *Store) FightersAt(sector int) ([]SectorFighters, error) {
	rows, err := s.db.Query(`SELECT owner, qty FROM void_fighters WHERE sector = ? AND qty > 0 ORDER BY qty DESC`, sector)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SectorFighters
	for rows.Next() {
		var f SectorFighters
		if err := rows.Scan(&f.Owner, &f.Qty); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// SetFighters sets (or clears, when qty<=0) one owner's fighters in a sector.
func (s *Store) SetFighters(sector int, owner string, qty int) error {
	if qty <= 0 {
		_, err := s.db.Exec(`DELETE FROM void_fighters WHERE sector = ? AND owner = ? COLLATE NOCASE`, sector, owner)
		return err
	}
	_, err := s.db.Exec(`INSERT INTO void_fighters (sector, owner, qty) VALUES (?, ?, ?)
		ON CONFLICT(sector, owner) DO UPDATE SET qty = excluded.qty`, sector, owner, qty)
	return err
}
