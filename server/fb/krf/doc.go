// Package krf will host the production Filebrowser client: a Go cgo wrapper
// over the krfiles Kotlin Multiplatform shared library
// (https://github.com/rolandh15/krfiles).
//
// Integration plan (tracked as a v0.2.0 follow-up):
//
//  1. build/fetch-krfiles.sh downloads krfiles-native-<target>.tar.gz for the
//     current build matrix target and extracts it into native/<target>/.
//     The archive contains libkrfiles.{so,dylib}, libkrfiles_api.h and the
//     krfiles_shim.c that flattens Kotlin/Native's vtable into flat C symbols.
//  2. A build-tagged source file here imports the shim via cgo and exposes a
//     New(baseURL string) fb.Client that serialises calls behind a mutex.
//     Serialisation is required because the shim uses a single global client
//     (krfiles_create_client → nativeCreateClient) with no handle parameter.
//  3. Command handlers receive per-user tokens from the plugin KVStore and
//     call krfiles_set_token before each operation under the package mutex.
//  4. The Fake in server/fb stays as the test double so unit tests never
//     depend on cgo or a real Filebrowser.
//
// Known gaps to resolve before wiring this in:
//
//   - krfiles_shim.c has no free-string helper, so every returned C string
//     leaks for the lifetime of the process. Acceptable for a one-shot Rust
//     CLI but unacceptable for a long-running Mattermost plugin. Needs a
//     krfiles_free_string(const char*) export in krfiles ≥ 0.2.0.
//   - The shim has no share endpoint. The plugin's Share command will
//     either call the Filebrowser /api/share endpoint directly via Go's
//     net/http or wait for krfiles to grow the operation.
//   - The shim uses a single global client (one baseURL, one authenticated
//     user at a time). The plugin serialises access; longer term a per-
//     handle API in krfiles would remove that bottleneck.
package krf
