package handler

import (
	"golang.org/x/xerrors"

	"github.com/tatsuworks/gateway/discordetf"
)

func (s *Client) ChannelCreate(d []byte) error {
	ch, err := discordetf.DecodeChannel(d)
	if err != nil {
		return err
	}

	err = s.db.SetChannel(ch.Guild, ch.Id, ch.Raw)
	if err != nil {
		return xerrors.Errorf("set channel: %w", err)
	}

	return nil
}

func (c *Client) ChannelDelete(d []byte) error {
	ch, err := discordetf.DecodeChannel(d)
	if err != nil {
		return err
	}

	err = c.db.DeleteChannel(ch.Guild, ch.Id, ch.Raw)
	if err != nil {
		return xerrors.Errorf("delete channel: %w", err)
	}

	return nil
}

func (c *Client) VoiceStateUpdate(d []byte) error {
	vs, err := discordetf.DecodeVoiceState(d)
	if err != nil {
		return err
	}

	err = c.db.SetVoiceState(vs.Guild, vs.User, vs.Raw)
	if err != nil {
		return xerrors.Errorf("set voice state: %w", err)
	}

	return nil
}
