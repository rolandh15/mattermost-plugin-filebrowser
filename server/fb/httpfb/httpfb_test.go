package httpfb

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rolandh15/mattermost-plugin-filebrowser/server/fb"
)

func TestLogin_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/login", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "alice", body["username"])
		w.Write([]byte(`"tok123"`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	token, err := c.Login(context.Background(), "alice", "hunter2")
	require.NoError(t, err)
	assert.Equal(t, "tok123", token)
}

func TestLogin_BadCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := New(srv.URL)
	_, err := c.Login(context.Background(), "alice", "wrong")
	assert.ErrorIs(t, err, fb.ErrUnauthorized)
}

func TestList_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/resources/docs", r.URL.Path)
		assert.Equal(t, "mytoken", r.Header.Get("X-Auth"))
		json.NewEncoder(w).Encode(resource{
			Items: []resource{
				{Name: "readme.md", Path: "/docs/readme.md", Size: 100},
				{Name: "pics", Path: "/docs/pics", IsDir: true},
			},
		})
	}))
	defer srv.Close()

	c := New(srv.URL)
	entries, err := c.List(context.Background(), "mytoken", "/docs")
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "readme.md", entries[0].Name)
	assert.Equal(t, int64(100), entries[0].Size)
	assert.True(t, entries[1].IsDir)
}

func TestShare_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/share/reports/q4.pdf", r.URL.Path)
		json.NewEncoder(w).Encode(shareResponse{Hash: "abc123"})
	}))
	defer srv.Close()

	c := New(srv.URL)
	url, err := c.Share(context.Background(), "tok", "/reports/q4.pdf")
	require.NoError(t, err)
	assert.Equal(t, srv.URL+"/share/abc123", url)
}

func TestUpload_Success(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/resources/docs/file.txt", r.URL.Path)
		assert.Equal(t, "true", r.URL.Query().Get("override"))
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL)
	err := c.Upload(context.Background(), "tok", "/docs/file.txt", strings.NewReader("hello"))
	require.NoError(t, err)
	assert.Equal(t, "hello", gotBody)
}

func TestSearch_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/search/", r.URL.Path)
		assert.Equal(t, "report", r.URL.Query().Get("query"))
		json.NewEncoder(w).Encode([]searchResult{
			{Path: "/docs/report.pdf", Size: 1024},
		})
	}))
	defer srv.Close()

	c := New(srv.URL)
	entries, err := c.Search(context.Background(), "tok", "/", "report")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "report.pdf", entries[0].Name)
}

func TestCheckStatus_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := New(srv.URL)
	_, err := c.List(context.Background(), "expired", "/")
	assert.ErrorIs(t, err, fb.ErrUnauthorized)
}

func TestCheckStatus_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(srv.URL)
	_, err := c.List(context.Background(), "tok", "/nope")
	assert.ErrorIs(t, err, fb.ErrNotFound)
}
