// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// Patched for HarmonyOS (see README.md): this replaces the Go
// runtime's own runtime/tls_arm64.s in HarmonyOS builds, through `go build
// -overlay` (platform/ohos/build.sh). A library musl dlopens may not have
// initial-exec TLS, which is how the runtime reaches its thread's g
// (runtime.tls_g); here the g lives in the library's own dynamic TLS,
// through go/tls_linux.c. Nothing above GOOS_linux changes: iOS and
// Android builds keep the runtime's own file.

#include "go_asm.h"
#include "go_tls.h"
#include "funcdata.h"
#include "textflag.h"
#include "tls_arm64.h"

TEXT runtime·load_g(SB),NOSPLIT,$16-0
#ifndef GOOS_darwin
#ifndef GOOS_openbsd
#ifndef GOOS_windows
	MOVB	runtime·iscgo(SB), R0
	CBZ	R0, nocgo
#endif
#endif
#endif

#ifdef GOOS_linux
	// The loader's own __tls_get_addr: dynamic TLS, which musl allows here.
	MOVD	R30, 0(RSP)
	MOVD	runtime·gokitGLoad(SB), R27
	BL	(R27)
	MOVD	0(RSP), R30
	MOVD	R0, g
#else
	MRS_TPIDR_R0
#ifdef TLS_darwin
	// Darwin sometimes returns unaligned pointers
	AND	$0xfffffffffffffff8, R0
#endif
	MOVD	runtime·tls_g(SB), R27
	MOVD	(R0)(R27), g
#endif

nocgo:
	RET

TEXT runtime·save_g(SB),NOSPLIT,$16-0
#ifndef GOOS_darwin
#ifndef GOOS_openbsd
#ifndef GOOS_windows
	MOVB	runtime·iscgo(SB), R0
	CBZ	R0, nocgo
#endif
#endif
#endif

#ifdef GOOS_linux
	MOVD	R30, 0(RSP)
	MOVD	runtime·gokitGSave(SB), R27
	MOVD	g, R0
	BL	(R27)
	MOVD	0(RSP), R30
#else
	MRS_TPIDR_R0
#ifdef TLS_darwin
	AND	$0xfffffffffffffff8, R0
#endif
	MOVD	runtime·tls_g(SB), R27
	MOVD	g, (R0)(R27)
#endif

nocgo:
	RET

#ifdef TLSG_IS_VARIABLE
#ifdef GOOS_android
// Use the free TLS_SLOT_APP slot #2 on Android Q.
// Earlier androids are set up in gcc_android.c.
DATA runtime·tls_g+0(SB)/8, $16
#endif
GLOBL runtime·tls_g+0(SB), NOPTR, $8
#else
#ifndef GOOS_linux
GLOBL runtime·tls_g+0(SB), TLSBSS, $8
#else
// Nothing reaches tls_g on HarmonyOS now, but asm_arm64.s still passes its
// address to _cgo_init: a plain global keeps the symbol without TLS, so the
// linker puts no initial-exec relocation in the library.
GLOBL runtime·tls_g+0(SB), NOPTR, $8
#endif
#endif
