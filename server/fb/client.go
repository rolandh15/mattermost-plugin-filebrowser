// Package fb is the plugin's abstraction over a Filebrowser instance.
//
// The plugin's command handlers depend only on the Client interface defined
// here; the production implementation (httpfb) talks to Filebrowser's REST
// API over plain HTTP. Tests use the in-memory Fake in this package so they
// never touch the network or a real Filebrowser.
package fb

import (
	"context"
	"errors"
	"io"
	"time"
)

// Sentinel errors returned by Client implementations. Callers use errors.Is
// against these to react without inspecting vendor-specific error strings.
var (
	// ErrUnauthorized is returned when the supplied token is invalid or
	// expired, or when credentials do not match a known user.
	ErrUnauthorized = errors.New("filebrowser: unauthorized")

	// ErrNotFound is returned when the requested path does not exist.
	ErrNotFound = errors.New("filebrowser: not found")
)

// Entry describes a single resource returned by List or Search.
type Entry struct {
	// Name is the final path component ("report.pdf", "2025-q4", …).
	Name string
	// Path is the absolute path from the Filebrowser root. Always begins with
	// a slash and uses forward slashes on every platform.
	Path string
	// IsDir reports whether the entry is a directory.
	IsDir bool
	// Size in bytes. Zero for directories.
	Size int64
	// ModTime is the last-modified timestamp as reported by Filebrowser. The
	// zero value is acceptable when a backend does not track it.
	ModTime time.Time
}

// Client is the minimum surface the plugin needs from a Filebrowser-like
// backend. Every method is context-aware so callers can propagate timeouts
// and cancellation from the Mattermost request handler.
//
// Implementations must be safe for concurrent use.
type Client interface {
	// Login exchanges a username and password for a session token. The token
	// is opaque; callers must treat it as a string and pass it back to
	// subsequent calls.
	Login(ctx context.Context, username, password string) (string, error)

	// List returns the direct children of the directory at path. The path is
	// absolute ("/", "/reports", …) and uses forward slashes.
	List(ctx context.Context, token, path string) ([]Entry, error)

	// Share returns a publicly reachable URL for the file at path. Short-
	// lived share links are fine; the plugin never persists them.
	Share(ctx context.Context, token, path string) (string, error)

	// Upload writes body to the file at path. If the file already exists it
	// is overwritten. Any missing parent directories are created.
	Upload(ctx context.Context, token, path string, body io.Reader) error

	// Search returns entries whose Name or Path matches query, restricted to
	// the sub-tree rooted at root.
	Search(ctx context.Context, token, root, query string) ([]Entry, error)
}
