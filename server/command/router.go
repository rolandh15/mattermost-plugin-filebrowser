// Package command routes `/filebrowser …` subcommands to their handlers.
//
// The Router is deliberately framework-agnostic: it takes a raw command
// string and returns a Response, so unit tests can drive it without building
// Mattermost model values. The outer plugin wraps Response into a
// *model.CommandResponse before handing it to the server.
package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/rolandh15/mattermost-plugin-filebrowser/server/fb"
)

// TokenStore persists per-user Filebrowser session tokens. The plugin backs
// this with Mattermost's KVStore; tests use an in-memory implementation.
//
// Implementations of GetToken should return an error that wraps
// ErrNotConnected (via errors.Is) when a user has never connected or has
// disconnected — the router uses that to emit a friendly "run connect
// first" message instead of a raw error.
type TokenStore interface {
	GetToken(ctx context.Context, mmUserID string) (string, error)
	SetToken(ctx context.Context, mmUserID, token string) error
	ClearToken(ctx context.Context, mmUserID string) error
}

// ErrNotConnected is the sentinel TokenStore implementations wrap when no
// token exists for a user. It is intentionally exported from the command
// package so tests can fabricate the condition without importing the
// plugin's KVStore-backed implementation.
var ErrNotConnected = errors.New("filebrowser: not connected")

// ErrNoFile is returned by FileGetter when no recent file attachment exists.
var ErrNoFile = errors.New("filebrowser: no recent file in channel")

// FileGetter retrieves the most recent file attachment from a Mattermost
// channel. The plugin implements this against p.API; tests use a fake.
type FileGetter interface {
	GetRecentFile(channelID string) (name string, content io.ReadCloser, err error)
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
	files  FileGetter
}

// New constructs a Router that talks to the given Filebrowser client and
// persists per-user tokens via the given store.
func New(client fb.Client, tokens TokenStore, files FileGetter) *Router {
	return &Router{client: client, tokens: tokens, files: files}
}

// Handle parses the raw command string (e.g. "/filebrowser browse /reports")
// and dispatches it. channelID is needed for upload (to find recent files).
func (r *Router) Handle(ctx context.Context, mmUserID, channelID, raw string) (*Response, error) {
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
	case "connect":
		return r.handleConnect(ctx, mmUserID, args)
	case "share":
		return r.handleShare(ctx, mmUserID, args)
	case "disconnect":
		return r.handleDisconnect(ctx, mmUserID)
	case "browse", "ls":
		return r.handleBrowse(ctx, mmUserID, args)
	case "search":
		return r.handleSearch(ctx, mmUserID, args)
	case "upload", "save":
		return r.handleUpload(ctx, mmUserID, channelID, args)
	default:
		return unknownResponse(sub), nil
	}
}

func helpResponse() *Response {
	return &Response{Text: "**Filebrowser** — available subcommands:\n" +
		"• `/filebrowser connect <user> <pass>` — link your Filebrowser account\n" +
		"• `/filebrowser ls [path]` — list a directory (alias: `browse`)\n" +
		"• `/filebrowser share <path>` — get a shareable link for a file\n" +
		"• `/filebrowser search <query>` — search across Filebrowser\n" +
		"• `/filebrowser upload [path]` — upload the last file posted in this channel\n" +
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

// handleConnect takes a username and password, calls client.Login, and
// persists the returned token against the Mattermost user ID. It is the
// only command that writes to TokenStore, and the only one that accepts
// credentials — all follow-up commands read the token back out and pass
// it to the Filebrowser client.
//
// The argument form is `/filebrowser connect <username> <password>`.
// Passwords containing spaces are not supported today; the follow-up is
// to replace this with an OpenDialogRequest so Mattermost renders a
// password input that is never echoed into the channel. Until then, the
// slash command is still safer than pasting credentials into a channel
// because the server-side response is always ephemeral.
func (r *Router) handleConnect(ctx context.Context, mmUserID string, args []string) (*Response, error) {
	if len(args) < 2 {
		return &Response{Text: "Usage: `/filebrowser connect <username> <password>`. Your credentials are stored only on the server and never visible to anyone else."}, nil
	}
	username, password := args[0], args[1]

	token, err := r.client.Login(ctx, username, password)
	if err != nil {
		if errors.Is(err, fb.ErrUnauthorized) {
			return &Response{Text: "Login failed — check your username and password and try again."}, nil
		}
		return nil, fmt.Errorf("connect: login failed: %w", err)
	}

	if err := r.tokens.SetToken(ctx, mmUserID, token); err != nil {
		return nil, fmt.Errorf("connect: store token failed: %w", err)
	}

	return &Response{Text: fmt.Sprintf("Connected to Filebrowser as `%s`. You can now run `/filebrowser share <path>` to create share links.", username)}, nil
}

// handleBrowse lists the contents of a directory. Defaults to "/" if no path given.
func (r *Router) handleBrowse(ctx context.Context, mmUserID string, args []string) (*Response, error) {
	dir := "/"
	if len(args) > 0 {
		dir = args[0]
	}

	token, err := r.tokens.GetToken(ctx, mmUserID)
	if err != nil {
		if errors.Is(err, ErrNotConnected) {
			return &Response{Text: "You are not connected to Filebrowser. Run `/filebrowser connect <username> <password>` first."}, nil
		}
		return nil, fmt.Errorf("browse: read token: %w", err)
	}

	entries, err := r.client.List(ctx, token, dir)
	if err != nil {
		if errors.Is(err, fb.ErrUnauthorized) {
			return &Response{Text: "Your Filebrowser session has expired. Run `/filebrowser connect` again."}, nil
		}
		if errors.Is(err, fb.ErrNotFound) {
			return &Response{Text: fmt.Sprintf("Directory `%s` not found.", dir)}, nil
		}
		return nil, fmt.Errorf("browse: list failed: %w", err)
	}

	if len(entries) == 0 {
		return &Response{Text: fmt.Sprintf("`%s` is empty.", dir)}, nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "**%s** (%d items)\n\n", dir, len(entries))
	for _, e := range entries {
		icon := ":page_facing_up:"
		if e.IsDir {
			icon = ":file_folder:"
		}
		if e.Size > 0 {
			fmt.Fprintf(&b, "%s `%s` (%s)\n", icon, e.Name, humanSize(e.Size))
		} else {
			fmt.Fprintf(&b, "%s `%s`\n", icon, e.Name)
		}
	}
	return &Response{Text: b.String()}, nil
}

func humanSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// handleUpload grabs the most recent file attachment from the current channel
// and uploads it to the specified path on Filebrowser. If no path is given,
// uploads to "/" + the original filename.
func (r *Router) handleUpload(ctx context.Context, mmUserID, channelID string, args []string) (*Response, error) {
	token, err := r.tokens.GetToken(ctx, mmUserID)
	if err != nil {
		if errors.Is(err, ErrNotConnected) {
			return &Response{Text: "You are not connected to Filebrowser. Run `/filebrowser connect <username> <password>` first."}, nil
		}
		return nil, fmt.Errorf("upload: read token: %w", err)
	}

	name, content, err := r.files.GetRecentFile(channelID)
	if err != nil {
		if errors.Is(err, ErrNoFile) {
			return &Response{Text: "No recent file attachment found in this channel. Post a file first, then run `/filebrowser upload [path]`."}, nil
		}
		return nil, fmt.Errorf("upload: get file: %w", err)
	}
	defer content.Close()

	remotePath := "/" + name
	if len(args) > 0 {
		remotePath = args[0]
		// If path looks like a directory (ends with /), append filename
		if strings.HasSuffix(remotePath, "/") {
			remotePath += name
		}
	}

	if err := r.client.Upload(ctx, token, remotePath, content); err != nil {
		if errors.Is(err, fb.ErrUnauthorized) {
			return &Response{Text: "Your Filebrowser session has expired. Run `/filebrowser connect` again."}, nil
		}
		return nil, fmt.Errorf("upload: failed: %w", err)
	}

	return &Response{Text: fmt.Sprintf("Uploaded `%s` to `%s`.", name, remotePath)}, nil
}

// handleSearch finds entries matching a query string.
func (r *Router) handleSearch(ctx context.Context, mmUserID string, args []string) (*Response, error) {
	if len(args) < 1 {
		return &Response{Text: "Usage: `/filebrowser search <query>` — for example `/filebrowser search report`."}, nil
	}
	query := strings.Join(args, " ")

	token, err := r.tokens.GetToken(ctx, mmUserID)
	if err != nil {
		if errors.Is(err, ErrNotConnected) {
			return &Response{Text: "You are not connected to Filebrowser. Run `/filebrowser connect <username> <password>` first."}, nil
		}
		return nil, fmt.Errorf("search: read token: %w", err)
	}

	entries, err := r.client.Search(ctx, token, "/", query)
	if err != nil {
		if errors.Is(err, fb.ErrUnauthorized) {
			return &Response{Text: "Your Filebrowser session has expired. Run `/filebrowser connect` again."}, nil
		}
		return nil, fmt.Errorf("search: failed: %w", err)
	}

	if len(entries) == 0 {
		return &Response{Text: fmt.Sprintf("No results for `%s`.", query)}, nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "**Search results for** `%s` (%d matches)\n\n", query, len(entries))
	for _, e := range entries {
		icon := ":page_facing_up:"
		if e.IsDir {
			icon = ":file_folder:"
		}
		fmt.Fprintf(&b, "%s `%s`\n", icon, e.Path)
	}
	return &Response{Text: b.String()}, nil
}

// handleShare resolves the user's stored token, asks the Filebrowser
// backend for a share link, and returns the public URL back as an
// ephemeral message. The user can then copy-paste it wherever they
// like — the plugin intentionally never posts the URL into the channel
// on the user's behalf.
func (r *Router) handleShare(ctx context.Context, mmUserID string, args []string) (*Response, error) {
	if len(args) < 1 {
		return &Response{Text: "Usage: `/filebrowser share <path>` — for example `/filebrowser share /reports/q4.pdf`."}, nil
	}
	path := args[0]

	token, err := r.tokens.GetToken(ctx, mmUserID)
	if err != nil {
		if errors.Is(err, ErrNotConnected) {
			return &Response{Text: "You are not connected to Filebrowser. Run `/filebrowser connect <username> <password>` first."}, nil
		}
		return nil, fmt.Errorf("share: read token: %w", err)
	}

	url, err := r.client.Share(ctx, token, path)
	if err != nil {
		if errors.Is(err, fb.ErrUnauthorized) {
			return &Response{Text: "Your Filebrowser session has expired. Run `/filebrowser connect` again to re-link your account."}, nil
		}
		if errors.Is(err, fb.ErrNotFound) {
			return &Response{Text: fmt.Sprintf("No file found at `%s`. Double-check the path and try again.", path)}, nil
		}
		return nil, fmt.Errorf("share: create link failed: %w", err)
	}

	return &Response{Text: fmt.Sprintf("Share link for `%s`:\n\n%s", path, url)}, nil
}
