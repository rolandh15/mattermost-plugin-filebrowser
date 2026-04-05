// Package command routes `/filebrowser …` subcommands to their handlers.
//
// The Router is deliberately framework-agnostic: it takes a raw command
// string and returns a Response, so unit tests can drive it without building
// Mattermost model values. The outer plugin wraps Response into a
// *model.CommandResponse before handing it to the server.
package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/rolandh15/mattermost-plugin-filebrowser/server/fb"
)

// TokenStore persists per-user Filebrowser session tokens. The plugin backs
// this with Mattermost's KVStore; tests use an in-memory implementation.
type TokenStore interface {
	GetToken(ctx context.Context, mmUserID string) (string, error)
	SetToken(ctx context.Context, mmUserID, token string) error
	ClearToken(ctx context.Context, mmUserID string) error
}

// Response is a framework-neutral command reply. The plugin translates it
// into a *model.CommandResponse (ephemeral, with Text).
type Response struct {
	Text string
}

// Router dispatches the second word of a slash command ("browse" in
// "/filebrowser browse /reports") to the corresponding handler.
type Router struct {
	client fb.Client
	tokens TokenStore
}

// New constructs a Router that talks to the given Filebrowser client and
// persists per-user tokens via the given store.
func New(client fb.Client, tokens TokenStore) *Router {
	return &Router{client: client, tokens: tokens}
}

// Handle parses the raw command string (e.g. "/filebrowser browse /reports")
// and dispatches it. The first token must be "/filebrowser"; anything else
// is caller error and panics — the plugin only routes its own trigger here.
func (r *Router) Handle(ctx context.Context, mmUserID, raw string) (*Response, error) {
	fields := strings.Fields(raw)
	if len(fields) == 0 || fields[0] != "/filebrowser" {
		return nil, fmt.Errorf("router: unexpected command prefix in %q", raw)
	}

	sub := ""
	var args []string
	if len(fields) > 1 {
		sub = fields[1]
	}
	if len(fields) > 2 {
		args = fields[2:]
	}

	switch strings.ToLower(sub) {
	case "", "help":
		return helpResponse(), nil
	case "disconnect":
		return r.handleDisconnect(ctx, mmUserID)
	default:
		_ = args // handlers land here in follow-up MRs
		return unknownResponse(sub), nil
	}
}

func helpResponse() *Response {
	return &Response{Text: "**Filebrowser** — available subcommands:\n" +
		"• `/filebrowser connect` — link your Filebrowser account (one-time)\n" +
		"• `/filebrowser browse [path]` — list a directory\n" +
		"• `/filebrowser save <path>` — upload the file you just attached in this channel\n" +
		"• `/filebrowser share <path>` — get a shareable link for a file\n" +
		"• `/filebrowser search <query>` — search across Filebrowser\n" +
		"• `/filebrowser disconnect` — forget your stored credentials\n" +
		"• `/filebrowser help` — show this message"}
}

func unknownResponse(sub string) *Response {
	return &Response{Text: fmt.Sprintf("Unknown subcommand `%s`. Try `/filebrowser help` for the full list.", sub)}
}

func (r *Router) handleDisconnect(ctx context.Context, mmUserID string) (*Response, error) {
	if err := r.tokens.ClearToken(ctx, mmUserID); err != nil {
		return nil, fmt.Errorf("disconnect: failed to clear token: %w", err)
	}
	return &Response{Text: "You have been disconnected from Filebrowser. Run `/filebrowser connect` to link again."}, nil
}
