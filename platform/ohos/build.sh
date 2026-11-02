#!/bin/sh
# Builds a Go core as a HarmonyOS NAPI module: go/ → libgocore.so, a shared
# library ArkTS imports and calls.
#
#   platform/ohos/build.sh [go dir] [-o libgocore.so] [-project <dir>]
#                          [-abi arm64-v8a|x86_64] [-release]
#
# The Go runtime needs three files replaced to live in a HarmonyOS app (see
# runtime/ and ../README.md): this script passes them with `go build
# -overlay`, so the installed Go isn't touched. The NDK comes from
# $DEVECO_SDK_HOME, else from the command-line-tools SDK.
set -eu

go_dir=go
out=libgocore.so
project=
abi=arm64-v8a
release=

while [ $# -gt 0 ]; do
	case $1 in
	-abi) abi=$2; shift 2 ;;
	-o) out=$2; shift 2 ;;
	-project) project=$2; shift 2 ;;
	-release) release=1; shift ;;
	-h | --help)
		sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*) go_dir=$1; shift ;;
	esac
done

sdk=${DEVECO_SDK_HOME:-/Users/shen/command-line-tools/sdk}
ndk=$sdk/default/openharmony/native
if [ ! -d "$ndk" ]; then
	echo "build.sh: no NDK at $ndk (set DEVECO_SDK_HOME)" >&2
	exit 1
fi

# HarmonyOS NEXT's ABIs: phones and tablets (arm64), an Intel Mac's emulator
# (x86_64). The NDK names the target aarch64-linux-ohos or x86_64-linux-ohos.
case $abi in
arm64-v8a) goarch=arm64; triple=aarch64-unknown-linux-ohos; gosuffix=arm64 ;;
x86_64) goarch=amd64; triple=x86_64-unknown-linux-ohos; gosuffix=amd64 ;;
*)
	echo "build.sh: unknown ABI $abi (arm64-v8a or x86_64)" >&2
	exit 2
	;;
esac
# The runtime files this build replaces are arm64's, and only those matter:
# HarmonyOS NEXT phones are arm64.
if [ "$goarch" != arm64 ]; then
	echo "build.sh: only arm64-v8a is supported for now (the runtime patches are arm64's)" >&2
	exit 2
fi

go_dir=$(cd "$go_dir" && pwd)
case $out in
/*) ;;
*) out=$(pwd)/$out ;;
esac
here=$(cd "$(dirname "$0")" && pwd)
goroot=$(go env GOROOT)

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT INT TERM
cat >"$work/overlay.json" <<JSON
{"Replace": {
  "$goroot/src/runtime/tls_arm64.s": "$here/runtime/tls_arm64.s",
  "$goroot/src/runtime/rt0_linux_arm64.s": "$here/runtime/rt0_linux_arm64.s",
  "$goroot/src/runtime/os_linux.go": "$here/runtime/os_linux.go",
  "$goroot/src/runtime/gokit_ohos.go": "$here/runtime/gokit_ohos.go"
}}
JSON

echo "gokit: building $go_dir for $abi" >&2
(cd "$go_dir" && CGO_ENABLED=1 GOOS=linux GOARCH="$goarch" \
	CC="$ndk/llvm/bin/$triple-clang" \
	CXX="$ndk/llvm/bin/$triple-clang++" \
	go build -buildmode=c-shared -overlay "$work/overlay.json" -o "$out" .)

if [ -n "$release" ]; then
	"$ndk/llvm/bin/llvm-strip" --strip-unneeded "$out"
fi
echo "gokit: $out" >&2

if [ -n "$project" ]; then
	libs=$project/entry/libs/$abi
	mkdir -p "$libs"
	cp "$out" "$libs/libgocore.so"
	echo "gokit: $libs/libgocore.so" >&2
fi
