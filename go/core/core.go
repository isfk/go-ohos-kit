// Package core is the app's logic: what the UI shows, and what it can do.
// It's plain Go — no cgo, no HarmonyOS — so it's built and tested on a
// developer's machine, and the same code runs in the app (see ../main.go).
package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"os"
	"sync"
	"syscall"
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

// InspectFD reads a file the system opened for the UI (a photo from the picker,
// say) and reports what it is.
//
// Two things about that descriptor, and both are load-bearing. It is closed here,
// which is why it is duped first: os.NewFile + Close would otherwise take the
// caller's descriptor with it. And its offset is *not* moved, which is why the
// reads go through a SectionReader: ArkTS hands the same descriptor to the image
// decoder a line after it asks us what the file is, and a dup does not help there
// — it is a second handle on one open file description, so reading through it
// walks the original to EOF too.
func InspectFD(fd int, name string) (FileInfo, error) {
	ownedFD, err := syscall.Dup(fd)
	if err != nil {
		return FileInfo{}, err
	}
	f := os.NewFile(uintptr(ownedFD), name)
	if f == nil {
		_ = syscall.Close(ownedFD)
		return FileInfo{}, os.ErrInvalid
	}
	defer f.Close()
	// SectionReader reads with ReadAt (pread): the hash covers the whole file even
	// if the caller had already read part of it, and the caller's offset is
	// exactly where it left it.
	return Inspect(io.NewSectionReader(f, 0, math.MaxInt64), name)
}

// Inspect is the arithmetic above, over any io.Reader: a descriptor the system
// opened, or a buffer in a test.

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

// Persister is where the list lives. The core decides what a list is and when it
// changes; the host decides where it goes — a JSON file in the first version, the
// app's SQLite now (see ../store). Keeping that behind one interface is what lets
// this package stay pure Go and testable without a phone.
type Persister interface {
	Load() ([]Todo, error)
	Save([]Todo) error
}

// FilePersister keeps the list in a JSON file. It is still here for two reasons:
// the first version of the app wrote it, so the first launch on a device imports
// it once; and it is the simplest possible Persister to test against.
type FilePersister struct{ Path string }

// Load reads the file, and treats "no file yet" as an empty list rather than an
// error: a first launch is not a failure.
func (p FilePersister) Load() ([]Todo, error) {
	data, err := os.ReadFile(p.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var todos []Todo
	if err := json.Unmarshal(data, &todos); err != nil {
		return nil, err
	}
	return todos, nil
}

// Save writes the whole list through a temporary file, so a half-written list is
// never what a reader finds.
func (p FilePersister) Save(todos []Todo) error {
	data, err := json.Marshal(todos)
	if err != nil {
		return err
	}
	tmp := p.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p.Path)
}

// Store is the app's state. Every method is safe from any goroutine: the UI
// calls it on its main thread, and the heartbeat calls it from its own.
type Store struct {
	mutationMu sync.Mutex
	mu         sync.Mutex
	todos      []Todo
	next       int
	persist    Persister     // where the list goes; nil: nowhere
	emit       func(string)  // tells the UI; nil until the host sets one
	stop       chan struct{} // the heartbeat's
	fg         bool          // in the foreground (the heartbeat only ticks then)
	level      *Level        // the accelerometer's, made on first use
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

// SetPersister says where the list is kept. Call it before Start: Start reads.
func (s *Store) SetPersister(p Persister) {
	s.mu.Lock()
	s.persist = p
	s.mu.Unlock()
}

// load reads the list from wherever the host keeps it, if anywhere.
func (s *Store) load() {
	s.mu.Lock()
	p := s.persist
	s.mu.Unlock()
	if p == nil {
		return
	}
	todos, err := p.Load()
	if err != nil || todos == nil {
		return
	}
	s.mu.Lock()
	s.todos = todos
	// next is the last id handed out, not the next one: Add increments before it
	// uses it. Setting it to the highest id here keeps a restart from skipping a
	// number — the first version of this set it to +1 and lost one per launch.
	for _, t := range todos {
		if t.ID > s.next {
			s.next = t.ID
		}
	}
	s.mu.Unlock()
}

// save writes the list, best effort: the UI has already been told, and a failed
// write is not something it could act on anyway. Same bargain the file version
// made; the difference is that SQLite has far fewer ways to fail.
func (s *Store) save() {
	s.mu.Lock()
	p := s.persist
	list := append([]Todo(nil), s.todos...)
	s.mu.Unlock()
	if p == nil {
		return
	}
	_ = p.Save(list)
}

// Snapshot is the list, as the UI takes it.
func (s *Store) Snapshot() string {
	s.mu.Lock()
	todos := append([]Todo(nil), s.todos...)
	s.mu.Unlock()
	return todosJSON(todos)
}

// todosJSON is the list event, and the only place the list's wire shape is
// written down.
//
// `Todos` has no `omitempty`, unlike Event's: an empty list has to travel as
// `"todos":[]` rather than as a missing field. The UI treats a mutation reply
// without a list as a failed save — that is what the error object looks like — so
// dropping the field turned "deleted my last row" into an error banner on the
// device (found by the ArkTS shell both repos now share; the Rust core always
// sent the empty array, so the two kernels now agree on the wire).
func todosJSON(todos []Todo) string {
	// A nil slice marshals to `null`, and `null` is not a list: normalise first,
	// so the field is an array whether it is empty or not.
	if todos == nil {
		todos = []Todo{}
	}
	data, err := json.Marshal(struct {
		Kind  string `json:"kind"`
		Todos []Todo `json:"todos"`
	}{"todos", todos})
	if err != nil {
		return `{"kind":"text","text":"encode error"}`
	}
	return string(data)
}

// Add makes a todo. It returns the list, as every change does.
func (s *Store) Add(title string) string {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
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
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
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
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
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
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
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
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
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
	s.emitJSON(todosJSON(todos))
}

// push sends an event to the UI, if there's one listening.
func (s *Store) push(e Event) {
	s.emitJSON(encode(e))
}

// emitJSON hands an already-encoded event to the UI.
func (s *Store) emitJSON(json string) {
	s.mu.Lock()
	emit := s.emit
	s.mu.Unlock()
	if emit != nil {
		emit(json)
	}
}

func encode(e Event) string {
	data, err := json.Marshal(e)
	if err != nil {
		return `{"kind":"text","text":"encode error"}`
	}
	return string(data)
}
