#!/bin/sh
# fix-lse.sh — ARM64 LSE atomics visibility workaround
#
# On ARM64, libkrfiles.so uses outline LSE atomics whose symbols live in
# libgcc_s.so.1 with default visibility at runtime.  gcc's static libgcc.a
# marks them STV_HIDDEN, so the linker refuses to let a DSO reference them.
#
# Fix: extract the LSE atomic objects from libgcc.a and build a small
# shared library (libgcc_lse.so) with a version script that forces
# default visibility.  Linking -lgcc_lse before the implicit -lgcc
# provides a visible definition first.
#
# On amd64 this script is a no-op.

set -e

ARCH="$(dpkg --print-architecture)"
if [ "$ARCH" != "arm64" ]; then
    echo "fix-lse: $ARCH — no LSE fixup needed"
    exit 0
fi

GCC_DIR="$(dirname "$(gcc -print-libgcc-file-name)")"
WORK="$(mktemp -d)"
cd "$WORK"

# Extract all objects from libgcc.a
ar x "$GCC_DIR/libgcc.a"

# Find objects with hidden __aarch64_ symbols
LSE_OBJS=""
for obj in *.o; do
    if readelf -sW "$obj" 2>/dev/null | grep -q 'HIDDEN.*__aarch64_'; then
        LSE_OBJS="$LSE_OBJS $obj"
    fi
done

if [ -z "$LSE_OBJS" ]; then
    echo "fix-lse: no hidden __aarch64_ symbols found — nothing to do"
    cd / && rm -rf "$WORK"
    exit 0
fi

# Version script forces all __aarch64_ symbols to default visibility
printf '{ global: __aarch64_*; local: *; };\n' > lse.map

# Build a shared library from the LSE objects
# shellcheck disable=SC2086
gcc -shared -nostdlib -o "$GCC_DIR/libgcc_lse.so" \
    -Wl,--version-script=lse.map $LSE_OBJS

cd / && rm -rf "$WORK"

COUNT="$(readelf -sW "$GCC_DIR/libgcc_lse.so" | grep -c 'DEFAULT.*__aarch64_' || true)"
echo "fix-lse: libgcc_lse.so built in $GCC_DIR ($COUNT LSE symbols with DEFAULT visibility)"
