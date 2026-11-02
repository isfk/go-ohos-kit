package core

import (
	"bytes"
	"strings"
	"testing"
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
