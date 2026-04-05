#!/usr/bin/env bash
# fetch-krfiles.sh — download the krfiles native release archive for a given
# OS/arch, extract it into server/fb/krf/native/ (flat, no target subdir), and
# verify the shim + header + shared library all landed.
#
# Usage: ./build/fetch-krfiles.sh <target>
#   target ∈ {linux-amd64, linux-arm64, darwin-amd64, darwin-arm64}
#
# The destination is deliberately flat because the Go cgo directives in
# server/fb/krf/krf_cgo.go hard-code the include/lib path to
# ${SRCDIR}/native — one target per checkout. Re-run this script when you
# switch host targets locally; CI fetches the target matching each build job.
#
# Pinning: the version is hard-coded below so that CI cannot silently drift to
# a newer krfiles release. Bump it alongside a test that verifies the plugin
# still builds against the new artifacts.

set -euo pipefail

KRFILES_VERSION="0.2.0"

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
DEST_DIR="$REPO_ROOT/server/fb/krf/native"
ARCHIVE_NAME="krfiles-native-${KRFILES_TARGET}.tar.gz"
ARCHIVE_URL="https://github.com/rolandh15/krfiles/releases/download/v${KRFILES_VERSION}/${ARCHIVE_NAME}"

echo "→ target       : $TARGET"
echo "→ krfiles ver  : $KRFILES_VERSION"
echo "→ archive      : $ARCHIVE_URL"
echo "→ destination  : $DEST_DIR"

# Clean any stale artifacts from a previous target — the layout is flat so
# files from a different arch would otherwise linger.
rm -rf "$DEST_DIR"
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

# On macOS, Kotlin/Native bakes an absolute build-host path into the dylib's
# LC_ID_DYLIB (install_name). That path points to the krfiles CI runner's
# workspace and obviously does not exist on any consumer machine, so a
# binary that links against it records the same absolute path in its load
# commands and dyld then fails to resolve the library at runtime. Rewrite
# the install_name to @rpath/libkrfiles.dylib so `-Wl,-rpath,@loader_path`
# on the consumer side makes dyld search next to the executable.
if [[ -f "$DEST_DIR/libkrfiles.dylib" ]]; then
    if ! command -v install_name_tool >/dev/null 2>&1; then
        echo "Warning: install_name_tool not found; skipping install_name rewrite." >&2
        echo "         The plugin binary will record the absolute macOS build-host" >&2
        echo "         path and fail to locate libkrfiles.dylib at runtime." >&2
    else
        install_name_tool -id @rpath/libkrfiles.dylib "$DEST_DIR/libkrfiles.dylib"
        # Re-sign the dylib: install_name_tool invalidates the ad-hoc code
        # signature that Kotlin/Native applies, and macOS refuses to load
        # unsigned dylibs on some code paths (notarised hosts, hardened
        # runtime). Re-signing ad-hoc is sufficient for local development
        # and for Mattermost plugin installs on Linux targets.
        if command -v codesign >/dev/null 2>&1; then
            codesign --force --sign - "$DEST_DIR/libkrfiles.dylib" 2>/dev/null || true
        fi
    fi
fi

echo "✓ krfiles $KRFILES_VERSION native artifacts ready for $TARGET"
