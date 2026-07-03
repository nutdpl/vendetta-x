package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Backup writes a consistent snapshot of the live database into dir (created
// if missing) using SQLite's VACUUM INTO -- safe under WAL with callers
// online -- then prunes old snapshots so at most keep remain. Returns the
// path of the snapshot it wrote. The whole board lives in one SQLite file;
// this is the "don't lose the community" button.
func (s *Store) Backup(dir string, keep int) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("store: backup dir: %w", err)
	}
	name := "backup-" + time.Now().Format("20060102-150405") + ".db"
	path := filepath.Join(dir, name)

	// VACUUM INTO takes a filename literal, not a bind parameter; single
	// quotes in the path are SQL-escaped by doubling.
	quoted := strings.ReplaceAll(path, "'", "''")
	if _, err := s.db.Exec("VACUUM INTO '" + quoted + "'"); err != nil {
		return "", fmt.Errorf("store: vacuum into %s: %w", path, err)
	}

	if keep > 0 {
		if err := pruneBackups(dir, keep); err != nil {
			return path, fmt.Errorf("store: prune backups: %w", err)
		}
	}
	return path, nil
}

// pruneBackups deletes the oldest backup-*.db files beyond keep. Names embed
// their timestamp, so lexical order is age order.
func pruneBackups(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "backup-") && strings.HasSuffix(e.Name(), ".db") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for len(names) > keep {
		if err := os.Remove(filepath.Join(dir, names[0])); err != nil {
			return err
		}
		names = names[1:]
	}
	return nil
}
