package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

// kvTokenPrefix namespaces per-user Filebrowser token entries in the plugin
// KVStore. Keys look like "fb_token:<mattermostUserID>" so an operator
// scrubbing secrets can find them with a single prefix scan.
const kvTokenPrefix = "fb_token:"

// kvTokenStore is a command.TokenStore backed by Mattermost's plugin KVStore.
// Tokens are stored as raw bytes with no TTL; Filebrowser tokens never
// expire on the server side, so the only time we remove them is when a
// user runs /filebrowser disconnect (or an admin wipes the namespace).
type kvTokenStore struct {
	api plugin.API
}

// newKVTokenStore wraps a Mattermost plugin API into a TokenStore. The
// plugin constructs exactly one of these in OnActivate and hands it to the
// command router.
func newKVTokenStore(api plugin.API) *kvTokenStore {
	return &kvTokenStore{api: api}
}

// key returns the fully-namespaced KVStore key for a given user.
func (s *kvTokenStore) key(mmUserID string) string {
	return kvTokenPrefix + mmUserID
}

// GetToken returns the token for a user, or the empty string plus a
// sentinel error if no token has been stored yet. Callers use that to
// tell "not connected" apart from "connected but something else broke".
func (s *kvTokenStore) GetToken(_ context.Context, mmUserID string) (string, error) {
	raw, appErr := s.api.KVGet(s.key(mmUserID))
	if appErr != nil {
		return "", kvErr("read token", appErr)
	}
	if len(raw) == 0 {
		return "", ErrNotConnected
	}
	return string(raw), nil
}

// SetToken persists the given token for a user, overwriting any previous
// value.
func (s *kvTokenStore) SetToken(_ context.Context, mmUserID, token string) error {
	if appErr := s.api.KVSet(s.key(mmUserID), []byte(token)); appErr != nil {
		return kvErr("write token", appErr)
	}
	return nil
}

// ClearToken removes the stored token for a user. It is idempotent —
// deleting a non-existent key is not an error.
func (s *kvTokenStore) ClearToken(_ context.Context, mmUserID string) error {
	if appErr := s.api.KVDelete(s.key(mmUserID)); appErr != nil {
		return kvErr("delete token", appErr)
	}
	return nil
}

// ErrNotConnected is returned from GetToken when a user has never connected
// (no entry in KVStore) or has explicitly disconnected. Command handlers
// check errors.Is(err, ErrNotConnected) so they can emit a helpful
// "run /filebrowser connect first" ephemeral message instead of a raw
// error.
var ErrNotConnected = errors.New("filebrowser: not connected — run /filebrowser connect first")

// kvErr wraps a Mattermost AppError into a plain Go error with context.
// We never leak the AppError's internal `DetailedError` field into user
// messages because it can contain plugin internals.
func kvErr(op string, appErr *model.AppError) error {
	if appErr == nil {
		return nil
	}
	return fmt.Errorf("kvstore: %s: %s", op, appErr.Message)
}
