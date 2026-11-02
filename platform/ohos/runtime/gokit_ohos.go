// Added to the Go runtime by HarmonyOS builds (platform/ohos/build.sh, through
// `go build -overlay`): the two functions of the HarmonyOS host that keep the
// running thread's g (go/tls_linux.c), which platform/ohos/tls_arm64.s
// calls. A library musl dlopens may not use initial-exec TLS, which is how the
// runtime reaches its thread's g elsewhere.

package runtime

import "unsafe"

//go:cgo_import_static gokit_g_load
//go:linkname __cgofn_gokit_g_load gokit_g_load
var __cgofn_gokit_g_load byte

//go:cgo_import_static gokit_g_save
//go:linkname __cgofn_gokit_g_save gokit_g_save
var __cgofn_gokit_g_save byte

var (
	gokitGLoad = unsafe.Pointer(&__cgofn_gokit_g_load)
	gokitGSave = unsafe.Pointer(&__cgofn_gokit_g_save)
)
