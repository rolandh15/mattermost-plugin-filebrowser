package command

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rolandh15/mattermost-plugin-filebrowser/server/fb"
)

type staticTokenStore struct{ token string }

func (s *staticTokenStore) GetToken(_ context.Context, _ string) (string, error) {
	return s.token, nil
}
func (s *staticTokenStore) SetToken(_ context.Context, _, _ string) error { return nil }
func (s *staticTokenStore) ClearToken(_ context.Context, _ string) error  { return nil }

type fakeFileGetter struct {
	name    string
	content []byte
}

func (f *fakeFileGetter) GetRecentFile(_ string) (string, io.ReadCloser, error) {
	if f == nil || f.name == "" {
		return "", nil, ErrNoFile
	}
	return f.name, io.NopCloser(bytes.NewReader(f.content)), nil
}

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

func newTestRouter(t *testing.T) (*Router, *fb.Fake) {
	t.Helper()
	fake := fb.NewFake()
	fake.AddUser("u", "p")
	tok, err := fake.Login(context.Background(), "u", "p")
	require.NoError(t, err)
	return New(fake, &staticTokenStore{token: tok}, &fakeFileGetter{}), fake
}

func TestRouter_Help(t *testing.T) {
	r, _ := newTestRouter(t)
	resp, err := r.Handle(context.Background(), "U1", "CH1", "/filebrowser help")
	require.NoError(t, err)
	text := strings.ToLower(resp.Text)
	assert.Contains(t, text, "connect")
	assert.Contains(t, text, "ls")
	assert.Contains(t, text, "share")
	assert.Contains(t, text, "search")
	assert.Contains(t, text, "upload")
	assert.Contains(t, text, "disconnect")
}

func TestRouter_NoSubcommandShowsHelp(t *testing.T) {
	r, _ := newTestRouter(t)
	resp, err := r.Handle(context.Background(), "U1", "CH1", "/filebrowser")
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(resp.Text), "connect")
}

func TestRouter_UnknownSubcommand(t *testing.T) {
	r, _ := newTestRouter(t)
	resp, err := r.Handle(context.Background(), "U1", "CH1", "/filebrowser nonsense")
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(resp.Text), "unknown")
}

func TestRouter_Disconnect(t *testing.T) {
	r, _ := newTestRouter(t)
	resp, err := r.Handle(context.Background(), "U1", "CH1", "/filebrowser disconnect")
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(resp.Text), "disconnect")
}

func TestRouter_ConnectSuccessStoresToken(t *testing.T) {
	fake := fb.NewFake()
	fake.AddUser("alice", "hunter2")
	store := newRecordingTokenStore()
	r := New(fake, store, &fakeFileGetter{})

	resp, err := r.Handle(context.Background(), "U1", "CH1", "/filebrowser connect alice hunter2")
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(resp.Text), "connected")
	assert.NotEmpty(t, store.tokens["U1"])
}

func TestRouter_ConnectMissingCredentials(t *testing.T) {
	fake := fb.NewFake()
	r := New(fake, newRecordingTokenStore(), &fakeFileGetter{})
	resp, err := r.Handle(context.Background(), "U1", "CH1", "/filebrowser connect")
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(resp.Text), "usage")
}

func TestRouter_ConnectBadCredentials(t *testing.T) {
	fake := fb.NewFake()
	fake.AddUser("alice", "hunter2")
	store := newRecordingTokenStore()
	r := New(fake, store, &fakeFileGetter{})
	resp, err := r.Handle(context.Background(), "U1", "CH1", "/filebrowser connect alice wrong")
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(resp.Text), "login failed")
	assert.Empty(t, store.tokens["U1"])
}

func TestRouter_ShareHappyPath(t *testing.T) {
	fake := fb.NewFake()
	fake.AddUser("alice", "hunter2")
	fake.Seed("/reports/q4.pdf", []byte("pdf"))
	token, err := fake.Login(context.Background(), "alice", "hunter2")
	require.NoError(t, err)
	store := newRecordingTokenStore()
	require.NoError(t, store.SetToken(context.Background(), "U1", token))

	r := New(fake, store, &fakeFileGetter{})
	resp, err := r.Handle(context.Background(), "U1", "CH1", "/filebrowser share /reports/q4.pdf")
	require.NoError(t, err)
	assert.Contains(t, resp.Text, "https://fake.filebrowser.invalid/share/reports/q4.pdf")
}

func TestRouter_ShareNotConnected(t *testing.T) {
	r := New(fb.NewFake(), newRecordingTokenStore(), &fakeFileGetter{})
	resp, err := r.Handle(context.Background(), "U1", "CH1", "/filebrowser share /x")
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(resp.Text), "not connected")
}

func TestRouter_ShareMissingPath(t *testing.T) {
	r := New(fb.NewFake(), newRecordingTokenStore(), &fakeFileGetter{})
	resp, err := r.Handle(context.Background(), "U1", "CH1", "/filebrowser share")
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(resp.Text), "usage")
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

	r := New(fake, store, &fakeFileGetter{})
	resp, err := r.Handle(context.Background(), "U1", "CH1", "/filebrowser ls /docs")
	require.NoError(t, err)
	assert.Contains(t, resp.Text, "readme.md")
	assert.Contains(t, resp.Text, "notes.txt")
	assert.Contains(t, resp.Text, "2 items")
}

func TestRouter_BrowseDefaultsToRoot(t *testing.T) {
	r, _ := newTestRouter(t)
	resp, err := r.Handle(context.Background(), "U1", "CH1", "/filebrowser ls")
	require.NoError(t, err)
	assert.Contains(t, resp.Text, "/")
}

func TestRouter_SearchHappyPath(t *testing.T) {
	fake := fb.NewFake()
	fake.AddUser("alice", "hunter2")
	fake.Seed("/reports/q4.pdf", []byte("data"))
	token, err := fake.Login(context.Background(), "alice", "hunter2")
	require.NoError(t, err)
	store := newRecordingTokenStore()
	require.NoError(t, store.SetToken(context.Background(), "U1", token))

	r := New(fake, store, &fakeFileGetter{})
	resp, err := r.Handle(context.Background(), "U1", "CH1", "/filebrowser search q4")
	require.NoError(t, err)
	assert.Contains(t, resp.Text, "q4.pdf")
}

func TestRouter_SearchNoArgs(t *testing.T) {
	r, _ := newTestRouter(t)
	resp, err := r.Handle(context.Background(), "U1", "CH1", "/filebrowser search")
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(resp.Text), "usage")
}

func TestRouter_UploadHappyPath(t *testing.T) {
	fake := fb.NewFake()
	fake.AddUser("alice", "hunter2")
	token, err := fake.Login(context.Background(), "alice", "hunter2")
	require.NoError(t, err)
	store := newRecordingTokenStore()
	require.NoError(t, store.SetToken(context.Background(), "U1", token))

	files := &fakeFileGetter{name: "report.pdf", content: []byte("pdf-data")}
	r := New(fake, store, files)
	resp, err := r.Handle(context.Background(), "U1", "CH1", "/filebrowser upload /docs/report.pdf")
	require.NoError(t, err)
	assert.Contains(t, resp.Text, "Uploaded")
	assert.Contains(t, resp.Text, "/docs/report.pdf")
}

func TestRouter_UploadNoRecentFile(t *testing.T) {
	r, _ := newTestRouter(t)
	resp, err := r.Handle(context.Background(), "U1", "CH1", "/filebrowser upload /some/path")
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(resp.Text), "no recent file")
}

func TestRouter_UploadDefaultPath(t *testing.T) {
	fake := fb.NewFake()
	fake.AddUser("alice", "hunter2")
	token, err := fake.Login(context.Background(), "alice", "hunter2")
	require.NoError(t, err)
	store := newRecordingTokenStore()
	require.NoError(t, store.SetToken(context.Background(), "U1", token))

	files := &fakeFileGetter{name: "photo.jpg", content: []byte("jpg")}
	r := New(fake, store, files)
	resp, err := r.Handle(context.Background(), "U1", "CH1", "/filebrowser upload")
	require.NoError(t, err)
	assert.Contains(t, resp.Text, "/photo.jpg")
}

func TestRouter_UploadDirectoryPath(t *testing.T) {
	fake := fb.NewFake()
	fake.AddUser("alice", "hunter2")
	token, err := fake.Login(context.Background(), "alice", "hunter2")
	require.NoError(t, err)
	store := newRecordingTokenStore()
	require.NoError(t, store.SetToken(context.Background(), "U1", token))

	files := &fakeFileGetter{name: "photo.jpg", content: []byte("jpg")}
	r := New(fake, store, files)
	resp, err := r.Handle(context.Background(), "U1", "CH1", "/filebrowser upload /photos/")
	require.NoError(t, err)
	assert.Contains(t, resp.Text, "/photos/photo.jpg")
}
