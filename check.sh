#!/bin/sh
# Check that TinyGo gen-device-svd makes a device file that builds for each SVD file.
# Usage: ./check.sh path/to/tinygo
set -eu

tinygo=$(cd "$1" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

(cd "$tinygo" && go build -o "$work/gen-device-svd" ./tools/gen-device-svd)
mkdir "$work/out"
if ! "$work/gen-device-svd" -source=https://github.com/tinygo-org/esp32-svd -interrupts=software svd/ "$work/out/" >"$work/gen.log" 2>&1; then
	cat "$work/gen.log"
	exit 1
fi
if grep 'struct size' "$work/gen.log"; then
	echo "gen-device-svd found clusters with the wrong size"
	exit 1
fi

# Build each device file as its own package against a copy of runtime/volatile.
# An empty .s file lets Go build functions that have no body.
mod=$work/mod
mkdir -p "$mod/volatile"
printf 'module check\n\ngo 1.22\n' >"$mod/go.mod"
cp "$tinygo"/src/runtime/volatile/*.go "$mod/volatile/"
touch "$mod/volatile/stub.s"
for f in "$work"/out/*.go; do
	chip=$(basename "$f" .go)
	mkdir "$mod/$chip"
	sed -e '/^\/\/go:build/d' -e 's|"runtime/volatile"|"check/volatile"|' "$f" >"$mod/$chip/$chip.go"
	touch "$mod/$chip/stub.s"
done
(cd "$mod" && go build ./...)
echo "$(ls "$work"/out/*.go | wc -l) device files build"
