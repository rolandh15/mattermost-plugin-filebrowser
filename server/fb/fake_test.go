package fb

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFakeClient_Login(t *testing.T) {
	t.Run("returns a token for known user", func(t *testing.T) {
		c := NewFake()
		c.AddUser("roland", "hunter2")

		token, err := c.Login(context.Background(), "roland", "hunter2")
		require.NoError(t, err)
		assert.NotEmpty(t, token)
	})

	t.Run("returns stable token across calls for same user", func(t *testing.T) {
		c := NewFake()
		c.AddUser("roland", "hunter2")

		first, err := c.Login(context.Background(), "roland", "hunter2")
		require.NoError(t, err)
		second, err := c.Login(context.Background(), "roland", "hunter2")
		require.NoError(t, err)
		assert.Equal(t, first, second)
	})

	t.Run("different users get different tokens", func(t *testing.T) {
		c := NewFake()
		c.AddUser("a", "pw")
		c.AddUser("b", "pw")

		ta, _ := c.Login(context.Background(), "a", "pw")
		tb, _ := c.Login(context.Background(), "b", "pw")
		assert.NotEqual(t, ta, tb)
	})

	t.Run("returns ErrUnauthorized for wrong password", func(t *testing.T) {
		c := NewFake()
		c.AddUser("roland", "hunter2")

		_, err := c.Login(context.Background(), "roland", "nope")
		assert.ErrorIs(t, err, ErrUnauthorized)
	})

	t.Run("returns ErrUnauthorized for unknown user", func(t *testing.T) {
		c := NewFake()

		_, err := c.Login(context.Background(), "ghost", "pw")
		assert.ErrorIs(t, err, ErrUnauthorized)
	})
}

func TestFakeClient_List(t *testing.T) {
	t.Run("lists seeded entries under a directory", func(t *testing.T) {
		c := NewFake()
		c.AddUser("u", "p")
		c.Seed("/reports/summary.pdf", []byte("PDF"))
		c.Seed("/reports/2025-q4/plan.md", []byte("# plan"))

		tok, err := c.Login(context.Background(), "u", "p")
		require.NoError(t, err)

		entries, err := c.List(context.Background(), tok, "/reports")
		require.NoError(t, err)

		names := namesOf(entries)
		assert.ElementsMatch(t, []string{"summary.pdf", "2025-q4"}, names)
	})

	t.Run("marks subdirectories as IsDir", func(t *testing.T) {
		c := NewFake()
		c.AddUser("u", "p")
		c.Seed("/a/b/c.txt", []byte("c"))

		tok, _ := c.Login(context.Background(), "u", "p")

		entries, err := c.List(context.Background(), tok, "/a")
		require.NoError(t, err)
		require.Len(t, entries, 1)
		assert.Equal(t, "b", entries[0].Name)
		assert.True(t, entries[0].IsDir)
	})

	t.Run("returns ErrNotFound for unknown path", func(t *testing.T) {
		c := NewFake()
		c.AddUser("u", "p")
		tok, _ := c.Login(context.Background(), "u", "p")

		_, err := c.List(context.Background(), tok, "/does/not/exist")
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("returns ErrUnauthorized for an invalid token", func(t *testing.T) {
		c := NewFake()
		c.Seed("/x.txt", []byte("x"))

		_, err := c.List(context.Background(), "bogus", "/")
		assert.ErrorIs(t, err, ErrUnauthorized)
	})

	t.Run("dedupes direct children when nested paths share a prefix", func(t *testing.T) {
		c := NewFake()
		c.AddUser("u", "p")
		c.Seed("/d/a.txt", []byte("a"))
		c.Seed("/d/a.txt.bak", []byte("bak"))
		c.Seed("/d/sub/inner.txt", []byte("i"))

		tok, _ := c.Login(context.Background(), "u", "p")
		entries, err := c.List(context.Background(), tok, "/d")
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"a.txt", "a.txt.bak", "sub"}, namesOf(entries))
	})
}

func TestFakeClient_Share(t *testing.T) {
	t.Run("returns a URL for an existing path", func(t *testing.T) {
		c := NewFake()
		c.AddUser("u", "p")
		c.Seed("/report.pdf", []byte("pdf"))

		tok, _ := c.Login(context.Background(), "u", "p")
		url, err := c.Share(context.Background(), tok, "/report.pdf")
		require.NoError(t, err)
		assert.Contains(t, url, "/report.pdf")
	})

	t.Run("returns ErrNotFound for unknown path", func(t *testing.T) {
		c := NewFake()
		c.AddUser("u", "p")
		tok, _ := c.Login(context.Background(), "u", "p")

		_, err := c.Share(context.Background(), tok, "/ghost.pdf")
		assert.ErrorIs(t, err, ErrNotFound)
	})
}

func TestFakeClient_Upload(t *testing.T) {
	t.Run("stores the uploaded bytes at the given path", func(t *testing.T) {
		c := NewFake()
		c.AddUser("u", "p")
		tok, _ := c.Login(context.Background(), "u", "p")

		err := c.Upload(context.Background(), tok, "/upload/hello.txt", bytes.NewReader([]byte("hi")))
		require.NoError(t, err)

		// Listing the parent directory should show the file.
		entries, err := c.List(context.Background(), tok, "/upload")
		require.NoError(t, err)
		require.Len(t, entries, 1)
		assert.Equal(t, "hello.txt", entries[0].Name)
		assert.False(t, entries[0].IsDir)
		assert.EqualValues(t, 2, entries[0].Size)
	})

	t.Run("returns ErrUnauthorized for invalid token", func(t *testing.T) {
		c := NewFake()
		err := c.Upload(context.Background(), "bogus", "/x", bytes.NewReader([]byte("x")))
		assert.ErrorIs(t, err, ErrUnauthorized)
	})

	t.Run("overwrites an existing file", func(t *testing.T) {
		c := NewFake()
		c.AddUser("u", "p")
		tok, _ := c.Login(context.Background(), "u", "p")
		c.Seed("/x.txt", []byte("old"))

		err := c.Upload(context.Background(), tok, "/x.txt", bytes.NewReader([]byte("new")))
		require.NoError(t, err)

		body, err := c.Read("/x.txt")
		require.NoError(t, err)
		got, _ := io.ReadAll(body)
		assert.Equal(t, "new", string(got))
	})
}

func TestFakeClient_Search(t *testing.T) {
	t.Run("returns matching files by substring", func(t *testing.T) {
		c := NewFake()
		c.AddUser("u", "p")
		c.Seed("/reports/q3.pdf", []byte("q3"))
		c.Seed("/reports/q4.pdf", []byte("q4"))
		c.Seed("/notes/q4.md", []byte("md"))

		tok, _ := c.Login(context.Background(), "u", "p")

		hits, err := c.Search(context.Background(), tok, "/", "q4")
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"/reports/q4.pdf", "/notes/q4.md"}, pathsOf(hits))
	})

	t.Run("scopes the search to the given root", func(t *testing.T) {
		c := NewFake()
		c.AddUser("u", "p")
		c.Seed("/reports/q4.pdf", []byte("q4"))
		c.Seed("/notes/q4.md", []byte("md"))

		tok, _ := c.Login(context.Background(), "u", "p")

		hits, err := c.Search(context.Background(), tok, "/reports", "q4")
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"/reports/q4.pdf"}, pathsOf(hits))
	})
}

// ── helpers ───────────────────────────────────────────────────────────────────

func namesOf(entries []Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name)
	}
	return out
}

func pathsOf(entries []Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Path)
	}
	return out
}
