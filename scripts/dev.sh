#!/bin/sh
# Development build signed with a stable identity: macOS then keeps the
# permission grants (screen recording, files and folders, accessibility)
# across rebuilds.
set -eu
cd "$(dirname "$0")/.."
. scripts/signing.sh

identity="$(mygo_sign_identity)"
if [ -n "$identity" ]; then
  exec go tool mygo dev -sign "$identity" "$@"
fi

echo "warning: no Apple Development identity found; the dev app is ad-hoc signed," >&2
echo "         so macOS asks for permissions again after every rebuild." >&2
exec go tool mygo dev "$@"
