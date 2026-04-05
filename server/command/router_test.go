package command

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rolandh15/mattermost-plugin-filebrowser/server/fb"
)

// staticTokenStore returns the same token for every user. Tests that care
// about per-user isolation replace it with a recording fake.
type staticTokenStore struct{ token string }

func (s *staticTokenStore) GetToken(_ context.Context, _ string) (string, error) {
	return s.token, nil
}

func (s *staticTokenStore) SetToken(_ context.Context, _ string, _ string) error {
	return nil
}

func (s *staticTokenStore) ClearToken(_ context.Context, _ string) error {
	return nil
}

// newTestRouter wires a Router against a seeded Fake and a no-op token store.
// The "u"/"p" user is pre-registered and a token is cached for user ID "U1".
func newTestRouter(t *testing.T) (*Router, *fb.Fake) {
	t.Helper()
	fake := fb.NewFake()
	fake.AddUser("u", "p")
	tok, err := fake.Login(context.Background(), "u", "p")
	require.NoError(t, err)

	return New(fake, &staticTokenStore{token: tok}), fake
}

func TestRouter_Help(t *testing.T) {
	r, _ := newTestRouter(t)

	resp, err := r.Handle(context.Background(), "U1", "/filebrowser help")
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Help lists every subcommand the user can run so a new user learns the
	// surface without reading a wiki.
	text := strings.ToLower(resp.Text)
	assert.Contains(t, text, "connect")
	assert.Contains(t, text, "browse")
	assert.Contains(t, text, "save")
	assert.Contains(t, text, "share")
	assert.Contains(t, text, "search")
	assert.Contains(t, text, "disconnect")
}

func TestRouter_NoSubcommandShowsHelp(t *testing.T) {
	r, _ := newTestRouter(t)

	// Bare "/filebrowser" lands on help — no-arg should never be an error.
	resp, err := r.Handle(context.Background(), "U1", "/filebrowser")
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(resp.Text), "connect")
}

func TestRouter_UnknownSubcommand(t *testing.T) {
	r, _ := newTestRouter(t)

	resp, err := r.Handle(context.Background(), "U1", "/filebrowser nonsense")
	require.NoError(t, err) // unknown isn't an error — it's a friendly message
	require.NotNil(t, resp)
	assert.Contains(t, strings.ToLower(resp.Text), "unknown")
	assert.Contains(t, strings.ToLower(resp.Text), "nonsense")
	assert.Contains(t, strings.ToLower(resp.Text), "help")
}

func TestRouter_Disconnect(t *testing.T) {
	r, _ := newTestRouter(t)

	resp, err := r.Handle(context.Background(), "U1", "/filebrowser disconnect")
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(resp.Text), "disconnect")
}
