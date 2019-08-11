package handler

import (
	"github.com/tatsuworks/gateway/discordetf"
	"golang.org/x/xerrors"
)

func (c *Client) MessageCreate(d []byte) error {
	mc, err := discordetf.DecodeMessage(d)
	if err != nil {
		return err
	}

	err = c.db.SetChannelMessage(mc.Channel, mc.Id, mc.Raw)
	if err != nil {
		return xerrors.Errorf("failed to set channel message: %w", err)
	}

	return nil
}

func (c *Client) MessageDelete(d []byte) error {
	mc, err := discordetf.DecodeMessage(d)
	if err != nil {
		return err
	}

	err = c.db.DeleteChannelMessage(mc.Channel, mc.Id)
	if err != nil {
		return xerrors.Errorf("failed to delete channel message: %w", err)
	}

	return nil
}

func (c *Client) MessageReactionAdd(d []byte) error {
	rc, err := discordetf.DecodeMessageReaction(d)
	if err != nil {
		return err
	}

	err = c.db.SetChannelMessageReaction(rc.Channel, rc.Message, rc.User, rc.Name, rc.Raw)
	if err != nil {
		return xerrors.Errorf("failed to set channel message reaction: %w", err)
	}

	return nil
}

func (c *Client) MessageReactionRemove(d []byte) error {
	rc, err := discordetf.DecodeMessageReaction(d)
	if err != nil {
		return err
	}

	err = c.db.DeleteChannelMessageReaction(rc.Channel, rc.Message, rc.User, rc.Name)
	if err != nil {
		return xerrors.Errorf("failed to delete channel message reaction: %w", err)
	}

	return nil
}

func (c *Client) MessageReactionRemoveAll(d []byte) error {
	rc, err := discordetf.DecodeMessageReactionRemoveAll(d)
	if err != nil {
		return err
	}

	err = c.db.DeleteChannelMessageReactions(rc.Message, rc.Message, rc.User)
	if err != nil {
		return xerrors.Errorf("failed to remove channel message reactions: %w", err)
	}

	return nil
}
