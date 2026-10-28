#!/bin/bash
# Builds build/AgentScreensaver.saver (universal arm64 + x86_64, ad-hoc signed).
# The bundle holds the binary, Info.plist and the System Settings thumbnail
# (thumbnail.svg is its source); the web page lives in DATA_DIR.
set -euo pipefail
cd "$(dirname "$0")/.."

OUT=build/AgentScreensaver.saver
TMP=build/saver-tmp
rm -rf "$OUT" "$TMP"
mkdir -p "$OUT/Contents/MacOS" "$OUT/Contents/Resources" "$TMP"

for arch in arm64 x86_64; do
  swiftc -emit-library -module-name AgentScreensaver -target "$arch-apple-macos14" \
    -framework ScreenSaver -framework WebKit -framework AppKit \
    saver/AgentSaverView.swift -o "$TMP/AgentScreensaver-$arch"
done

lipo -create "$TMP/AgentScreensaver-arm64" "$TMP/AgentScreensaver-x86_64" \
  -output "$OUT/Contents/MacOS/AgentScreensaver"
cp saver/Info.plist "$OUT/Contents/Info.plist"
cp saver/thumbnail.png saver/thumbnail@2x.png "$OUT/Contents/Resources/"
codesign --force --sign - "$OUT"
rm -rf "$TMP"
echo "built $OUT ($(lipo -archs "$OUT/Contents/MacOS/AgentScreensaver"))"
