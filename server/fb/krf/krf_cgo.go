//go:build cgo

// This file is the real krfiles-backed fb.Client. It only compiles when
// cgo is enabled and `build/fetch-krfiles.sh <target>` has populated
// server/fb/krf/native/ with libkrfiles.{so,dylib}, libkrfiles_api.h and
// krfiles_shim.c — release builds do both steps automatically; unit-test
// runs build the stub in krf_stub.go instead.
//
// Concurrency: every exported Client method acquires `c.mu` before touching
// the shim and releases it afterwards. That is necessary because the shim
// uses a single global client with no handle parameter (see NativeExports.kt
// in krfiles — `globalClient` is the shared state). Two users calling
// /filebrowser commands at the same time therefore serialise through this
// mutex; the alternative is letting their tokens clobber each other inside
// the shim. Track the "per-handle API" follow-up in krfiles for when this
// cap starts to bite.
//
// Memory: every `*C.char` returned from the shim was allocated by Kotlin's
// DisposeString-backed heap. Each such pointer is copied into a Go string
// and then immediately released via krfiles_free_string — if you add a new
// method here, keep that discipline or the plugin will leak on every call.

package krf

/*
#cgo CFLAGS: -I${SRCDIR}/native
// On Linux ARM64, libkrfiles.so uses outline LSE atomics whose symbols
// (__aarch64_ldadd8_acq_rel etc.) live in libgcc_s.so.1 with default
// visibility at runtime. The default BFD linker also scans gcc's static
// libgcc.a, finds the same symbols with STV_HIDDEN visibility, and
// refuses to let the DSO reference them ("hidden symbol referenced by
// DSO"). The gold linker (-fuse-ld=gold) does not have this bug — it
// correctly leaves DSO symbol resolution to the dynamic linker.
// -fuse-ld=gold is passed via CGO_LDFLAGS + CGO_LDFLAGS_ALLOW in CI
// because cgo's built-in security filter rejects it from #cgo directives.
#cgo linux LDFLAGS: -L${SRCDIR}/native -lkrfiles -Wl,-rpath,\$ORIGIN
#cgo darwin LDFLAGS: -L${SRCDIR}/native -lkrfiles -Wl,-rpath,@loader_path

#include <stdlib.h>
#include "libkrfiles_api.h"

// Forward-declarations for the flat C entry points provided by the shim.
// These match the prototypes in native/krfiles_shim.c exactly.
extern void        krfiles_create_client(const char* base_url);
extern void        krfiles_destroy_client(void);
extern const char* krfiles_get_last_error(void);
extern void        krfiles_free_string(const char* s);

extern const char* krfiles_login(const char* username, const char* password);
extern int         krfiles_set_token(const char* token);

extern const char* krfiles_list_directory(const char* path);
extern const char* krfiles_search(const char* query, const char* path);
extern const char* krfiles_create_share(const char* path, const char* password, const char* expires, const char* unit);

extern int         krfiles_upload_from_file(const char* remote_path, const char* local_path, int override_);
*/
import "C"

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/rolandh15/mattermost-plugin-filebrowser/server/fb"
)

// Client is the krfiles-backed fb.Client. Construct with New; its methods
// are safe for concurrent use but serialise internally on c.mu.
type Client struct {
	baseURL string
	mu      sync.Mutex
}

// Compile-time assertion that Client satisfies fb.Client.
var _ fb.Client = (*Client)(nil)

// New builds a Client bound to the given Filebrowser base URL and issues
// the one-time `krfiles_create_client` call that initialises the shim's
// global state. Calling New twice with different base URLs is undefined —
// the plugin only constructs one Client per activation.
func New(baseURL string) *Client {
	c := &Client{baseURL: strings.TrimRight(baseURL, "/")}
	curl := C.CString(c.baseURL)
	defer C.free(unsafe.Pointer(curl))
	C.krfiles_create_client(curl)
	return c
}

// BaseURL returns the trimmed base URL this Client was constructed with.
// Used by command handlers that need to build share URLs locally.
func (c *Client) BaseURL() string { return c.baseURL }

// lastError drains the shim's last-error slot and returns it as a Go error.
// The returned pointer is freed unconditionally. Callers should only invoke
// this after the shim signalled failure (NULL return or false bool).
func lastError(fallback string) error {
	ptr := C.krfiles_get_last_error()
	if ptr == nil {
		return errors.New(fallback)
	}
	msg := C.GoString(ptr)
	C.krfiles_free_string(ptr)
	if msg == "" {
		return errors.New(fallback)
	}
	return fmt.Errorf("krf: %s", msg)
}

// takeString copies the bytes at ptr into a Go string and releases the
// Kotlin/Native allocation. Passing NULL returns ("", false) so callers
// can distinguish a successful empty string (not meaningful for any krfiles
// endpoint today, but keeps the helper correct) from an error case.
func takeString(ptr *C.char) (string, bool) {
	if ptr == nil {
		return "", false
	}
	s := C.GoString(ptr)
	C.krfiles_free_string(ptr)
	return s, true
}

// setToken wires the given token into the shim's global client. Must be
// called while holding c.mu, immediately before the operation that needs
// authenticated access.
func (c *Client) setToken(token string) error {
	ct := C.CString(token)
	defer C.free(unsafe.Pointer(ct))
	if C.krfiles_set_token(ct) == 0 {
		return lastError("krfiles_set_token failed")
	}
	return nil
}

// Login exchanges username/password for an opaque session token. The token
// is persisted by the plugin via TokenStore and passed back to subsequent
// method calls.
func (c *Client) Login(_ context.Context, username, password string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	cu := C.CString(username)
	defer C.free(unsafe.Pointer(cu))
	cp := C.CString(password)
	defer C.free(unsafe.Pointer(cp))

	token, ok := takeString(C.krfiles_login(cu, cp))
	if !ok {
		return "", fmt.Errorf("%w: %s", fb.ErrUnauthorized, lastError("login failed").Error())
	}
	return token, nil
}

// resource mirrors the subset of Filebrowser's `Resource` struct that the
// plugin cares about. Unused fields are ignored via the struct omission.
type resource struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	Size     float64    `json:"size"`
	IsDir    bool       `json:"isDir"`
	Modified string     `json:"modified"`
	Items    []resource `json:"items"`
}

// toEntry converts a single Filebrowser resource into an fb.Entry. The
// `modified` timestamp is parsed best-effort; a parse failure yields the
// zero time rather than an error because fb.Entry's contract explicitly
// allows a zero ModTime.
func (r *resource) toEntry() fb.Entry {
	e := fb.Entry{
		Name:  r.Name,
		Path:  r.Path,
		IsDir: r.IsDir,
		Size:  int64(r.Size),
	}
	if r.Modified != "" {
		if t, err := time.Parse(time.RFC3339Nano, r.Modified); err == nil {
			e.ModTime = t
		}
	}
	return e
}

// List returns the direct children of a directory. The incoming token must
// be the one returned by a previous Login call.
func (c *Client) List(_ context.Context, token, dir string) ([]fb.Entry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.setToken(token); err != nil {
		return nil, err
	}

	cdir := C.CString(dir)
	defer C.free(unsafe.Pointer(cdir))

	raw, ok := takeString(C.krfiles_list_directory(cdir))
	if !ok {
		return nil, lastError("list failed")
	}

	var r resource
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return nil, fmt.Errorf("krf: parse list response: %w", err)
	}
	out := make([]fb.Entry, 0, len(r.Items))
	for i := range r.Items {
		out = append(out, r.Items[i].toEntry())
	}
	return out, nil
}

// shareResponse mirrors the krfiles `Share` data class (which in turn
// mirrors Filebrowser's `share.Link`). Only `hash` is strictly needed to
// construct the public URL; the rest are decoded for logging/debug.
type shareResponse struct {
	Hash   string  `json:"hash"`
	Path   string  `json:"path"`
	UserID int     `json:"userID"`
	Expire float64 `json:"expire"`
}

// Share creates a permanent, unprotected share link for the given path and
// returns the public URL a user can hit from a browser. Password-protected
// and expiring shares are follow-up work — the slash command UI needs to
// ask for those options first.
func (c *Client) Share(_ context.Context, token, path string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.setToken(token); err != nil {
		return "", err
	}

	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	empty := C.CString("")
	defer C.free(unsafe.Pointer(empty))
	unit := C.CString("hours")
	defer C.free(unsafe.Pointer(unit))

	raw, ok := takeString(C.krfiles_create_share(cpath, empty, empty, unit))
	if !ok {
		return "", lastError("share failed")
	}

	var sr shareResponse
	if err := json.Unmarshal([]byte(raw), &sr); err != nil {
		return "", fmt.Errorf("krf: parse share response: %w", err)
	}
	if sr.Hash == "" {
		return "", errors.New("krf: share response missing hash")
	}
	return c.baseURL + "/share/" + sr.Hash, nil
}

// Upload streams body into a tempfile and then asks krfiles to upload it.
// The shim's upload entry point is file-backed (nativeUploadFromFile) so a
// scratch file is unavoidable on this code path — the tempfile is removed
// before the method returns regardless of outcome.
func (c *Client) Upload(_ context.Context, token, path string, body io.Reader) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.setToken(token); err != nil {
		return err
	}

	tmp, err := os.CreateTemp("", "krf-upload-*")
	if err != nil {
		return fmt.Errorf("krf: create tempfile: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmp, body); err != nil {
		tmp.Close()
		return fmt.Errorf("krf: stage upload body: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("krf: close tempfile: %w", err)
	}

	// Ensure the temp path is absolute — krfiles' nativeUploadFromFile
	// opens it via fopen from whatever Kotlin/Native's working directory
	// happens to be, which is not guaranteed to match ours.
	absTmp, err := filepath.Abs(tmpPath)
	if err != nil {
		return fmt.Errorf("krf: resolve tempfile: %w", err)
	}

	cremote := C.CString(path)
	defer C.free(unsafe.Pointer(cremote))
	clocal := C.CString(absTmp)
	defer C.free(unsafe.Pointer(clocal))

	if C.krfiles_upload_from_file(cremote, clocal, C.int(1)) == 0 {
		return lastError("upload failed")
	}
	return nil
}

// searchResult mirrors krfiles' `SearchResult` (path + dir flag).
type searchResult struct {
	Path string `json:"path"`
	Dir  bool   `json:"dir"`
}

// Search returns entries matching query under root. `root` currently is
// passed through to krfiles; today that means Filebrowser searches from
// the root regardless of the value, but the parameter is retained so the
// plugin can switch to scoped searches when krfiles grows the option.
func (c *Client) Search(_ context.Context, token, root, query string) ([]fb.Entry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.setToken(token); err != nil {
		return nil, err
	}

	cquery := C.CString(query)
	defer C.free(unsafe.Pointer(cquery))
	croot := C.CString(root)
	defer C.free(unsafe.Pointer(croot))

	raw, ok := takeString(C.krfiles_search(cquery, croot))
	if !ok {
		return nil, lastError("search failed")
	}

	var results []searchResult
	if err := json.Unmarshal([]byte(raw), &results); err != nil {
		return nil, fmt.Errorf("krf: parse search response: %w", err)
	}
	out := make([]fb.Entry, 0, len(results))
	for _, r := range results {
		out = append(out, fb.Entry{
			Name:  filepath.Base(r.Path),
			Path:  r.Path,
			IsDir: r.Dir,
		})
	}
	return out, nil
}
