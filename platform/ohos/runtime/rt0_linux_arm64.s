// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// Patched for HarmonyOS (see README.md): this replaces the Go
// runtime's own runtime/rt0_linux_arm64.s in HarmonyOS builds, through `go
// build -overlay` (platform/ohos/build.sh).
//
// musl calls a library's constructors with no arguments, so the argc and argv
// the runtime reads from R0/R1 are whatever happened to be in the registers:
// getGodebugEarly walks that "environment" and crashes on it. A library has no
// argv and no environment of its own — the core reads GOKIT_* what the host
// sets (gokit_setenv) — so it gets an empty one: a NULL argv and a NULL envp.

#include "textflag.h"

TEXT _rt0_arm64_linux(SB),NOSPLIT,$0
	JMP	_rt0_arm64(SB)

// When building with -buildmode=c-shared, this symbol is called when the shared
// library is loaded.
TEXT _rt0_arm64_linux_lib(SB),NOSPLIT,$0
	MOVD	$0, R0
	MOVD	$runtime·gokitArgv(SB), R1
	JMP	_rt0_arm64_lib(SB)

// An empty argv and envp: argv[0] is NULL, then the environment's terminator.
// The runtime's auxiliary vector comes after them, and reads as _AT_NULL, so it
// falls back to /proc/self/auxv.
DATA runtime·gokitArgv+0(SB)/8, $0
DATA runtime·gokitArgv+8(SB)/8, $0
GLOBL runtime·gokitArgv(SB), NOPTR, $16
