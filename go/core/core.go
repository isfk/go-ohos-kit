// Package core is the app's logic: what the UI shows, and what it can do.
// It's plain Go — no cgo, no HarmonyOS — so it's built and tested on a
// developer's machine, and the same code runs in the app (see ../main.go).
package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Todo is one row of the list.
type Todo struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

// Event is what the core tells the UI: a new list, or a line of text (the
// uptime this demo pushes every second). The UI reads Kind and what goes
// with it.
type Event struct {
	Kind  string        `json:"kind"` // "todos", "text", "level"
	Todos []Todo        `json:"todos,omitempty"`
	Text  string        `json:"text,omitempty"`
	Level *LevelReading `json:"level,omitempty"`
}

// FileInfo is what the core makes of a file the UI hands it: a photo from
// the system picker, say. Kind is read from the file's first bytes, not its
// name — the UI's word for a format isn't evidence.
type FileInfo struct {
	Name   string `json:"name"`
	Bytes  int64  `json:"bytes"`
	Kind   string `json:"kind"`
	SHA256 string `json:"sha256"`
}

// Inspect reads a file and reports what it is: its size, its format (from its
// magic bytes), and its SHA-256. Plain io.Reader: the bridge hands it a file
// descriptor the system opened, and a test hands it a buffer.
func Inspect(r io.Reader, name string) (FileInfo, error) {
	h := sha256.New()
	buf := make([]byte, 64<<10)
	var head []byte
	var n int64
	for {
		got, err := r.Read(buf)
		if got > 0 {
			if len(head) < 32 {
				head = append(head, buf[:min(got, 32-len(head))]...)
			}
			h.Write(buf[:got])
			n += int64(got)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return FileInfo{}, err
		}
	}
	return FileInfo{
		Name:   name,
		Bytes:  n,
		Kind:   kindOf(head),
		SHA256: hex.EncodeToString(h.Sum(nil)),
	}, nil
}

// kindOf names a format from its first bytes.
func kindOf(head []byte) string {
	switch {
	case len(head) >= 3 && head[0] == 0xFF && head[1] == 0xD8 && head[2] == 0xFF:
		return "image/jpeg"
	case len(head) >= 8 && string(head[:8]) == "\x89PNG\r\n\x1a\n":
		return "image/png"
	case len(head) >= 12 && string(head[4:8]) == "ftyp":
		// HEIF/HEIC, what HarmonyOS cameras save by default.
		return "image/heif"
	case len(head) >= 6 && (string(head[:6]) == "GIF87a" || string(head[:6]) == "GIF89a"):
		return "image/gif"
	case len(head) >= 4 && string(head[:4]) == "%PDF":
		return "application/pdf"
	case len(head) >= 4 && string(head[:4]) == "PK\x03\x04":
		return "application/zip"
	}
	return "application/octet-stream"
}

// Row is one row of a big list: what a scrolling list shows, thousands of
// them. The same shape as Todo, on purpose: the UI can reuse its rendering.
type Row struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

// rowCount is how many rows the demo's big list has: enough that a JSON wire
// hurts, small enough to stay honest on a phone.
const rowCount = 20000

// RowCount is how many rows there are.
func (s *Store) RowCount() int { return rowCount }

// Rows is a page of the big list. Made on demand: a page of 200 costs what
// 200 rows cost, whether the list is 20 or 20,000.
func (s *Store) Rows(offset, limit int) []Row {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || offset >= rowCount {
		return nil
	}
	if offset+limit > rowCount {
		limit = rowCount - offset
	}
	rows := make([]Row, limit)
	for i := range rows {
		n := offset + i
		rows[i] = Row{
			ID:    n + 1,
			Title: "Row " + itoa(n+1) + " · the quick brown fox jumps over the lazy dog",
			Done:  n%7 == 0,
		}
	}
	return rows
}

// EncodeRows writes rows in the compact form the UI reads: u32 count, then per
// row u32 id, u8 done, u16 title length, title bytes. It's a tenth of JSON's
// bytes and needs no parsing, which is the point of having it.
func EncodeRows(rows []Row) []byte {
	size := 4
	for _, r := range rows {
		size += 4 + 1 + 2 + len(r.Title)
	}
	b := make([]byte, 0, size)
	b = appendU32(b, uint32(len(rows)))
	for _, r := range rows {
		b = appendU32(b, uint32(r.ID))
		if r.Done {
			b = append(b, 1)
		} else {
			b = append(b, 0)
		}
		b = append(b, byte(len(r.Title)), byte(len(r.Title)>>8))
		b = append(b, r.Title...)
	}
	return b
}

func appendU32(b []byte, v uint32) []byte {
	return append(b, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d [20]byte
	i := len(d)
	for n > 0 {
		i--
		d[i] = byte('0' + n%10)
		n /= 10
	}
	return string(d[i:])
}

// Store is the app's state. Every method is safe from any goroutine: the UI
// calls it on its main thread, and the heartbeat calls it from its own.
type Store struct {
	mu    sync.Mutex
	todos []Todo
	next  int
	dir   string        // where the list is kept; empty: nowhere
	emit  func(string)  // tells the UI; nil until the host sets one
	stop  chan struct{} // the heartbeat's
	fg    bool          // in the foreground (the heartbeat only ticks then)
	level *Level        // the accelerometer's, made on first use
	// levelPush is when the level last went to the UI: the sensor calls 50
	// times a second, the screen refreshes 60, and the bridge doesn't need
	// every sample.
	lastLevelPush time.Time
}

// New returns an empty store.
func New() *Store { return &Store{} }

// Listen sets where events go (a JSON string per event). It's called by the
// host's bridge; the UI only sees them.
func (s *Store) Listen(emit func(json string)) {
	s.mu.Lock()
	s.emit = emit
	s.mu.Unlock()
}

// SetDir says where the list is kept (the app's files directory).
func (s *Store) SetDir(dir string) {
	s.mu.Lock()
	s.dir = dir
	s.mu.Unlock()
}

// load reads the list from disk, if there is one.
func (s *Store) load() {
	s.mu.Lock()
	path := s.path()
	s.mu.Unlock()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var todos []Todo
	if json.Unmarshal(data, &todos) != nil {
		return
	}
	s.mu.Lock()
	s.todos = todos
	for _, t := range todos {
		if t.ID >= s.next {
			s.next = t.ID + 1
		}
	}
	s.mu.Unlock()
}

// save writes the list, best effort: a demo of the app's own files.
func (s *Store) save() {
	s.mu.Lock()
	path := s.path()
	data, err := json.Marshal(s.todos)
	s.mu.Unlock()
	if path == "" || err != nil {
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, 0o600) == nil {
		os.Rename(tmp, path)
	}
}

// path is the list's file, locked.
func (s *Store) path() string {
	if s.dir == "" {
		return ""
	}
	return filepath.Join(s.dir, "todos.json")
}

// Snapshot is the list, as the UI takes it.
func (s *Store) Snapshot() string {
	s.mu.Lock()
	todos := append([]Todo(nil), s.todos...)
	s.mu.Unlock()
	return encode(Event{Kind: "todos", Todos: todos})
}

// Add makes a todo. It returns the list, as every change does.
func (s *Store) Add(title string) string {
	if title == "" {
		return s.Snapshot()
	}
	s.mu.Lock()
	s.next++
	s.todos = append(s.todos, Todo{ID: s.next, Title: title})
	s.mu.Unlock()
	s.save()
	s.changed()
	return s.Snapshot()
}

// Toggle turns a todo done, or not.
func (s *Store) Toggle(id int) string {
	s.mu.Lock()
	for i := range s.todos {
		if s.todos[i].ID == id {
			s.todos[i].Done = !s.todos[i].Done
			break
		}
	}
	s.mu.Unlock()
	s.save()
	s.changed()
	return s.Snapshot()
}

// Remove drops a todo.
func (s *Store) Remove(id int) string {
	s.mu.Lock()
	for i := range s.todos {
		if s.todos[i].ID == id {
			s.todos = append(s.todos[:i], s.todos[i+1:]...)
			break
		}
	}
	s.mu.Unlock()
	s.save()
	s.changed()
	return s.Snapshot()
}

// Remaining is how many are left to do.
func (s *Store) Remaining() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, t := range s.todos {
		if !t.Done {
			n++
		}
	}
	return n
}

// Accel takes one accelerometer sample (m/s²). The UI calls this from the
// sensor's callback — 50 times a second — so it does the least it can: smooth,
// and hand the result on at most 30 times a second.
func (s *Store) Accel(x, y, z float64) {
	s.mu.Lock()
	if s.level == nil {
		s.level = NewLevel(nil)
	}
	level := s.level
	s.mu.Unlock()

	level.mu.Lock()
	if !level.primed {
		level.x, level.y, level.z = x, y, z
		level.primed = true
	} else {
		level.x += level.alpha * (x - level.x)
		level.y += level.alpha * (y - level.y)
		level.z += level.alpha * (z - level.z)
	}
	level.samples++
	elapsed := time.Since(level.since).Seconds()
	if elapsed >= 1 {
		level.perSec = float64(level.samples) / elapsed
		level.samples = 0
		level.since = time.Now()
	}
	reading := LevelReading{Pitch: level.pitch(), Roll: level.roll(), Rate: level.perSec,
		X: level.x, Y: level.y, Z: level.z}
	level.mu.Unlock()

	s.mu.Lock()
	due := time.Since(s.lastLevelPush) >= 33*time.Millisecond
	if due {
		s.lastLevelPush = time.Now()
	}
	s.mu.Unlock()
	if due {
		s.push(Event{Kind: "level", Level: &reading})
	}
}

// SetForeground tells the core whether the app is on screen: a backgrounded
// app does no work it doesn't have to.
func (s *Store) SetForeground(on bool) {
	s.mu.Lock()
	s.fg = on
	s.mu.Unlock()
}

// Start reads the list and starts the heartbeat: a goroutine of the core's
// own, which the UI hears about through Listen.
func (s *Store) Start() {
	s.load()
	s.changed()
	s.mu.Lock()
	if s.stop != nil {
		s.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	s.stop = stop
	s.fg = true
	s.mu.Unlock()
	go func() {
		start := time.Now()
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				s.mu.Lock()
				fg := s.fg
				s.mu.Unlock()
				if !fg {
					continue // backgrounded: the UI can't show it anyway
				}
				// A goroutine pushing to the UI: the bridge gets this to
				// the main thread (see napi_linux.c).
				s.push(Event{Kind: "text", Text: "up " + time.Since(start).Truncate(time.Second).String()})
			}
		}
	}()
}

// Stop ends the heartbeat.
func (s *Store) Stop() {
	s.mu.Lock()
	stop := s.stop
	s.stop = nil
	s.mu.Unlock()
	if stop != nil {
		close(stop)
	}
}

// changed tells the UI the list is new.
func (s *Store) changed() {
	s.mu.Lock()
	todos := append([]Todo(nil), s.todos...)
	s.mu.Unlock()
	s.push(Event{Kind: "todos", Todos: todos})
}

// push sends an event to the UI, if there's one listening.
func (s *Store) push(e Event) {
	s.mu.Lock()
	emit := s.emit
	s.mu.Unlock()
	if emit != nil {
		emit(encode(e))
	}
}

func encode(e Event) string {
	data, err := json.Marshal(e)
	if err != nil {
		return `{"kind":"text","text":"encode error"}`
	}
	return string(data)
}
