#!/bin/sh
# Makes a new app: a Go core and the HarmonyOS project around it, both copies
# of the kit's (which stay the reference when they drift).
#
#   platform/ohos/new.sh <name> [dir] [--bundle id] [--label "My App"]
#
# <name> is the directory's name under apps/ (dir overrides where it goes).
# The bundle ID defaults to the kit demo's, so the app installs and runs with
# the signing material this machine already has: a *new* bundle ID needs a new
# signing profile (DevEco's automatic signing), which is why it's opt-in.
set -eu

name=
dir=
bundle=
label=

while [ $# -gt 0 ]; do
	case $1 in
	--bundle) bundle=$2; shift 2 ;;
	--label) label=$2; shift 2 ;;
	-h | --help)
		sed -n '2,11p' "$0" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*)
		if [ -z "$name" ]; then name=$1; else dir=$1; fi
		shift
		;;
	esac
done

if [ -z "$name" ]; then
	echo "usage: $0 <name> [dir] [--bundle id] [--label \"My App\"]" >&2
	exit 2
fi

here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
if [ -z "$dir" ]; then dir=$root/apps/$name; fi
if [ -e "$dir" ]; then
	echo "new.sh: $dir is already there" >&2
	exit 1
fi
if [ -z "$label" ]; then label=$name; fi

mkdir -p "$dir"
# The core: its own copy, to change freely.
cp -R "$root/go" "$dir/go"
rm -f "$dir/go"/*.so "$dir/go"/*.h
# The project, without what a build left behind.
cp -R "$root/app" "$dir/app"
# build-profile.json5 is per-machine (this machine's cert paths and passwords),
# so it isn't tracked: the template stands in when it's absent.
if [ ! -f "$root/app/build-profile.json5" ]; then
	cp "$dir/app/build-profile.example.json5" "$dir/app/build-profile.json5"
fi
rm -rf "$dir/app/entry/build" "$dir/app/.hvigor" "$dir/app/entry/oh_modules" \
       "$dir/app/entry/libs" "$dir/app/entry/.cxx" "$dir/app/build" 2>/dev/null || true

old_bundle=$(sed -n 's/.*"bundleName"[^"]*"\([^"]*\)".*/\1/p' "$dir/app/AppScope/app.json5")
if [ -z "$bundle" ]; then
	bundle=$old_bundle
	echo "new.sh: keeping the bundle ID $bundle, so the signing this machine has still fits" >&2
	echo "new.sh: give --bundle id for a new one (then sign it in DevEco: Project Structure → Signing Configs)" >&2
fi

python3 - "$dir" "$name" "$bundle" "$old_bundle" "$label" <<'PY'
import sys, re, pathlib
dir, name, bundle, old, label = sys.argv[1:6]
d = pathlib.Path(dir)

def edit(rel, fn):
    p = d / rel
    if not p.exists():
        return
    p.write_text(fn(p.read_text()))

edit("app/AppScope/app.json5", lambda s: s
     .replace(f'"{old}"', f'"{bundle}"')
     .replace(f'"$string:app_name"', '"$string:app_name"'))
edit("app/AppScope/resources/base/element/string.json",
     lambda s: re.sub(r'("name"\s*:\s*"app_name",\s*"value"\s*:\s*)"[^"]*"', rf'\1"{label}"', s))
edit("app/entry/src/main/resources/base/element/string.json",
     lambda s: re.sub(r'("EntryAbility_label"\s*:\s*"value"\s*:\s*)"[^"]*"', rf'\1"{label}"', s))
edit("app/oh-package.json5", lambda s: s.replace('"name": "app"', f'"name": "{name}"'))
edit("app/entry/oh-package.json5", lambda s: s.replace('"name": "entry"', '"name": "entry"'))

# A new bundle ID can't use the old signing profile (it names the bundle):
# drop it, and the build makes an unsigned HAP until DevEco signs it.
if bundle != old:
    edit("app/build-profile.json5", lambda s: s
         .replace('"signingConfig": "default",\n', '')
         .replace('"signingConfig": "default",', ''))
    print("new.sh: dropped the old signing config; enable signing in DevEco before running")
PY

# The project's own ways in, pointing at the kit's scripts.
cat >"$dir/build.sh" <<SH
#!/bin/sh
# Builds this app's Go core into its HarmonyOS project.
set -eu
exec "$here/build.sh" "\$(dirname "\$0")/go" -project "\$(dirname "\$0")/app" "\$@"
SH
cat >"$dir/run.sh" <<SH
#!/bin/sh
# Builds, installs and starts this app on the connected device.
set -eu
exec "$here/run.sh" "\$(dirname "\$0")/app" -go "\$(dirname "\$0")/go" "\$@"
SH
chmod +x "$dir/build.sh" "$dir/run.sh"

echo "gokit: $dir"
echo "  cd $dir && ./run.sh" >&2
echo "  hdc shell hilog -T gokit" >&2
