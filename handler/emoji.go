package handler

import (
	"golang.org/x/xerrors"

	"github.com/tatsuworks/gateway/discordetf"
)

func (c *Client) GuildEmojisUpdate(d []byte) error {
	eu, err := discordetf.DecodeGuildEmojisUpdate(d)
	if err != nil {
		return xerrors.Errorf("decode guild emojis update: %w", err)
	}

	err = c.db.SetGuildEmojis(eu.GuildID, eu.Emojis)
	if err != nil {
		return xerrors.Errorf("set guild emojis: %w", err)
	}

	return nil
}