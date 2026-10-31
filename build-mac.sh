#!/usr/bin/env bash
# Build pmm-dump-inspect for macOS (Finder app) and Linux (copy to the lab hosts).
#
# Usage:
#   ./build-mac.sh            # universal binary (arm64 + amd64) -> dist/pmm-dump-inspect
#   ./build-mac.sh native     # only the current architecture   -> dist/pmm-dump-inspect
#   both also build dist/pmm-dump-inspect-linux-amd64

set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
DIST="$ROOT/dist"
MODE="${1:-universal}"

GO="$(command -v go || true)"
[ -n "$GO" ] || { [ -x /opt/homebrew/bin/go ] && GO=/opt/homebrew/bin/go; } || true
[ -n "$GO" ] || { [ -x /usr/local/go/bin/go ] && GO=/usr/local/go/bin/go; } || true
[ -n "$GO" ] || { echo "go not found" >&2; exit 1; }
echo ">> using $("$GO" version)"

cd "$ROOT"
mkdir -p "$DIST"

build() { # $1 = GOARCH, $2 = output path
  echo ">> building darwin/$1"
  rm -f "$2" # go build refuses to overwrite a lipo (universal) binary
  CGO_ENABLED=0 GOOS=darwin GOARCH="$1" "$GO" build -trimpath -ldflags "-s -w" -o "$2" .
}

case "$MODE" in
  native)
    build "$("$GO" env GOHOSTARCH)" "$DIST/pmm-dump-inspect"
    ;;
  universal)
    build arm64 "$DIST/pmm-dump-inspect-darwin-arm64"
    build amd64 "$DIST/pmm-dump-inspect-darwin-amd64"
    lipo -create -output "$DIST/pmm-dump-inspect" "$DIST/pmm-dump-inspect-darwin-arm64" "$DIST/pmm-dump-inspect-darwin-amd64"
    ;;
  *)
    echo "unknown mode: $MODE (use 'universal' or 'native')" >&2
    exit 1
    ;;
esac

chmod +x "$DIST/pmm-dump-inspect"
echo ">> building linux/amd64"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 "$GO" build -trimpath -ldflags "-s -w" -o "$DIST/pmm-dump-inspect-linux-amd64" .
echo ">> done: $DIST/pmm-dump-inspect ($(lipo -archs "$DIST/pmm-dump-inspect"))"
