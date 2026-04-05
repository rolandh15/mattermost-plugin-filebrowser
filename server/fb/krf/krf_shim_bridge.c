/*
 * krf_shim_bridge.c — build-time bridge that pulls the krfiles C shim into
 * this cgo package's compilation unit.
 *
 * The real shim source lives under native/krfiles_shim.c, where
 * build/fetch-krfiles.sh drops it alongside libkrfiles.{so,dylib} and the
 * generated libkrfiles_api.h. We do not copy the shim into this directory
 * because that would fork the file between krfiles releases; instead this
 * tiny translation unit #includes it, and cgo auto-compiles this file
 * because it sits next to the Go sources that use `import "C"`.
 *
 * The #include path is relative to this file, so the preprocessor finds
 * native/krfiles_shim.c and in turn native/libkrfiles_api.h (the shim
 * includes the header via a plain `#include "libkrfiles_api.h"`, which
 * resolves relative to its own location).
 *
 * Consumers never compile this file directly — only cgo does.
 */

#include "native/krfiles_shim.c"
