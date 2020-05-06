package handler

import (
	"context"

	"golang.org/x/xerrors"
)

func (c *Client) GuildEmojisUpdate(ctx context.Context, d []byte) error {
	eu, err := c.enc.DecodeGuildEmojisUpdate(d)
	if err != nil {
		return xerrors.Errorf("decode guild emojis update: %w", err)
	}

	if len(eu.Emojis) > 0 {
		err = c.db.SetGuildEmojis(ctx, eu.GuildID, eu.Emojis)
		if err != nil {
			return xerrors.Errorf("set guild emojis: %w", err)
		}
	}

	return nil
}
