#!/bin/bash
# Makes internal/install/AppIcon.icns, the icon of the launcher app, from
# internal/platform/appicon.png (1024x1024). Run it again after the artwork
# changes and commit the new .icns. Needs macOS (sips and iconutil).
set -euo pipefail
cd "$(dirname "$0")/.."

SOURCE=internal/platform/appicon.png
OUTPUT=internal/install/AppIcon.icns
SET=$(mktemp -d)/AppIcon.iconset
mkdir -p "$SET"

for size in 16 32 128 256 512; do
  sips -z "$size" "$size" "$SOURCE" --out "$SET/icon_${size}x${size}.png" >/dev/null
  sips -z "$((size * 2))" "$((size * 2))" "$SOURCE" --out "$SET/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$SET" -o "$OUTPUT"
rm -rf "$(dirname "$SET")"
echo "wrote $OUTPUT"
