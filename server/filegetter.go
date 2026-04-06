package main

import (
	"bytes"
	"io"

	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/rolandh15/mattermost-plugin-filebrowser/server/command"
)

// mmFileGetter implements command.FileGetter by scanning recent channel posts
// for file attachments via the Mattermost plugin API.
type mmFileGetter struct {
	api plugin.API
}

func (g *mmFileGetter) GetRecentFile(channelID string) (string, io.ReadCloser, error) {
	posts, appErr := g.api.GetPostsForChannel(channelID, 0, 20)
	if appErr != nil {
		return "", nil, appErr
	}

	// Walk posts newest-first looking for one with a file attachment.
	for _, id := range posts.Order {
		post := posts.Posts[id]
		if len(post.FileIds) == 0 {
			continue
		}
		fileID := post.FileIds[0]
		info, appErr := g.api.GetFileInfo(fileID)
		if appErr != nil {
			continue
		}
		data, appErr := g.api.GetFile(fileID)
		if appErr != nil {
			continue
		}
		return info.Name, io.NopCloser(bytes.NewReader(data)), nil
	}
	return "", nil, command.ErrNoFile
}
