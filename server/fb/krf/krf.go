// Package krf wraps the krfiles Kotlin/Native shared library and exposes a
// fb.Client implementation backed by it.
//
// The package is split across build tags so that the unit-test suite can
// build and run without cgo or a fetched libkrfiles.{so,dylib}:
//
//   - krf.go           — no build tags. Defines shared errors.
//   - krf_cgo.go       — //go:build cgo. The real implementation that calls
//     into libkrfiles via a tiny C shim bridge.
//   - krf_stub.go      — //go:build !cgo. A drop-in that compiles cleanly
//     with no native dependencies and returns ErrCgoDisabled from every
//     Client method so that tests using fb.Fake continue to run under
//     CGO_ENABLED=0 without linking against the shared library.
//
// Callers depend on the exported Client/New symbols — one build tag supplies
// the concrete type, the other supplies the stub, and either way the plugin
// builds and runs its unit tests. Only release builds are expected to have
// cgo enabled, native artifacts fetched via build/fetch-krfiles.sh, and the
// shared library shipped alongside the plugin binary in the bundle with
// rpath=$ORIGIN (Linux) / @loader_path (Darwin) so the dynamic linker finds
// it at runtime.
//
// The shim uses a single global client with no handle parameter (see
// NativeExports.kt in krfiles), so every public method here serialises
// against a per-Client mutex. That is a hard cap on concurrency and the
// main known scalability limit — track krfiles issue "per-handle API" for
// the long-term fix.
package krf

import "errors"

// ErrCgoDisabled is returned from every Client method when the plugin was
// built with CGO_ENABLED=0 (for example during unit-test runs). Callers
// check errors.Is(err, ErrCgoDisabled) to distinguish "krf is not available
// in this build" from other failure modes.
var ErrCgoDisabled = errors.New("krf: plugin was built without cgo; krfiles shared library is unavailable")
