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
#
# store/ is in this list on purpose: modernc.org/sqlite is pure Go, so the
# database layer is testable here too — which is most of the reason it was
# picked over the cgo driver (see go/bench/sqlite for the numbers).
pkgs="./core/... ./store/..."

echo "== go test =="
(cd go && go test $pkgs)
(cd tools/patch && go test ./...)

echo "== go vet =="
(cd go && go vet $pkgs)
(cd tools/patch && go vet ./...)

echo "== runtime patches =="
"$here/check-runtime.sh"

echo "== ArkTS shell, against the sibling =="
# The shell is this experiment's control variable: the same behaviour has to run
# on both cores, so a fix that lands on one side has to land on the other. It
# drifted once already (the strict todo-mutation path, the composer's return
# value, the todo error banner existed only in the Rust fork) and nothing
# noticed until it was found by hand.
#
# Four files carry that behaviour; the rest of the shell is per-feature text,
# colour and layout, which is allowed to differ. Both copies are normalised to
# the same neutral names first (rustcore and gocore both become "gocore", both
# languages become "LANG"), then required to match byte for byte.
normalise() {
	sed -e 's/librustcore\.so/libgocore.so/g' \
		-e 's#types/librustcore#types/libgocore#g' \
		-e 's/RustCore/GoCore/g' \
		-e 's/rustcore/gocore/g' \
		-e 's/RustKit/GoKit/g' \
		-e 's/Rust/LANG/g' \
		-e 's/Go/LANG/g' "$1"
}
sib=../rust-ohos-kit/app/entry/src/main/ets
if [ -d "$sib" ]; then
	shell_tmp=$(mktemp -d)
	for pair in \
		core/GoCore.ts:core/RustCore.ts \
		core/events.ets:core/events.ets \
		features/todos/TodoComposer.ets:features/todos/TodoComposer.ets \
		pages/Index.ets:pages/Index.ets; do
		here_file=${pair%%:*}
		there_file=${pair##*:}
		normalise "app/entry/src/main/ets/$here_file" >"$shell_tmp/mine.ets"
		normalise "$sib/$there_file" >"$shell_tmp/theirs.ets"
		if ! cmp -s "$shell_tmp/mine.ets" "$shell_tmp/theirs.ets"; then
			echo "$here_file drifted from $there_file — change both, or neither:" >&2
			diff "$shell_tmp/mine.ets" "$shell_tmp/theirs.ets" | head -20 >&2
			rm -rf "$shell_tmp"
			exit 1
		fi
	done
	rm -rf "$shell_tmp"
	echo "4 behaviour files match (names normalised)"
else
	echo "no sibling checkout: skipped"
fi

echo "== shared conclusions page =="
# docs/go-vs-rust.md exists in ../rust-ohos-kit too, byte for byte the same, so
# that either repository reads correctly on its own. Two copies drift, so when the
# sibling checkout happens to be next to this one they are compared — and when it
# is not, this says so rather than pretending to have checked.
sibling=../rust-ohos-kit/docs/go-vs-rust.md
mine=docs/go-vs-rust.md
if [ -f "$sibling" ]; then
	if ! cmp -s "$mine" "$sibling"; then
		echo "docs/go-vs-rust.md differs from $sibling — change both, or neither" >&2
		diff "$mine" "$sibling" | head -20 >&2
		exit 1
	fi
	echo "identical to the sibling's copy"
else
	echo "no sibling checkout: skipped"
fi

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

# The regression this whole kit is about: a Go library with initial-exec TLS is
# refused by musl, and the app dies at load with a confusing JS error. The check
# itself lives in tools/patch (verify), next to the patches it protects.
(cd tools/patch && DEVECO_SDK_HOME="$sdk" go run . verify "$out")
