// Package store is the app's database: a SQLite file the core owns.
//
// It is pure Go (modernc.org/sqlite, no cgo) so it builds wherever the core
// builds, and so `go test ./store/` covers it on a laptop without a phone. It
// decides what the table looks like; it does not decide where the file lives —
// that path arrives from ArkTS, because a Go library inside HarmonyOS has no
// environment to read it from (empty argv and envp, see ../README.md).
//
// One file, one owner. This package is the owner of the file it opened; the
// platform's relationalStore is the owner of the one it manages. Pointing both
// at the same file is SQLITE_BUSY waiting to happen, which is why the demo's
// comparison card keeps them in two different files.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"

	_ "modernc.org/sqlite"
)

// Row is one row of the table the demo keeps.
type Row struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

const schema = `CREATE TABLE IF NOT EXISTS rows (
	id    INTEGER PRIMARY KEY,
	title TEXT    NOT NULL,
	done  INTEGER NOT NULL
)`

// Store is one SQLite file. Its methods are safe for concurrent use.
type Store struct {
	mu   sync.Mutex
	db   *sql.DB
	path string
}

// Open opens the database at path, creating it if it isn't there. The path is
// the app's sandbox, handed over by ArkTS.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("store: no path")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// SQLite has one writer at a time anyway. One connection says so instead
	// of letting the pool turn it into "database is locked" under load.
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{
		// A reader never waits for the writer.
		"PRAGMA journal_mode=WAL",
		// A commit doesn't wait for the platter.
		"PRAGMA synchronous=NORMAL",
		// And if something else does hold the file, wait rather than fail.
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, err
		}
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, path: path}, nil
}

// Path is where this store lives.
func (s *Store) Path() string { return s.path }

// Seed replaces the table with n rows, in one transaction with one prepared
// statement: the way a bulk import is supposed to be written. The difference
// against one commit per row is the largest number in ../bench/sqlite (9.6 µs
// vs 0.67 µs per row), and it has nothing to do with which language asks.
func (s *Store) Seed(n int) (int, error) {
	if n < 0 {
		return 0, errors.New("store: negative row count")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() // a no-op after a successful Commit

	if _, err := tx.Exec("DELETE FROM rows"); err != nil {
		return 0, err
	}
	stmt, err := tx.Prepare("INSERT INTO rows (id, title, done) VALUES (?, ?, ?)")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	for i := range n {
		// The title is built here, inside the timed call, for both sides of
		// the comparison: the platform's writer builds its own too.
		title := fmt.Sprintf("row %d", i)
		if _, err := stmt.Exec(i, title, i%2 == 0); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}

// Count is how many rows the table holds.
func (s *Store) Count() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int
	err := s.db.QueryRow("SELECT COUNT(*) FROM rows").Scan(&n)
	return n, err
}

// Page reads one screenful, in id order. Offset past the end is not an error:
// a list scrolled beyond its data is empty, not broken.
func (s *Store) Page(offset, limit int) ([]Row, error) {
	if offset < 0 || limit <= 0 {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out, err := s.db.Query("SELECT id, title, done FROM rows ORDER BY id LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		return nil, err
	}
	defer out.Close()

	var rows []Row
	for out.Next() {
		var (
			r    Row
			done int
		)
		if err := out.Scan(&r.ID, &r.Title, &done); err != nil {
			return nil, err
		}
		r.Done = done != 0
		rows = append(rows, r)
	}
	return rows, out.Err()
}

// Reset empties the table, keeping the file.
func (s *Store) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec("DELETE FROM rows")
	return err
}

// Close closes the file. The app calls it when it is done with the database;
// the demo lets the process exit instead, and SQLite's WAL is recovered on the
// next open either way.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}
