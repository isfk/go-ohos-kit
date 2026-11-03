package store

import (
	"path/filepath"
	"testing"
)

// open puts a database in the test's own temporary directory: the app's
// sandbox path is the one thing this package can't invent, so a test supplies
// one that disappears with the test.
func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "rows.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return s
}

func TestSeedThenCount(t *testing.T) {
	s := open(t)
	if n, err := s.Seed(100); err != nil || n != 100 {
		t.Fatalf("Seed(100) = %d, %v", n, err)
	}
	n, err := s.Count()
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 100 {
		t.Errorf("Count = %d, want 100", n)
	}
}

// Seeding twice replaces the table rather than doubling it: the demo's button
// says "seed 20k", and pressing it twice must not leave 40k rows behind.
func TestSeedReplaces(t *testing.T) {
	s := open(t)
	if _, err := s.Seed(50); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if _, err := s.Seed(10); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	n, err := s.Count()
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 10 {
		t.Errorf("Count = %d after reseeding with 10, want 10", n)
	}
}

func TestPage(t *testing.T) {
	s := open(t)
	if _, err := s.Seed(25); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	page, err := s.Page(20, 10)
	if err != nil {
		t.Fatalf("Page: %v", err)
	}
	// 25 rows, asking from 20 for 10: five left, in order, with the fields the
	// core stores coming back the way it stored them.
	if len(page) != 5 {
		t.Fatalf("Page(20, 10) = %d rows, want the 5 that are left", len(page))
	}
	if page[0].ID != 20 || page[4].ID != 24 {
		t.Errorf("ids = %d..%d, want 20..24", page[0].ID, page[4].ID)
	}
	if page[0].Title != "row 20" {
		t.Errorf("Title = %q, want \"row 20\"", page[0].Title)
	}
	if !page[0].Done || page[1].Done {
		t.Errorf("Done = %v, %v, want alternating from even = done", page[0].Done, page[1].Done)
	}
}

// A list scrolled past its data is empty, not broken — and asking for nothing
// is not an error either.
func TestPageEdges(t *testing.T) {
	s := open(t)
	if _, err := s.Seed(3); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if page, err := s.Page(50, 10); err != nil || len(page) != 0 {
		t.Errorf("Page past the end = %d rows, %v; want none and no error", len(page), err)
	}
	if page, err := s.Page(0, 0); err != nil || page != nil {
		t.Errorf("Page(0, 0) = %v, %v; want nothing", page, err)
	}
	if page, err := s.Page(-1, 10); err != nil || page != nil {
		t.Errorf("Page(-1, 10) = %v, %v; want nothing", page, err)
	}
}

// The file outlives the process: that is the whole point of a database, and
// it's what the on-disk WAL has to get right.
func TestSurvivesReopen(t *testing.T) {
	s := open(t)
	path := s.Path()
	if _, err := s.Seed(7); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	again, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer again.Close()
	n, err := again.Count()
	if err != nil {
		t.Fatalf("Count after reopen: %v", err)
	}
	if n != 7 {
		t.Errorf("Count = %d after reopen, want 7", n)
	}
}

func TestReset(t *testing.T) {
	s := open(t)
	if _, err := s.Seed(9); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if err := s.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if n, err := s.Count(); err != nil || n != 0 {
		t.Errorf("Count after Reset = %d, %v; want 0", n, err)
	}
}

// No path means no database: this is the error a caller gets when it forgets
// to pass the sandbox directory through.
func TestOpenNeedsAPath(t *testing.T) {
	if _, err := Open(""); err == nil {
		t.Error("Open(\"\") succeeded, want an error")
	}
	if _, err := Open(filepath.Join(t.TempDir(), "no such dir", "rows.db")); err == nil {
		t.Error("Open in a missing directory succeeded, want an error")
	}
}
