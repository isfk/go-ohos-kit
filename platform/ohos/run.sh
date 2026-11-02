#!/bin/sh
# Builds the Go core, the app around it, installs it on the connected
# HarmonyOS device and starts it, then shows the core's log.
#
#   platform/ohos/run.sh [app project dir] [-go dir] [-bundle id] [-ability EntryAbility]
set -eu

project=app
go_dir=
bundle=
ability=EntryAbility

while [ $# -gt 0 ]; do
	case $1 in
	-go) go_dir=$2; shift 2 ;;
	-bundle) bundle=$2; shift 2 ;;
	-ability) ability=$2; shift 2 ;;
	-h | --help)
		sed -n '2,6p' "$0" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*) project=$1; shift ;;
	esac
done

sdk=${DEVECO_SDK_HOME:-/Users/shen/command-line-tools/sdk}
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
project=$(cd "$project" && pwd)
hdc=$sdk/default/openharmony/toolchains/hdc

if [ -z "$bundle" ]; then
	bundle=$(sed -n 's/.*"bundleName"[^"]*"\([^"]*\)".*/\1/p' "$project/AppScope/app.json5")
fi

# build-profile.json5 is per-machine (DevEco writes this machine's cert paths
# and passwords into it), so it's not in the repo: start from the template.
if [ ! -f "$project/build-profile.json5" ]; then
	cp "$project/build-profile.example.json5" "$project/build-profile.json5"
	echo "gokit: created build-profile.json5 from the template — it has no signing yet" >&2
	echo "gokit: (DevEco → Project Structure → Signing Configs → Automatically generate signature)" >&2
fi

if [ -z "$go_dir" ]; then
	go_dir=$root/go
fi
"$here/build.sh" "$go_dir" -project "$project" >/dev/null

echo "gokit: building the HAP" >&2
(cd "$project" && DEVECO_SDK_HOME="$sdk" "$sdk/../bin/hvigorw" assembleHap \
	--mode module -p product=default -p buildMode=debug --no-daemon >/dev/null)

hap=$project/entry/build/default/outputs/default/entry-default-signed.hap
if [ ! -f "$hap" ]; then
	# A HarmonyOS app only installs signed, and a signing profile names the
	# bundle: a new one is signed in DevEco, once.
	echo "gokit: the app isn't signed for $bundle." >&2
	echo "gokit: open $project in DevEco once → Project Structure → Signing Configs →" >&2
	echo "gokit: 'Automatically generate signature' (a Huawei ID), then run this again." >&2
	exit 1
fi
echo "gokit: installing $bundle" >&2
"$hdc" install -r "$hap" | tail -1
"$hdc" shell hilog -r >/dev/null 2>&1 || true
"$hdc" shell aa force-stop "$bundle" >/dev/null 2>&1 || true
"$hdc" shell aa start -a "$ability" -b "$bundle" | tail -1

sleep 4
echo "gokit: log (hilog -T gokit)" >&2
"$hdc" shell "hilog -x | grep -iE 'gokit|SIGSEGV|panic|fatal' | tail -30"
