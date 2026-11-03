package main

// patch is one file the build hands to `go build -overlay`: what it replaces in
// GOROOT/src/runtime, why it exists, and the two things that break silently
// after a Go upgrade — the identifiers our version carries, and the upstream
// lines we hook.
type patch struct {
	file     string   // in platform/ohos/runtime
	replaces string   // in GOROOT/src/runtime; "" means a file Go doesn't have
	what     string   // why it exists, in a few words
	markers  []string // identifiers our copy must contain: proof the patch is in
	anchors  []string // upstream lines it hooks: an upgrade that moves them breaks it
}

var patches = []patch{
	{
		file:     "tls_arm64.s",
		replaces: "tls_arm64.s",
		what:     "the g lives in this library's own TLS",
		markers:  []string{"gokitGLoad"},
		anchors:  []string{"TEXT runtime·load_g(SB),NOSPLIT,$0", `#include "tls_arm64.h"`},
	},
	{
		file:     "rt0_linux_arm64.s",
		replaces: "rt0_linux_arm64.s",
		what:     "a library gets an empty argv/envp",
		markers:  []string{"gokitArgv"},
		anchors:  []string{"TEXT _rt0_arm64_linux(SB),NOSPLIT,$0"},
	},
	{
		file:     "os_linux.go",
		replaces: "os_linux.go",
		what:     "the page size is probed when nothing hands it over",
		markers:  []string{"gokitProbePageSize", "gokitSysargs"},
		// sysargs is renamed by the patch, mincore is what the probe leans on:
		// if either moves, the patch lands in the wrong place.
		anchors: []string{"func sysargs(argc int32, argv **byte) {", "func mincore("},
	},
	{
		file:     "gokit_ohos.go",
		replaces: "", // added, not replaced
		what:     "the runtime names the two functions the host provides",
		markers:  []string{"gokit_g_load", "gokit_g_save"},
	},
}
