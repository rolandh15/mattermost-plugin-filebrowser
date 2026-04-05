package main

import (
	"github.com/mattermost/mattermost/server/public/plugin"
)

// main is the plugin executable entry point. The Mattermost server runs this
// binary as a subprocess when it activates the plugin; plugin.ClientMain sets
// up the RPC channel and blocks until the server tells us to stop.
func main() {
	plugin.ClientMain(&Plugin{})
}
