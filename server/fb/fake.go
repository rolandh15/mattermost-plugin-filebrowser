package fb

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"path"
	"strings"
	"sync"
	"time"
)

// Fake is an in-memory Client used by tests. It intentionally implements just
// enough Filebrowser semantics for the plugin's unit tests: a flat map of
// paths to byte contents, with directory entries derived from the path
// structure on each call.
//
// Fake is safe for concurrent use.
type Fake struct {
	mu    sync.RWMutex
	users map[string]string      // username → password
	files map[string][]byte      // path → content
	times map[string]time.Time   // path → modtime
	tokens map[string]string     // token → username
}

// NewFake returns an empty Fake.
func NewFake() *Fake {
	return &Fake{
		users:  make(map[string]string),
		files:  make(map[string][]byte),
		times:  make(map[string]time.Time),
		tokens: make(map[string]string),
	}
}

// AddUser registers a user that can Login. Tests typically call this once in
// setup.
func (f *Fake) AddUser(username, password string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.users[username] = password
}

// Seed stores content at an absolute path and sets its mod time to now. Any
// missing parent directories are implied (the Fake does not maintain explicit
// directory entries — they are derived at list time).
func (f *Fake) Seed(path string, content []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[normalise(path)] = append([]byte(nil), content...)
	f.times[normalise(path)] = time.Now()
}

// Read returns an io.ReadCloser over the stored bytes at the given path.
// It is a test helper (not part of the Client interface) that lets tests
// assert what was written by Upload.
func (f *Fake) Read(p string) (io.ReadCloser, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	b, ok := f.files[normalise(p)]
	if !ok {
		return nil, ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

// ── Client implementation ─────────────────────────────────────────────────────

func (f *Fake) Login(_ context.Context, username, password string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	stored, ok := f.users[username]
	if !ok || stored != password {
		return "", ErrUnauthorized
	}

	// Deterministic token so callers that re-login within the same test see
	// a stable value.
	sum := sha256.Sum256([]byte("fake-token:" + username))
	token := hex.EncodeToString(sum[:16])
	f.tokens[token] = username
	return token, nil
}

func (f *Fake) List(_ context.Context, token, dir string) ([]Entry, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if _, ok := f.tokens[token]; !ok {
		return nil, ErrUnauthorized
	}

	dir = normalise(dir)
	prefix := dir
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	seen := make(map[string]Entry)
	matched := false
	for p, content := range f.files {
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		matched = true
		rest := strings.TrimPrefix(p, prefix)
		if rest == "" {
			continue
		}
		// Direct child: either the whole rest (file) or the first segment
		// (subdirectory).
		if idx := strings.IndexByte(rest, '/'); idx >= 0 {
			name := rest[:idx]
			seen[name] = Entry{
				Name:  name,
				Path:  prefix + name,
				IsDir: true,
			}
		} else {
			seen[rest] = Entry{
				Name:    rest,
				Path:    p,
				IsDir:   false,
				Size:    int64(len(content)),
				ModTime: f.times[p],
			}
		}
	}

	if !matched && dir != "/" {
		return nil, ErrNotFound
	}

	out := make([]Entry, 0, len(seen))
	for _, e := range seen {
		out = append(out, e)
	}
	return out, nil
}

func (f *Fake) Share(_ context.Context, token, p string) (string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if _, ok := f.tokens[token]; !ok {
		return "", ErrUnauthorized
	}
	p = normalise(p)
	if _, ok := f.files[p]; !ok {
		return "", ErrNotFound
	}
	return "https://fake.filebrowser.invalid/share" + p, nil
}

func (f *Fake) Upload(_ context.Context, token, p string, body io.Reader) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, ok := f.tokens[token]; !ok {
		return ErrUnauthorized
	}

	buf, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	f.files[normalise(p)] = buf
	f.times[normalise(p)] = time.Now()
	return nil
}

func (f *Fake) Search(_ context.Context, token, root, query string) ([]Entry, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if _, ok := f.tokens[token]; !ok {
		return nil, ErrUnauthorized
	}

	root = normalise(root)
	prefix := root
	if root != "/" {
		prefix += "/"
	}

	var out []Entry
	for p, content := range f.files {
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		if !strings.Contains(path.Base(p), query) && !strings.Contains(p, query) {
			continue
		}
		out = append(out, Entry{
			Name:    path.Base(p),
			Path:    p,
			IsDir:   false,
			Size:    int64(len(content)),
			ModTime: f.times[p],
		})
	}
	return out, nil
}

// normalise cleans an absolute path so that repeated slashes and trailing
// slashes do not produce different keys in the Fake's maps.
func normalise(p string) string {
	if p == "" {
		return "/"
	}
	cleaned := path.Clean(p)
	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	return cleaned
}
