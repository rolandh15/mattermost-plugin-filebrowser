package main

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/rolandh15/mattermost-plugin-filebrowser/server/command"
)

// commandTrigger is the slash command users type in Mattermost: `/filebrowser ...`.
const commandTrigger = "filebrowser"

// Plugin is the Mattermost plugin entry point. The struct embeds
// plugin.MattermostPlugin so that the MM runtime can dispatch lifecycle hooks
// (OnActivate, OnDeactivate, …) and gives us access to the API via p.API.
type Plugin struct {
	plugin.MattermostPlugin

	// configMu guards the active configuration value. Mattermost may call
	// OnConfigurationChange concurrently with command handlers, so reads and
	// writes of the config struct must be synchronised.
	configMu sync.RWMutex
	config   *configuration

	// router dispatches /filebrowser subcommands. Tests may swap this with
	// a Router built against a Fake client to avoid hitting the network.
	router *command.Router
}

// OnActivate is called by the Mattermost server when the plugin is installed or
// enabled. We load the admin-console configuration, validate it, and register
// the `/filebrowser` slash command.
func (p *Plugin) OnActivate() error {
	cfg := &configuration{}
	if err := p.API.LoadPluginConfiguration(cfg); err != nil {
		return fmt.Errorf("failed to load plugin configuration: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return err
	}
	p.setConfiguration(cfg)

	if err := p.API.RegisterCommand(p.buildCommand()); err != nil {
		return fmt.Errorf("failed to register /%s command: %w", commandTrigger, err)
	}

	return nil
}

// setConfiguration atomically swaps the active configuration.
func (p *Plugin) setConfiguration(cfg *configuration) {
	p.configMu.Lock()
	defer p.configMu.Unlock()
	p.config = cfg
}

// getConfiguration returns the currently active configuration. It never returns
// nil once OnActivate has completed — a nil return indicates a programmer error
// (the plugin wasn't activated first).
func (p *Plugin) getConfiguration() *configuration {
	p.configMu.RLock()
	defer p.configMu.RUnlock()
	return p.config
}

// ExecuteCommand is called by Mattermost when a user runs `/filebrowser …`.
// We defer the actual parsing to the Router package so the code path can be
// unit-tested without building Mattermost model values by hand.
func (p *Plugin) ExecuteCommand(_ *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	if !strings.HasPrefix(args.Command, "/"+commandTrigger) {
		// Not our trigger — let the server route it to whoever owns it.
		return nil, nil
	}

	if p.router == nil {
		return ephemeral("Filebrowser plugin is not fully initialised. Please wait a moment and retry."), nil
	}

	resp, err := p.router.Handle(context.Background(), args.UserId, args.Command)
	if err != nil {
		p.API.LogError("filebrowser: command handler failed", "user", args.UserId, "err", err.Error())
		return ephemeral(fmt.Sprintf("Sorry, something went wrong: `%s`.", err.Error())), nil
	}
	return ephemeral(resp.Text), nil
}

// ephemeral wraps a message into a CommandResponse that is only visible to
// the caller. Filebrowser slash commands never post to the channel unless the
// user explicitly asks for it (e.g. "copy this link to channel" button).
func ephemeral(text string) *model.CommandResponse {
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         text,
	}
}

// buildCommand describes the `/filebrowser` slash command to the Mattermost
// server. Autocomplete metadata will be filled in by future command handlers.
func (p *Plugin) buildCommand() *model.Command {
	return &model.Command{
		Trigger:          commandTrigger,
		AutoComplete:     true,
		AutoCompleteDesc: "Browse, share and upload files on your Filebrowser instance.",
		AutoCompleteHint: "[connect|browse|save|share|search|disconnect|help]",
		DisplayName:      "Filebrowser",
		Description:      "Interact with a Filebrowser instance from Mattermost.",
	}
}
