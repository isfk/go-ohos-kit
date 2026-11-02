//go:build linux && cgo

// The core's side of the bridge: what ArkTS calls, as C functions the NAPI
// module (napi_linux.c) hands to JavaScript, and nothing more. The logic
// itself is in gokit/core, which runs anywhere.
package main

/*
#include <stdint.h>
#include <stdlib.h>

// gokit_emit hands a JSON event to the UI: napi_linux.c copies it and gets it
// to the main thread. It takes the string's ownership (frees it).
void gokit_emit(const char *json);
*/
import "C"

import (
	"encoding/json"
	"os"
	"unsafe"

	"gokit/core"
)

var store = core.New()

func init() {
	// Every event the core makes goes to the UI through the bridge.
	store.Listen(func(json string) {
		C.gokit_emit(C.CString(json))
	})
}

//export gokit_start
func gokit_start(filesDir *C.char) {
	if dir := C.GoString(filesDir); dir != "" {
		store.SetDir(dir)
	}
	store.Start()
}

//export gokit_stop
func gokit_stop() { store.Stop() }

//export gokit_snapshot
func gokit_snapshot() *C.char { return C.CString(store.Snapshot()) }

//export gokit_add
func gokit_add(title *C.char) *C.char { return C.CString(store.Add(C.GoString(title))) }

//export gokit_toggle
func gokit_toggle(id C.int) *C.char { return C.CString(store.Toggle(int(id))) }

//export gokit_remove
func gokit_remove(id C.int) *C.char { return C.CString(store.Remove(int(id))) }

//export gokit_remaining
func gokit_remaining() C.int { return C.int(store.Remaining()) }

// gokit_inspect reads a file the system opened for the UI (a photo the picker
// returned, say) and reports what it is. The UI closes the descriptor.
//
//export gokit_inspect
func gokit_inspect(fd C.int, name *C.char) *C.char {
	f := os.NewFile(uintptr(fd), C.GoString(name))
	if f == nil {
		return errorJSON("bad descriptor")
	}
	info, err := core.Inspect(f, C.GoString(name))
	if err != nil {
		return errorJSON(err.Error())
	}
	out, err := json.Marshal(struct {
		OK   bool          `json:"ok"`
		File core.FileInfo `json:"file"`
	}{true, info})
	if err != nil {
		return errorJSON(err.Error())
	}
	return C.CString(string(out))
}

// gokit_analyze_pixels describes a picture the UI decoded: the platform's
// decoder handles every format the camera writes (HEIF included), and the
// maths — brightness, histogram, colours — is the core's, so it's testable
// without a phone. The buffer belongs to JS and is only borrowed here.
//
//export gokit_analyze_pixels
func gokit_analyze_pixels(data unsafe.Pointer, length C.size_t, width, height, stride C.int) *C.char {
	if data == nil || length == 0 {
		return errorJSON("no pixels")
	}
	px := unsafe.Slice((*byte)(data), int(length))
	analysis, err := core.AnalyzePixels(px, int(width), int(height), int(stride))
	if err != nil {
		return errorJSON(err.Error())
	}
	out, err := json.Marshal(struct {
		OK    bool          `json:"ok"`
		Image core.Analysis `json:"image"`
	}{true, analysis})
	if err != nil {
		return errorJSON(err.Error())
	}
	return C.CString(string(out))
}

//export gokit_rows_count
func gokit_rows_count() C.int { return C.int(store.RowCount()) }

// gokit_rows is a page of the big list, in the compact form (see
// core.EncodeRows). It returns malloc'd bytes and their length: the bridge
// copies them into a JavaScript ArrayBuffer and frees them.
//
//export gokit_rows
func gokit_rows(offset, limit C.int, length *C.size_t) *C.uint8_t {
	b := core.EncodeRows(store.Rows(int(offset), int(limit)))
	*length = C.size_t(len(b))
	if len(b) == 0 {
		return nil
	}
	out := C.malloc(C.size_t(len(b)))
	if out == nil {
		*length = 0
		return nil
	}
	copy(unsafe.Slice((*byte)(out), len(b)), b)
	return (*C.uint8_t)(out)
}

// gokit_rows_json is the same page as JSON: the comparison that justifies the
// binary path.
//
//export gokit_rows_json
func gokit_rows_json(offset, limit C.int) *C.char {
	out, err := json.Marshal(struct {
		Rows []core.Row `json:"rows"`
	}{store.Rows(int(offset), int(limit))})
	if err != nil {
		return errorJSON(err.Error())
	}
	return C.CString(string(out))
}

// gokit_accel is one accelerometer sample, 50 times a second from the UI.
//
//export gokit_accel
func gokit_accel(x, y, z C.double) {
	store.Accel(float64(x), float64(y), float64(z))
}

// gokit_lifecycle tells the core the app moved to the foreground (1) or the
// background (0): a real app pauses its work there.
//
//export gokit_lifecycle
func gokit_lifecycle(foreground C.int) {
	store.SetForeground(foreground != 0)
}

func errorJSON(msg string) *C.char {
	out, _ := json.Marshal(struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}{false, msg})
	return C.CString(string(out))
}

func main() {
	// A c-shared library has no main of its own: the UI runs the app.
	_ = unsafe.Pointer(nil)
}
