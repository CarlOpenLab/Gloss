#!/bin/sh
# Prints the code-signing identity used for local builds. Signing every build
# with the same identity keeps the app's designated requirement stable, so
# macOS keeps TCC grants (files, screen recording, accessibility) across
# rebuilds instead of treating each build as a new app.
mygo_sign_identity() {
  if [ -n "${MACOS_SIGNING_IDENTITY:-}" ]; then
    printf '%s\n' "$MACOS_SIGNING_IDENTITY"
    return 0
  fi
  [ "$(uname -s)" = Darwin ] || return 0
  security find-identity -v -p codesigning 2>/dev/null |
    sed -n 's/.*"\(Apple Development: .*\)"/\1/p' | head -1
}
