//go:build !cgo

// This file supplies a Client type that satisfies fb.Client but returns
// ErrCgoDisabled from every method. It exists so that `go test ./...` and
// `go vet ./...` run cleanly on a workstation (or CI job) that has no
// libkrfiles.{so,dylib} on disk and cgo turned off. Unit tests that need to
// exercise real command-handler logic build a command.Router against
// fb.Fake instead.

package krf

import (
	"context"
	"io"

	"github.com/rolandh15/mattermost-plugin-filebrowser/server/fb"
)

// Client is the non-cgo stand-in for the krfiles-backed client. Every
// method returns ErrCgoDisabled. The zero value is not usable — always
// construct via New.
type Client struct {
	baseURL string
}

// Ensure the stub still satisfies fb.Client at compile time so any drift
// between the interface and this package gets caught by `go build`.
var _ fb.Client = (*Client)(nil)

// New returns a stub Client. It never fails because nothing is wired up;
// the failure surfaces when a method is called.
func New(baseURL string) *Client {
	return &Client{baseURL: baseURL}
}

// BaseURL returns the base URL the Client was constructed with. Useful for
// constructing share URLs on the caller side if the caller built a Client
// for configuration validation without actually intending to call into it.
func (c *Client) BaseURL() string { return c.baseURL }

func (c *Client) Login(context.Context, string, string) (string, error) {
	return "", ErrCgoDisabled
}

func (c *Client) List(context.Context, string, string) ([]fb.Entry, error) {
	return nil, ErrCgoDisabled
}

func (c *Client) Share(context.Context, string, string) (string, error) {
	return "", ErrCgoDisabled
}

func (c *Client) Upload(context.Context, string, string, io.Reader) error {
	return ErrCgoDisabled
}

func (c *Client) Search(context.Context, string, string, string) ([]fb.Entry, error) {
	return nil, ErrCgoDisabled
}
