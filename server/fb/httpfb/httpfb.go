// Package httpfb implements fb.Client against Filebrowser's REST API using
// plain net/http. No cgo, no native dependencies.
package httpfb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/rolandh15/mattermost-plugin-filebrowser/server/fb"
)

// Client talks to a Filebrowser instance over HTTP.
// Safe for concurrent use (net/http.Client handles this).
type Client struct {
	baseURL string
	http    *http.Client
}

var _ fb.Client = (*Client)(nil)

// New returns a Client pointed at the given Filebrowser base URL.
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Login exchanges credentials for a JWT token.
func (c *Client) Login(ctx context.Context, username, password string) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"username": username,
		"password": password,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/login", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("httpfb: build login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("httpfb: login request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return "", fb.ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("httpfb: login: unexpected status %d", resp.StatusCode)
	}

	// Filebrowser returns the raw JWT as the response body (quoted JSON string).
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("httpfb: read login response: %w", err)
	}

	token := strings.Trim(strings.TrimSpace(string(raw)), "\"")
	if token == "" {
		return "", fmt.Errorf("httpfb: login returned empty token")
	}
	return token, nil
}

// resource is the JSON shape Filebrowser returns for GET /api/resources/*.
type resource struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	Size     int64      `json:"size"`
	IsDir    bool       `json:"isDir"`
	Modified string     `json:"modified"`
	Items    []resource `json:"items"`
}

func (r *resource) toEntry() fb.Entry {
	e := fb.Entry{
		Name:  r.Name,
		Path:  r.Path,
		IsDir: r.IsDir,
		Size:  r.Size,
	}
	if r.Modified != "" {
		if t, err := time.Parse(time.RFC3339Nano, r.Modified); err == nil {
			e.ModTime = t
		}
	}
	return e
}

// List returns the direct children of a directory.
func (c *Client) List(ctx context.Context, token, dir string) ([]fb.Entry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/resources"+dir, nil)
	if err != nil {
		return nil, fmt.Errorf("httpfb: build list request: %w", err)
	}
	c.auth(req, token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("httpfb: list request: %w", err)
	}
	defer resp.Body.Close()

	if err := checkStatus(resp); err != nil {
		return nil, err
	}

	var r resource
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("httpfb: decode list response: %w", err)
	}

	out := make([]fb.Entry, 0, len(r.Items))
	for i := range r.Items {
		out = append(out, r.Items[i].toEntry())
	}
	return out, nil
}

// shareRequest is the POST body for /api/share/*.
type shareRequest struct {
	Password string `json:"password,omitempty"`
	Expires  string `json:"expires,omitempty"`
	Unit     string `json:"unit,omitempty"`
}

// shareResponse is the JSON Filebrowser returns after creating a share.
type shareResponse struct {
	Hash string `json:"hash"`
	Path string `json:"path"`
}

// Share creates a public share link for the file at path.
func (c *Client) Share(ctx context.Context, token, path string) (string, error) {
	body, _ := json.Marshal(shareRequest{})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/share"+path, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("httpfb: build share request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.auth(req, token)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("httpfb: share request: %w", err)
	}
	defer resp.Body.Close()

	if err := checkStatus(resp); err != nil {
		return "", err
	}

	var sr shareResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return "", fmt.Errorf("httpfb: decode share response: %w", err)
	}
	if sr.Hash == "" {
		return "", fmt.Errorf("httpfb: share response missing hash")
	}
	return c.baseURL + "/share/" + sr.Hash, nil
}

// Upload writes body to the file at remotePath.
func (c *Client) Upload(ctx context.Context, token, remotePath string, body io.Reader) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/resources"+remotePath+"?override=true", body)
	if err != nil {
		return fmt.Errorf("httpfb: build upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	c.auth(req, token)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("httpfb: upload request: %w", err)
	}
	defer resp.Body.Close()

	return checkStatus(resp)
}

// searchResult is one entry in the JSON array from GET /api/search/*.
type searchResult struct {
	Path  string `json:"path"`
	IsDir bool   `json:"dir"`
	Size  int64  `json:"size"`
}

// Search returns entries matching query under root.
func (c *Client) Search(ctx context.Context, token, root, query string) ([]fb.Entry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/search"+root+"?query="+query, nil)
	if err != nil {
		return nil, fmt.Errorf("httpfb: build search request: %w", err)
	}
	c.auth(req, token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("httpfb: search request: %w", err)
	}
	defer resp.Body.Close()

	if err := checkStatus(resp); err != nil {
		return nil, err
	}

	var results []searchResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("httpfb: decode search response: %w", err)
	}

	out := make([]fb.Entry, 0, len(results))
	for _, r := range results {
		out = append(out, fb.Entry{
			Name:  filepath.Base(r.Path),
			Path:  r.Path,
			IsDir: r.IsDir,
			Size:  r.Size,
		})
	}
	return out, nil
}

// auth sets the Authorization header for authenticated requests.
func (c *Client) auth(req *http.Request, token string) {
	req.Header.Set("X-Auth", token)
}

// checkStatus maps HTTP error codes to sentinel errors.
func checkStatus(resp *http.Response) error {
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fb.ErrUnauthorized
	case resp.StatusCode == http.StatusNotFound:
		return fb.ErrNotFound
	default:
		return fmt.Errorf("httpfb: unexpected status %d", resp.StatusCode)
	}
}
