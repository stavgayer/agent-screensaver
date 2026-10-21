#!/bin/bash
# Builds build/AgentProbe.saver (universal arm64 + x86_64, ad-hoc signed).
set -euo pipefail
cd "$(dirname "$0")/.."

OUT=build/AgentProbe.saver
TMP=build/probe-tmp
rm -rf "$OUT" "$TMP"
mkdir -p "$OUT/Contents/MacOS" "$TMP"

for arch in arm64 x86_64; do
  swiftc -emit-library -module-name AgentProbe -target "$arch-apple-macos14" \
    -framework ScreenSaver -framework WebKit -framework AppKit \
    probe/ProbeView.swift -o "$TMP/AgentProbe-$arch"
done

lipo -create "$TMP/AgentProbe-arm64" "$TMP/AgentProbe-x86_64" \
  -output "$OUT/Contents/MacOS/AgentProbe"
cp probe/Info.plist "$OUT/Contents/Info.plist"
codesign --force --sign - "$OUT"
rm -rf "$TMP"
echo "built $OUT ($(lipo -archs "$OUT/Contents/MacOS/AgentProbe"))"
