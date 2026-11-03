#!/bin/sh
# Production build signed with the same identity as dev builds, so a locally
# installed copy keeps its permission grants too.
set -eu
cd "$(dirname "$0")/.."
. scripts/signing.sh

identity="$(mygo_sign_identity)"
if [ -n "$identity" ]; then
  exec go tool mygo build -sign "$identity" "$@"
fi

echo "warning: no Apple Development identity found; the app is ad-hoc signed." >&2
exec go tool mygo build "$@"
