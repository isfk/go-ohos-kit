// Package sqlitebench measures two ways of reaching SQLite from Go on the work
// this repo actually does: a bulk import, a 2000-row page, point lookups, an
// aggregate. It answers "what does the driver cost?" — not "is SQLite fast?",
// since the engine underneath is the same in both cases.
//
// It is a separate module (its own go.mod) on purpose: the app's go.mod stays
// dependency-free and go/core stays cgo-free (AGENTS.md, rule 1). Delete the
// directory and nothing else notices.
//
//	cd go/bench/sqlite && go test -bench . -benchtime 3x
//
// What this cannot tell you: whether mattn/go-sqlite3 cross-compiles for
// OpenHarmony's musl target. That is scripts/check.sh's job, and the one
// question a Mac cannot stand in for.
package sqlitebench

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	_ "modernc.org/sqlite"
)

// The two drivers. Both speak database/sql, so what differs below is the
// driver and nothing else: same queries, same rows, same pragmas.
var drivers = []struct {
	name string
	dsn  string
}{
	{"mattn", "sqlite3"},  // cgo, the C library itself
	{"modernc", "sqlite"}, // pure Go translation of the same library
}

const (
	rowsCount = 20000 // the demo's big list, so the numbers line up with the README
	page      = 2000  // one page of it
)

const schema = `CREATE TABLE rows (
	id    INTEGER PRIMARY KEY,
	title TEXT    NOT NULL,
	done  INTEGER NOT NULL
)`

const insertSQL = "INSERT INTO rows (id, title, done) VALUES (?, ?, ?)"

var (
	mu   sync.Mutex
	dbs  = map[string]*sql.DB{}
	dirs = map[string]string{}
)

func TestMain(m *testing.M) {
	code := m.Run()
	for _, db := range dbs {
		db.Close()
	}
	for _, dir := range dirs {
		os.RemoveAll(dir)
	}
	os.Exit(code)
}

// newDB opens a database with the pragmas a real app wants: WAL so a reader
// never waits for the writer, and synchronous=NORMAL so a commit doesn't wait
// for the platter. Without them every commit pays an fsync, and the driver
// comparison drowns in disk noise instead of measuring the driver.
func newDB(tb testing.TB, driver, path string) *sql.DB {
	tb.Helper()
	db, err := sql.Open(driver, path)
	if err != nil {
		tb.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		tb.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		tb.Fatal(err)
	}
	if _, err := db.Exec(schema); err != nil {
		tb.Fatal(err)
	}
	// SQLite serialises writers anyway; one connection keeps the measurement
	// about the driver rather than about pool contention.
	db.SetMaxOpenConns(1)
	return db
}

// fixture is a database holding rowsCount rows: built once per driver, shared
// by every read benchmark, cleaned up by TestMain.
func fixture(tb testing.TB, driver string) *sql.DB {
	tb.Helper()
	mu.Lock()
	defer mu.Unlock()
	if db, ok := dbs[driver]; ok {
		return db
	}
	dir, err := os.MkdirTemp("", "sqlitebench-fixture-"+driver)
	if err != nil {
		tb.Fatal(err)
	}
	db := newDB(tb, driver, filepath.Join(dir, "rows.db"))
	tx, err := db.Begin()
	if err != nil {
		tb.Fatal(err)
	}
	stmt, err := tx.Prepare(insertSQL)
	if err != nil {
		tb.Fatal(err)
	}
	for r := range rowsCount {
		// The title is built the same way for both drivers, so its cost
		// cancels out of the comparison.
		title := fmt.Sprintf("row %d", r)
		if _, err := stmt.Exec(r, title, r%2 == 0); err != nil {
			tb.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		tb.Fatal(err)
	}
	dbs[driver] = db
	dirs[driver] = dir
	return db
}

// scratch is an empty database in its own file: a write benchmark needs a
// clean one, and its cost is not part of what's being measured.
func scratch(tb testing.TB, driver string) *sql.DB {
	tb.Helper()
	dir, err := os.MkdirTemp("", "sqlitebench-scratch-"+driver)
	if err != nil {
		tb.Fatal(err)
	}
	db := newDB(tb, driver, filepath.Join(dir, "rows.db"))
	tb.Cleanup(func() {
		db.Close()
		os.RemoveAll(dir)
	})
	return db
}

// BenchmarkInsertTx is the right way to write 20k rows: one transaction, one
// prepared statement. The whole import is one commit, so WAL does one fsync.
func BenchmarkInsertTx(b *testing.B) {
	for _, d := range drivers {
		b.Run(d.name, func(b *testing.B) {
			b.ReportAllocs()
			var total time.Duration
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				db := scratch(b, d.dsn)
				tx, err := db.Begin()
				if err != nil {
					b.Fatal(err)
				}
				stmt, err := tx.Prepare(insertSQL)
				if err != nil {
					b.Fatal(err)
				}
				b.StartTimer()

				start := time.Now()
				for r := range rowsCount {
					// The title is built the same way for both drivers, so its
					// cost cancels out of the comparison.
					title := fmt.Sprintf("row %d", r)
					if _, err := stmt.Exec(r, title, r%2 == 0); err != nil {
						b.Fatal(err)
					}
				}
				commitErr := tx.Commit()
				total += time.Since(start)
				if commitErr != nil {
					b.Fatal(commitErr)
				}
			}
			b.ReportMetric(float64(total.Nanoseconds())/float64(b.N*rowsCount), "ns/row")
		})
	}
}

// BenchmarkInsertNaive is the same 20k rows written the way they usually get
// written: no explicit transaction, so every row is its own commit (and, with
// database/sql, its own prepare). It is here to show which number moves first
// — "in a transaction or not" — while the driver stays the other variable.
//
// The row values are bound as parameters rather than pasted into the SQL: a
// benchmark has no business demonstrating the string-concatenation habit this
// package exists to argue against, and building the SQL is identical work for
// both drivers, so it would measure Sprintf either way.
func BenchmarkInsertNaive(b *testing.B) {
	for _, d := range drivers {
		b.Run(d.name, func(b *testing.B) {
			b.ReportAllocs()
			var total time.Duration
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				db := scratch(b, d.dsn)
				b.StartTimer()

				start := time.Now()
				for r := range rowsCount {
					title := fmt.Sprintf("row %d", r)
					if _, err := db.Exec(insertSQL, r, title, r%2 == 0); err != nil {
						b.Fatal(err)
					}
				}
				total += time.Since(start)
			}
			b.ReportMetric(float64(total.Nanoseconds())/float64(b.N*rowsCount), "ns/row")
		})
	}
}

// BenchmarkPage is one screenful: 2000 rows out, every column scanned into Go
// values. This is what the bridge would have to carry across to ArkTS.
func BenchmarkPage(b *testing.B) {
	for _, d := range drivers {
		b.Run(d.name, func(b *testing.B) {
			db := fixture(b, d.dsn)
			stmt, err := db.Prepare("SELECT id, title, done FROM rows ORDER BY id LIMIT ?")
			if err != nil {
				b.Fatal(err)
			}
			defer stmt.Close()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if n, err := scanPage(stmt, page); err != nil {
					b.Fatal(err)
				} else if n != page {
					b.Fatalf("got %d rows, want %d", n, page)
				}
			}
			b.ReportMetric(page, "rows/op")
		})
	}
}

func scanPage(stmt *sql.Stmt, limit int) (int, error) {
	out, err := stmt.Query(limit)
	if err != nil {
		return 0, err
	}
	defer out.Close()
	var (
		n     int
		id    int
		title string
		done  int
	)
	for out.Next() {
		if err := out.Scan(&id, &title, &done); err != nil {
			return n, err
		}
		n++
	}
	return n, out.Err()
}

// BenchmarkPointLookup is a detail screen: one row by primary key.
func BenchmarkPointLookup(b *testing.B) {
	for _, d := range drivers {
		b.Run(d.name, func(b *testing.B) {
			db := fixture(b, d.dsn)
			stmt, err := db.Prepare("SELECT title FROM rows WHERE id = ?")
			if err != nil {
				b.Fatal(err)
			}
			defer stmt.Close()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var title string
				if err := stmt.QueryRow(i % rowsCount).Scan(&title); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkAggregate is the query where the work happens inside SQLite rather
// than on the way out: if the driver were the bottleneck, this would be cheap
// for both and identical.
func BenchmarkAggregate(b *testing.B) {
	for _, d := range drivers {
		b.Run(d.name, func(b *testing.B) {
			db := fixture(b, d.dsn)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var (
					n   int
					sum sql.NullInt64
				)
				if err := db.QueryRow("SELECT COUNT(*), SUM(id) FROM rows").Scan(&n, &sum); err != nil {
					b.Fatal(err)
				}
				if n != rowsCount {
					b.Fatalf("count %d, want %d", n, rowsCount)
				}
			}
		})
	}
}
