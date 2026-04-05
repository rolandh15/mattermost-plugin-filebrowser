package main

import (
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOnActivate(t *testing.T) {
	t.Run("succeeds when Filebrowser URL is configured", func(t *testing.T) {
		api := &plugintest.API{}
		api.On("LoadPluginConfiguration", mock.Anything).Return(func(dest any) error {
			cfg := dest.(*configuration)
			cfg.FilebrowserURL = "https://files.example.com"
			return nil
		})
		api.On("RegisterCommand", mock.MatchedBy(func(c *model.Command) bool {
			return c.Trigger == "filebrowser"
		})).Return(nil)

		p := &Plugin{}
		p.SetAPI(api)

		require.NoError(t, p.OnActivate())
		api.AssertExpectations(t)
	})

	t.Run("fails when Filebrowser URL is missing", func(t *testing.T) {
		api := &plugintest.API{}
		api.On("LoadPluginConfiguration", mock.Anything).Return(func(dest any) error {
			cfg := dest.(*configuration)
			cfg.FilebrowserURL = ""
			return nil
		})

		p := &Plugin{}
		p.SetAPI(api)

		err := p.OnActivate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "FilebrowserURL")
	})

	t.Run("fails when Filebrowser URL has a trailing slash", func(t *testing.T) {
		api := &plugintest.API{}
		api.On("LoadPluginConfiguration", mock.Anything).Return(func(dest any) error {
			cfg := dest.(*configuration)
			cfg.FilebrowserURL = "https://files.example.com/"
			return nil
		})

		p := &Plugin{}
		p.SetAPI(api)

		err := p.OnActivate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "trailing slash")
	})
}
