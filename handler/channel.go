package handler

import (
	"context"

	"golang.org/x/xerrors"
)

func (c *Client) ChannelCreate(ctx context.Context, d []byte) error {
	ch, err := c.enc.DecodeChannel(d)
	if err != nil {
		return err
	}

	err = c.db.SetChannel(ctx, ch.GuildID, ch.ID, ch.Raw)
	if err != nil {
		return xerrors.Errorf("set channel: %w", err)
	}

	return nil
}

func (c *Client) ChannelDelete(ctx context.Context, d []byte) error {
	ch, err := c.enc.DecodeChannel(d)
	if err != nil {
		return err
	}

	err = c.db.DeleteChannel(ctx, ch.GuildID, ch.ID)
	if err != nil {
		return xerrors.Errorf("delete channel: %w", err)
	}

	return nil
}

func (c *Client) VoiceStateUpdate(ctx context.Context, d []byte) error {
	vs, err := c.enc.DecodeVoiceState(d)
	if err != nil {
		return err
	}

	err = c.db.SetVoiceState(ctx, vs.GuildID, vs.UserID, vs.Raw)
	if err != nil {
		return xerrors.Errorf("set voice state: %w", err)
	}

	return nil
}
