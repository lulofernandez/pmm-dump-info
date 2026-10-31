#!/usr/bin/env bash
# Install ~/Applications/pmm-dump-inspect.app so pmm-dump files can be
# inspected from Finder (right-click > Open With > pmm-dump-inspect). It runs
# dist/pmm-dump-inspect from this repo, so a later ./build-mac.sh is picked up
# without reinstalling.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
BIN="$ROOT/dist/pmm-dump-inspect"
APP="$HOME/Applications/pmm-dump-inspect.app"

[ -x "$BIN" ] || "$ROOT/build-mac.sh"

mkdir -p "$HOME/Applications"
rm -rf "$APP"
sed "s|__BIN__|$BIN|" "$ROOT/macos/pmm-dump-inspect-open.applescript" | osacompile -o "$APP"

# accept any file type in "Open With" (dumps are .tar.gz, sometimes renamed)
plutil -replace CFBundleDocumentTypes -json \
  '[{"CFBundleTypeName":"Any file","CFBundleTypeRole":"Viewer","LSItemContentTypes":["public.data","public.text","public.item"]}]' \
  "$APP/Contents/Info.plist"
plutil -replace CFBundleIdentifier -string "com.local.pmm-dump-inspect" "$APP/Contents/Info.plist"
codesign --force --sign - "$APP" >/dev/null 2>&1 || true

/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister -f "$APP"
echo ">> installed $APP"
