#!/usr/bin/env bash
# Regenerate internal/inspect/qan_columns.txt from the ClickHouse migrations of
# PMM: percona/pmm qan-api2/migrations/sql, with the first release of each
# migration from its tags (percona/qan-api2 for the ones older than PMM 2.33).
#
# Usage:
#   tools/update-qan-schema.sh           # rewrite qan_columns.txt, then ./build-mac.sh
#   tools/update-qan-schema.sh --check   # exit 1 if qan_columns.txt is out of date
#                                        # (only the "checked" date is refreshed when it is not)
# Exit 2: the migrations could not be fetched or parsed.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/internal/inspect/qan_columns.txt"
CHECK=0
[ "${1:-}" = "--check" ] && CHECK=1

GO="$(command -v go || true)"
[ -n "$GO" ] || { [ -x /opt/homebrew/bin/go ] && GO=/opt/homebrew/bin/go; } || true
[ -n "$GO" ] || { [ -x /usr/local/go/bin/go ] && GO=/usr/local/go/bin/go; } || true
[ -n "$GO" ] || { echo "go not found" >&2; exit 2; }

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
trap 'echo "update-qan-schema: failed, see the messages above" >&2; exit 2' ERR

echo ">> fetching percona/pmm and percona/qan-api2 migrations" >&2
git clone -q --filter=blob:none --no-checkout https://github.com/percona/pmm.git "$WORK/pmm"
git -C "$WORK/pmm" sparse-checkout set qan-api2/migrations/sql
git -C "$WORK/pmm" checkout -q
git clone -q --filter=blob:none --no-checkout https://github.com/percona/qan-api2.git "$WORK/qan-api2"

releases() { git -C "$1" tag | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sort -V; }
first_release() { # $1 repo, $2 path -> first release tag containing the commit that added it
  local c
  c="$(git -C "$1" log --diff-filter=A --format=%H -1 -- "$2")"
  [ -n "$c" ] || return 0
  git -C "$1" tag --contains "$c" | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sort -V | head -1 || true
}

SQL="$WORK/pmm/qan-api2/migrations/sql"
: > "$WORK/versions"
for f in "$SQL"/*.up.sql; do
  b="$(basename "$f")"
  t="$(first_release "$WORK/pmm" "qan-api2/migrations/sql/$b")"
  old="$(first_release "$WORK/qan-api2" "migrations/sql/$b")"
  if [ -n "$old" ] && { [ -z "$t" ] || [ "$(printf '%s\n%s\n' "$old" "$t" | sort -V | head -1)" = "$old" ]; }; then
    t="$old"
  fi
  echo "$b ${t:-unreleased}" >> "$WORK/versions"
done

LATEST="$(releases "$WORK/pmm" | tail -1)"
COMMIT="$(git -C "$WORK/pmm" log -1 --format='%h %cs')"
cd "$ROOT"
"$GO" run ./tools/qanschema -sql "$SQL" -versions "$WORK/versions" -release "$LATEST" \
  -source "percona/pmm qan-api2/migrations/sql @ $COMMIT" -checked "$(date +%F)" > "$WORK/new"
trap - ERR

# compare ignoring the source commit and the date
strip() { grep -v -E '^# (source|checked):' "$1" || true; }
if [ "$CHECK" = 1 ]; then
  if diff -u <(strip "$OUT") <(strip "$WORK/new") > "$WORK/diff"; then
    sed -i.bak -E "s/^# checked: .*/# checked: $(date +%F)/" "$OUT" && rm -f "$OUT.bak"
    echo ">> qan_columns.txt is up to date (PMM $LATEST)" >&2
    exit 0
  fi
  cat "$WORK/diff" >&2
  echo ">> qan_columns.txt is OUT OF DATE: run tools/update-qan-schema.sh and rebuild" >&2
  exit 1
fi
cp "$WORK/new" "$OUT"
grep '^@schema ' "$OUT" >&2
echo ">> wrote $OUT (PMM $LATEST); now run ./build-mac.sh" >&2
