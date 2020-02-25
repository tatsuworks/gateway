package handler

import (
	"golang.org/x/xerrors"
)

func (c *Client) GuildEmojisUpdate(d []byte) error {
	eu, err := c.enc.DecodeGuildEmojisUpdate(d)
	if err != nil {
		return xerrors.Errorf("decode guild emojis update: %w", err)
	}

	err = c.db.SetGuildEmojis(defaultCtx, eu.GuildID, eu.Emojis)
	if err != nil {
		return xerrors.Errorf("set guild emojis: %w", err)
	}

	return nil
}
