package main

import (
	"errors"
	"strings"
)

// configuration is the plugin's admin-console settings, loaded from plugin.json's
// settings_schema. Each user's per-user Filebrowser credentials live separately in
// the plugin KVStore — never here.
type configuration struct {
	// FilebrowserURL is the base URL of the Filebrowser instance, without a
	// trailing slash. Example: "https://files.example.com".
	FilebrowserURL string
}

// validate ensures the configuration is usable. It returns nil for valid configs,
// or a descriptive error otherwise.
func (c *configuration) validate() error {
	if strings.TrimSpace(c.FilebrowserURL) == "" {
		return errors.New("FilebrowserURL is required — set it in System Console → Plugin Management → Filebrowser")
	}
	if strings.HasSuffix(c.FilebrowserURL, "/") {
		return errors.New("FilebrowserURL must not have a trailing slash")
	}
	return nil
}
