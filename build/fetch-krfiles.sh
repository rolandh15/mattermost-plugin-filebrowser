#!/usr/bin/env bash
# fetch-krfiles.sh — download the krfiles native release archive for a given
# OS/arch, extract it into server/fb/krf/native/<target>/, and verify the shim
# + header + shared library all landed.
#
# Usage: ./build/fetch-krfiles.sh <target>
#   target ∈ {linux-amd64, linux-arm64, darwin-amd64, darwin-arm64}
#
# Pinning: the version is hard-coded below so that CI cannot silently drift to
# a newer krfiles release. Bump it alongside a test that verifies the plugin
# still builds against the new artifacts.

set -euo pipefail

KRFILES_VERSION="0.1.1"

if [[ $# -lt 1 ]]; then
    echo "Usage: $0 <target>" >&2
    echo "  target ∈ {linux-amd64, linux-arm64, darwin-amd64, darwin-arm64}" >&2
    exit 1
fi

TARGET="$1"

# Map the Go/Mattermost target name onto the krfiles artifact name.
case "$TARGET" in
    linux-amd64)  KRFILES_TARGET="linux-x64" ;;
    linux-arm64)  KRFILES_TARGET="linux-arm64" ;;
    darwin-amd64) KRFILES_TARGET="macos-x64" ;;
    darwin-arm64) KRFILES_TARGET="macos-arm64" ;;
    *)
        echo "Error: unsupported target '$TARGET'." >&2
        echo "Valid targets: linux-amd64, linux-arm64, darwin-amd64, darwin-arm64" >&2
        exit 1
        ;;
esac

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
DEST_DIR="$REPO_ROOT/server/fb/krf/native/$TARGET"
ARCHIVE_NAME="krfiles-native-${KRFILES_TARGET}.tar.gz"
ARCHIVE_URL="https://github.com/rolandh15/krfiles/releases/download/v${KRFILES_VERSION}/${ARCHIVE_NAME}"

echo "→ target       : $TARGET"
echo "→ krfiles ver  : $KRFILES_VERSION"
echo "→ archive      : $ARCHIVE_URL"
echo "→ destination  : $DEST_DIR"

mkdir -p "$DEST_DIR"
TMP_ARCHIVE="$(mktemp -t krfiles-native.XXXXXX.tar.gz)"
trap 'rm -f "$TMP_ARCHIVE"' EXIT

curl -fsSL -o "$TMP_ARCHIVE" "$ARCHIVE_URL"
tar xzf "$TMP_ARCHIVE" -C "$DEST_DIR"

# Sanity-check that the archive brought the three files we need to compile
# against it: the shared library, the generated C header, and the shim that
# flattens the Kotlin/Native vtable (added in krfiles v0.1.1).
missing=()
shopt -s nullglob
libs=("$DEST_DIR"/libkrfiles.so "$DEST_DIR"/libkrfiles.dylib)
if [[ ${#libs[@]} -eq 0 ]]; then
    missing+=("libkrfiles.{so,dylib}")
fi
shopt -u nullglob
[[ -f "$DEST_DIR/libkrfiles_api.h" ]] || missing+=("libkrfiles_api.h")
[[ -f "$DEST_DIR/krfiles_shim.c"   ]] || missing+=("krfiles_shim.c")

if [[ ${#missing[@]} -gt 0 ]]; then
    echo "Error: archive did not contain expected files: ${missing[*]}" >&2
    echo "       v0.1.1+ should bundle krfiles_shim.c alongside the .so/.dylib" >&2
    exit 1
fi

echo "✓ krfiles $KRFILES_VERSION native artifacts ready for $TARGET"
