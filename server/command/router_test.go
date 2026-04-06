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
	assert.Contains(t, text, "ls")
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

// recordingTokenStore tracks SetToken calls so connect tests can assert
// that the returned token was actually persisted. It is in-memory only.
type recordingTokenStore struct {
	tokens map[string]string
}

func newRecordingTokenStore() *recordingTokenStore {
	return &recordingTokenStore{tokens: make(map[string]string)}
}

func (s *recordingTokenStore) GetToken(_ context.Context, userID string) (string, error) {
	t, ok := s.tokens[userID]
	if !ok {
		return "", ErrNotConnected
	}
	return t, nil
}

func (s *recordingTokenStore) SetToken(_ context.Context, userID, token string) error {
	s.tokens[userID] = token
	return nil
}

func (s *recordingTokenStore) ClearToken(_ context.Context, userID string) error {
	delete(s.tokens, userID)
	return nil
}

func TestRouter_ConnectSuccessStoresToken(t *testing.T) {
	fake := fb.NewFake()
	fake.AddUser("alice", "hunter2")
	store := newRecordingTokenStore()
	r := New(fake, store)

	resp, err := r.Handle(context.Background(), "U1", "/filebrowser connect alice hunter2")
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(resp.Text), "connected")
	// The Fake's login returns a deterministic hex token; all we care about
	// is that *something* non-empty was persisted for the MM user.
	assert.NotEmpty(t, store.tokens["U1"])
}

func TestRouter_ConnectMissingCredentialsShowsUsage(t *testing.T) {
	fake := fb.NewFake()
	r := New(fake, newRecordingTokenStore())

	resp, err := r.Handle(context.Background(), "U1", "/filebrowser connect")
	require.NoError(t, err) // usage message is not an error
	assert.Contains(t, strings.ToLower(resp.Text), "usage")
}

func TestRouter_ConnectBadCredentialsSurfacesFriendlyMessage(t *testing.T) {
	fake := fb.NewFake()
	fake.AddUser("alice", "hunter2")
	store := newRecordingTokenStore()
	r := New(fake, store)

	resp, err := r.Handle(context.Background(), "U1", "/filebrowser connect alice wrong")
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(resp.Text), "login failed")
	assert.Empty(t, store.tokens["U1"]) // nothing should have been persisted
}

func TestRouter_ShareHappyPath(t *testing.T) {
	fake := fb.NewFake()
	fake.AddUser("alice", "hunter2")
	fake.Seed("/reports/q4.pdf", []byte("pretend this is a PDF"))
	token, err := fake.Login(context.Background(), "alice", "hunter2")
	require.NoError(t, err)

	store := newRecordingTokenStore()
	require.NoError(t, store.SetToken(context.Background(), "U1", token))

	r := New(fake, store)
	resp, err := r.Handle(context.Background(), "U1", "/filebrowser share /reports/q4.pdf")
	require.NoError(t, err)
	// Fake's Share builds a stable URL starting with "https://fake.filebrowser.invalid/share".
	assert.Contains(t, resp.Text, "https://fake.filebrowser.invalid/share/reports/q4.pdf")
}

func TestRouter_ShareNotConnectedPromptsConnect(t *testing.T) {
	fake := fb.NewFake()
	r := New(fake, newRecordingTokenStore())

	resp, err := r.Handle(context.Background(), "U1", "/filebrowser share /whatever.txt")
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(resp.Text), "not connected")
	assert.Contains(t, strings.ToLower(resp.Text), "connect")
}

func TestRouter_ShareMissingPathShowsUsage(t *testing.T) {
	fake := fb.NewFake()
	r := New(fake, newRecordingTokenStore())

	resp, err := r.Handle(context.Background(), "U1", "/filebrowser share")
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(resp.Text), "usage")
}

func TestRouter_ShareNotFoundReturnsFriendlyMessage(t *testing.T) {
	fake := fb.NewFake()
	fake.AddUser("alice", "hunter2")
	token, err := fake.Login(context.Background(), "alice", "hunter2")
	require.NoError(t, err)

	store := newRecordingTokenStore()
	require.NoError(t, store.SetToken(context.Background(), "U1", token))

	r := New(fake, store)
	resp, err := r.Handle(context.Background(), "U1", "/filebrowser share /does/not/exist.txt")
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(resp.Text), "no file found")
}

func TestRouter_UploadIsDeferred(t *testing.T) {
	r, _ := newTestRouter(t)

	for _, sub := range []string{"upload", "save"} {
		resp, err := r.Handle(context.Background(), "U1", "/filebrowser "+sub+" /some/path")
		require.NoError(t, err, "subcommand %s should not error", sub)
		assert.Contains(t, strings.ToLower(resp.Text), "not available", "subcommand %s should announce the deferral", sub)
	}
}

func TestRouter_BrowseHappyPath(t *testing.T) {
	fake := fb.NewFake()
	fake.AddUser("alice", "hunter2")
	fake.Seed("/docs/readme.md", []byte("hello"))
	fake.Seed("/docs/notes.txt", []byte("world"))
	token, err := fake.Login(context.Background(), "alice", "hunter2")
	require.NoError(t, err)

	store := newRecordingTokenStore()
	require.NoError(t, store.SetToken(context.Background(), "U1", token))

	r := New(fake, store)
	resp, err := r.Handle(context.Background(), "U1", "/filebrowser ls /docs")
	require.NoError(t, err)
	assert.Contains(t, resp.Text, "readme.md")
	assert.Contains(t, resp.Text, "notes.txt")
	assert.Contains(t, resp.Text, "2 items")
}

func TestRouter_BrowseDefaultsToRoot(t *testing.T) {
	r, _ := newTestRouter(t)

	resp, err := r.Handle(context.Background(), "U1", "/filebrowser ls")
	require.NoError(t, err)
	assert.Contains(t, resp.Text, "/")
}

func TestRouter_SearchHappyPath(t *testing.T) {
	fake := fb.NewFake()
	fake.AddUser("alice", "hunter2")
	fake.Seed("/reports/q4.pdf", []byte("data"))
	fake.Seed("/reports/q3.pdf", []byte("data"))
	token, err := fake.Login(context.Background(), "alice", "hunter2")
	require.NoError(t, err)

	store := newRecordingTokenStore()
	require.NoError(t, store.SetToken(context.Background(), "U1", token))

	r := New(fake, store)
	resp, err := r.Handle(context.Background(), "U1", "/filebrowser search q4")
	require.NoError(t, err)
	assert.Contains(t, resp.Text, "q4.pdf")
}

func TestRouter_SearchNoArgs(t *testing.T) {
	r, _ := newTestRouter(t)

	resp, err := r.Handle(context.Background(), "U1", "/filebrowser search")
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(resp.Text), "usage")
}
