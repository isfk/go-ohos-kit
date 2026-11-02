//go:build linux && cgo

package main

/*
#cgo LDFLAGS: -lace_napi.z -lhilog_ndk.z
*/
import "C"

// The libraries above are the ones the bridge and the logging need: Node-API
// (the ArkTS runtime's own) and hilog. ArkUI's node API isn't here at all —
// the UI is ArkTS's.
