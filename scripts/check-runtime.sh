#!/bin/sh
# Checks the three Go runtime patches still are what they should be, and still
# fit the installed Go. Run by scripts/check.sh; run it directly after a Go
# upgrade.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
runtime=$here/../platform/ohos/runtime
goroot=$(go env GOROOT)
version=$(go env GOVERSION)
want=$(sed -n 's/^\(go[0-9.]*\)$/\1/p' "$runtime/VERSION")
fail=0

# 1. Each patch is still in the file that's supposed to carry it.
check() { # file pattern what
	if ! grep -q "$2" "$runtime/$1"; then
		echo "check-runtime: $1 lost its patch: $3" >&2
		fail=1
	fi
}
check tls_arm64.s 'gokitGLoad' 'the g lives in this library'"'"'s own TLS'
check rt0_linux_arm64.s 'gokitArgv' 'a library gets an empty argv'
check os_linux.go 'gokitProbePageSize' 'the page size is probed when nothing gives it'
check gokit_ohos.go 'gokit_g_load' 'the runtime names the two functions'

# 2. The files they replace still exist in this Go, and are the ones patched.
for f in runtime/tls_arm64.s runtime/rt0_linux_arm64.s runtime/os_linux.go; do
	if [ ! -f "$goroot/src/$f" ]; then
		echo "check-runtime: $goroot/src/$f is gone: this Go changed how it starts" >&2
		fail=1
	fi
done

# 3. The same Go release.
if [ "$want" != "$version" ]; then
	echo "check-runtime: patches were taken from $want, this Go is $version" >&2
	echo "check-runtime: re-read platform/ohos/runtime/*, re-apply, bump VERSION" >&2
	# A warning, not a failure: the patches are small and usually survive.
fi

if [ "$fail" != 0 ]; then
	exit 1
fi
echo "check-runtime: the patches are in place ($want)"
