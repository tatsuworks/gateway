package handler

import (
	"golang.org/x/xerrors"
)

func (c *Client) ChannelCreate(d []byte) error {
	ch, err := c.enc.DecodeChannel(d)
	if err != nil {
		return err
	}

	err = c.db.SetChannel(defaultCtx, ch.GuildID, ch.ID, ch.Raw)
	if err != nil {
		return xerrors.Errorf("set channel: %w", err)
	}

	return nil
}

func (c *Client) ChannelDelete(d []byte) error {
	ch, err := c.enc.DecodeChannel(d)
	if err != nil {
		return err
	}

	err = c.db.DeleteChannel(defaultCtx, ch.GuildID, ch.ID)
	if err != nil {
		return xerrors.Errorf("delete channel: %w", err)
	}

	return nil
}

func (c *Client) VoiceStateUpdate(d []byte) error {
	vs, err := c.enc.DecodeVoiceState(d)
	if err != nil {
		return err
	}

	err = c.db.SetVoiceState(defaultCtx, vs.GuildID, vs.UserID, vs.Raw)
	if err != nil {
		return xerrors.Errorf("set voice state: %w", err)
	}

	return nil
}
