package core

import (
	"bytes"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAddToggleRemove(t *testing.T) {
	s := New()
	var events []string
	s.Listen(func(json string) { events = append(events, json) })

	if got := s.Add("buy milk"); !strings.Contains(got, "buy milk") {
		t.Fatalf("add: %s", got)
	}
	s.Toggle(1)
	if s.Remaining() != 0 {
		t.Fatalf("remaining after toggle: %d", s.Remaining())
	}
	s.Remove(1)
	if len(s.todos) != 0 {
		t.Fatalf("after remove: %v", s.todos)
	}
	// Every change pushes an event: that's how the UI hears about it.
	if len(events) != 3 {
		t.Fatalf("events: %v", events)
	}
}

func TestAddEmptyIsNothing(t *testing.T) {
	s := New()
	s.Add("")
	if len(s.todos) != 0 {
		t.Fatal("an empty title made a todo")
	}
}

func TestInspectFDDoesNotCloseHostDescriptor(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "photo")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.Write([]byte("host still owns this fd")); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	if _, err := InspectFD(int(file.Fd()), "photo"); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatalf("host descriptor was closed: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := file.Read(buf); err != nil || string(buf) != "host" {
		t.Fatalf("host descriptor read %q, %v", buf, err)
	}
}

func TestInspectNamesFormats(t *testing.T) {
	jpeg := append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{0}, 60)...)
	png := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}, bytes.Repeat([]byte{0}, 60)...)
	heif := append([]byte{0, 0, 0, 0x18, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'c'}, bytes.Repeat([]byte{0}, 60)...)

	for _, c := range []struct {
		data  []byte
		kind  string
		bytes int64
	}{
		{jpeg, "image/jpeg", 64},
		{png, "image/png", 68},
		{heif, "image/heif", 72},
		{[]byte("hello"), "application/octet-stream", 5},
	} {
		info, err := Inspect(bytes.NewReader(c.data), "a")
		if err != nil {
			t.Fatal(err)
		}
		if info.Kind != c.kind || info.Bytes != c.bytes || len(info.SHA256) != 64 {
			t.Fatalf("%s: %+v", c.kind, info)
		}
	}
}

func TestEncodeRows(t *testing.T) {
	s := New()
	rows := s.Rows(10, 3)
	if len(rows) != 3 || rows[0].ID != 11 {
		t.Fatalf("rows: %+v", rows)
	}
	b := EncodeRows(rows)
	// u32 count + 3 × (u32 id + u8 done + u16 len + title)
	if len(b) < 4+3*(4+1+2) {
		t.Fatalf("encoded %d bytes", len(b))
	}
	if got := int(b[0]) | int(b[1])<<8 | int(b[2])<<16 | int(b[3])<<24; got != 3 {
		t.Fatalf("count: %d", got)
	}
	// The last row's title reads back where the header says it is.
	off := 4
	for i := 0; i < len(rows); i++ {
		titleLen := int(b[off+5]) | int(b[off+6])<<8
		title := string(b[off+7 : off+7+titleLen])
		if title != rows[i].Title {
			t.Fatalf("row %d title %q, want %q", i, title, rows[i].Title)
		}
		off += 7 + titleLen
	}
	if off != len(b) {
		t.Fatalf("trailing bytes: %d of %d", off, len(b))
	}
}

func TestRowsPageIsBounded(t *testing.T) {
	s := New()
	if got := len(s.Rows(19990, 100)); got != 10 {
		t.Fatalf("last page: %d rows", got)
	}
	if got := len(s.Rows(0, 1000000)); got != 20000 {
		t.Fatalf("whole list: %d rows", got)
	}
	if got := s.Rows(20000, 10); got != nil {
		t.Fatalf("past the end: %v", got)
	}
}

// fakePersister records what the core asked it to keep, and hands back whatever
// the test put in it — the seam without a database or a file.
type fakePersister struct {
	loaded []Todo
	saved  [][]Todo
}

func (f *fakePersister) Load() ([]Todo, error) { return f.loaded, nil }

func (f *fakePersister) Save(todos []Todo) error {
	f.saved = append(f.saved, append([]Todo(nil), todos...))
	return nil
}

// blockingPersister pauses the first write, exposing out-of-order persistence.
type blockingPersister struct {
	mu      sync.Mutex
	entered chan struct{}
	release chan struct{}
	writes  [][]Todo
}

func (p *blockingPersister) Load() ([]Todo, error) { return nil, nil }
func (p *blockingPersister) Save(todos []Todo) error {
	p.mu.Lock()
	first := len(p.writes) == 0
	p.mu.Unlock()
	if first {
		select {
		case <-p.entered:
		default:
			close(p.entered)
		}
		<-p.release
	}
	p.mu.Lock()
	p.writes = append(p.writes, append([]Todo(nil), todos...))
	p.mu.Unlock()
	return nil
}

func TestConcurrentMutationsPersistInOrder(t *testing.T) {
	p := &blockingPersister{entered: make(chan struct{}), release: make(chan struct{})}
	s := New()
	s.SetPersister(p)
	first := make(chan struct{})
	go func() { s.Add("first"); close(first) }()
	select {
	case <-p.entered:
	case <-time.After(2 * time.Second):
		close(p.release)
		t.Fatal("first save never started")
	}
	second := make(chan struct{})
	go func() { s.Add("second"); close(second) }()
	// The second mutation must wait for the first save to finish.
	select {
	case <-second:
		t.Error("second mutation finished before the first save")
	case <-time.After(50 * time.Millisecond):
	}
	close(p.release)
	select {
	case <-first:
	case <-time.After(2 * time.Second):
		t.Fatal("first mutation hung")
	}
	select {
	case <-second:
	case <-time.After(2 * time.Second):
		t.Fatal("second mutation hung")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.writes) != 2 || len(p.writes[0]) != 1 || len(p.writes[1]) != 2 || p.writes[1][1].Title != "second" {
		t.Fatalf("out-of-order writes: %+v", p.writes)
	}
}

func TestTheListComesFromAndGoesToThePersister(t *testing.T) {
	p := &fakePersister{loaded: []Todo{
		{ID: 1, Title: "from last time", Done: true},
		{ID: 7, Title: "and this one", Done: false},
	}}
	s := New()
	s.Listen(func(string) {})
	s.SetPersister(p)
	s.Start()
	defer s.Stop()

	// Start reads: what the persister had is what the UI is shown.
	if len(s.todos) != 2 || s.todos[0].Title != "from last time" {
		t.Fatalf("loaded %v", s.todos)
	}
	// And the next id carries on from the highest one seen, not from the count:
	// a list that was deleted from must not reuse an id.
	if got := s.Add("new"); !strings.Contains(got, "\"id\":8") {
		t.Fatalf("new todo got the wrong id: %s", got)
	}
	if len(p.saved) != 1 || len(p.saved[0]) != 3 {
		t.Fatalf("saved %d times: %v", len(p.saved), p.saved)
	}

	s.Toggle(8)
	if len(p.saved) != 2 || !p.saved[1][2].Done {
		t.Fatalf("toggle did not reach the persister: %v", p.saved)
	}
}

// No persister is a legitimate state: the tests above run that way, and so does a
// launch where the database could not be opened.
func TestNoPersisterIsNotACrash(t *testing.T) {
	s := New()
	s.Listen(func(string) {})
	s.Start()
	defer s.Stop()
	s.Add("still works")
	if s.Remaining() != 1 {
		t.Fatalf("remaining: %d", s.Remaining())
	}
}

// The file persister is what the first version wrote, and what the app imports
// once. A missing file reads as an empty list, not an error: a first launch is
// not a failure.
func TestFilePersisterRoundTrip(t *testing.T) {
	path := t.TempDir() + "/todos.json"
	p := FilePersister{Path: path}
	if got, err := p.Load(); err != nil || got != nil {
		t.Fatalf("missing file: %v, %v", got, err)
	}
	want := []Todo{{ID: 1, Title: "kept", Done: true}}
	if err := p.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := p.Load()
	if err != nil || len(got) != 1 || got[0].Title != "kept" || !got[0].Done {
		t.Fatalf("Load: %v, %v", got, err)
	}
}
