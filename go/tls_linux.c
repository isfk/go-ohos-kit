// The Go runtime keeps the current thread's g in thread-local storage. On the
// other platforms it reaches it through runtime.tls_g with an initial-exec
// access, which is what a library linked at program start can do. A library
// musl dlopens may not: its TLS is dynamic, and the loader stops a
// "initial-exec TLS resolves to dynamic definition" (see platform/ohos/
// tls_arm64.s, which the build replaces the runtime's own with).
//
// So on HarmonyOS the core's g lives in this library's TLS instead: dynamic,
// resolved by the loader through __tls_get_addr, which musl supports. A
// thread that has never run Go has a nil g, and the runtime attaches an m to
// it (cgocallback's needm), as for any thread Go didn't make.

static __thread void *gSlot;

// gokit_g_load returns this thread's g: the runtime's load_g.
void *gokit_g_load(void) { return gSlot; }

// gokit_g_save sets this thread's g: the runtime's save_g.
void gokit_g_save(void *g) { gSlot = g; }
