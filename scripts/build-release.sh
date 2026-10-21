#!/bin/bash
# Builds dist/agent-screensaver-<version>-macos.zip: the universal binary, the
# signed saver, the web runtime and the built-in extensions.
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION=$(git describe --tags --always --dirty)
NAME="agent-screensaver-$VERSION-macos"
STAGE="dist/$NAME"
ZIP="dist/$NAME.zip"
export MACOSX_DEPLOYMENT_TARGET=14.0

[ -d web/node_modules ] || (cd web && npm ci)
(cd web && npm run build)

# One build per architecture, joined into a universal binary. cgo needs the
# C compiler to target the same architecture as Go.
mkdir -p build
for arch in arm64 amd64; do
  cc="clang -arch arm64"
  [ "$arch" = amd64 ] && cc="clang -arch x86_64"
  CGO_ENABLED=1 GOOS=darwin GOARCH="$arch" CC="$cc" \
    go build -trimpath -ldflags "-s -w -X main.version=$VERSION" \
    -o "build/agent-screensaver-$arch" ./cmd/agent-screensaver
done
lipo -create build/agent-screensaver-arm64 build/agent-screensaver-amd64 \
  -output build/agent-screensaver

saver/build.sh

rm -rf "$STAGE" "$ZIP"
mkdir -p "$STAGE"
cp build/agent-screensaver "$STAGE/"
cp -R build/AgentScreensaver.saver build/runtime build/extensions "$STAGE/"
# ditto keeps the bundle layout and the saver's signature intact.
ditto -c -k --keepParent "$STAGE" "$ZIP"
echo "$ZIP"
