package main

import (
	"context"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rolandh15/mattermost-plugin-filebrowser/server/command"
	"github.com/rolandh15/mattermost-plugin-filebrowser/server/fb"
)

// activatedPlugin returns a Plugin whose API mock accepts LoadPluginConfiguration
// and RegisterCommand, then has OnActivate run. The caller can then exercise
// ExecuteCommand without re-stubbing the activation path.
func activatedPlugin(t *testing.T) (*Plugin, *plugintest.API) {
	t.Helper()
	api := &plugintest.API{}
	api.On("LoadPluginConfiguration", mock.Anything).Return(func(dest any) error {
		cfg := dest.(*configuration)
		cfg.FilebrowserURL = "https://files.example.com"
		return nil
	})
	api.On("RegisterCommand", mock.Anything).Return(nil)

	p := &Plugin{}
	p.SetAPI(api)
	require.NoError(t, p.OnActivate())

	// Inject a Fake client + in-memory token store so the tests never touch
	// the real Filebrowser or the KVStore.
	fake := fb.NewFake()
	p.router = command.New(fake, &memoryTokenStore{})
	return p, api
}

// memoryTokenStore is a test-only TokenStore that keeps everything in a map.
type memoryTokenStore struct {
	tokens map[string]string
}

func (m *memoryTokenStore) GetToken(_ context.Context, user string) (string, error) {
	if m.tokens == nil {
		return "", nil
	}
	return m.tokens[user], nil
}

func (m *memoryTokenStore) SetToken(_ context.Context, user, token string) error {
	if m.tokens == nil {
		m.tokens = make(map[string]string)
	}
	m.tokens[user] = token
	return nil
}

func (m *memoryTokenStore) ClearToken(_ context.Context, user string) error {
	delete(m.tokens, user)
	return nil
}

func TestExecuteCommand_Help(t *testing.T) {
	p, _ := activatedPlugin(t)

	resp, appErr := p.ExecuteCommand(nil, &model.CommandArgs{
		Command: "/filebrowser help",
		UserId:  "U1",
	})
	require.Nil(t, appErr)
	require.NotNil(t, resp)
	assert.Equal(t, model.CommandResponseTypeEphemeral, resp.ResponseType)
	assert.Contains(t, resp.Text, "connect")
}

func TestExecuteCommand_UnknownTrigger(t *testing.T) {
	p, _ := activatedPlugin(t)

	resp, appErr := p.ExecuteCommand(nil, &model.CommandArgs{
		Command: "/somethingelse arg",
		UserId:  "U1",
	})
	// Mattermost will never route another trigger to us in practice; the
	// plugin should return nil, nil so the server falls through to whoever
	// owns that trigger.
	assert.Nil(t, resp)
	assert.Nil(t, appErr)
}
