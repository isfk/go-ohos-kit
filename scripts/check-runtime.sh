#!/bin/sh
# Checks the Go runtime patches still are what they should be, and still fit the
# installed Go. Run by scripts/check.sh; run it directly after a Go upgrade.
#
# The logic lives in tools/patch (Go, tested): this stays a wrapper so the name
# in the docs, the CI and people's fingers keeps working.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/.." && pwd)

cd "$root/tools/patch"
exec go run . check -root "$root"
