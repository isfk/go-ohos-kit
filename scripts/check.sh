#!/bin/sh
# Everything that can be checked without a phone: the logic's tests, the
# patches, and a real cross build of the library.
#
#   scripts/check.sh            # all of it
#   scripts/check.sh --no-build # skip the NDK part (a plain Go machine)
set -eu

here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/.." && pwd)
cd "$root"
build=1
[ "${1:-}" = "--no-build" ] && build=

# go/ itself is cgo + the HarmonyOS NDK (hilog/log.h): it only compiles with the
# cross compiler, in the build step below. So the host-side checks name the pure
# Go packages instead of ./... — which would fail on any Linux box without the
# NDK (a Mac hides it, because every bridge file is *_linux.c).
pkgs=./core/...

echo "== go test =="
(cd go && go test $pkgs)

echo "== go vet =="
(cd go && go vet $pkgs)

echo "== runtime patches =="
"$here/check-runtime.sh"

if [ -z "$build" ]; then
	echo "== cross build: skipped =="
	exit 0
fi

sdk=${DEVECO_SDK_HOME:-/Users/shen/command-line-tools/sdk}
if [ ! -d "$sdk/default/openharmony/native" ]; then
	echo "== cross build: skipped (no NDK; set DEVECO_SDK_HOME) =="
	exit 0
fi

echo "== cross build =="
out=$(mktemp -d)/libgocore.so
trap 'rm -rf "$(dirname "$out")"' EXIT INT TERM
"platform/ohos/build.sh" go -o "$out" >/dev/null
readelf=$sdk/default/openharmony/native/llvm/bin/llvm-readelf

# The regression this whole kit is about: a Go library with initial-exec TLS
# is refused by musl, and the app dies at load with a confusing JS error.
if [ "$("$readelf" -rW "$out" | grep -c TLS_TPREL64 || true)" != 0 ]; then
	echo "check: libgocore.so has initial-exec TLS relocations: musl will refuse it" >&2
	exit 1
fi
for lib in libace_napi.z.so libhilog_ndk.z.so libc.so; do
	if ! "$readelf" -d "$out" | grep -q "$lib"; then
		echo "check: libgocore.so doesn't link $lib" >&2
		exit 1
	fi
done
echo "check: libgocore.so is clean ($(du -h "$out" | cut -f1 | tr -d ' '), no initial-exec TLS)"
