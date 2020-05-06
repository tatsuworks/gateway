package handler

import (
	"context"

	"golang.org/x/xerrors"
)

func (c *Client) MessageCreate(ctx context.Context, d []byte) error {
	mc, err := c.enc.DecodeMessage(d)
	if err != nil {
		return err
	}

	err = c.db.SetChannelMessage(ctx, mc.ChannelID, mc.ID, mc.Raw)
	if err != nil {
		return xerrors.Errorf("set channel message: %w", err)
	}

	return nil
}

func (c *Client) MessageDelete(ctx context.Context, d []byte) error {
	mc, err := c.enc.DecodeMessage(d)
	if err != nil {
		return err
	}

	err = c.db.DeleteChannelMessage(ctx, mc.ChannelID, mc.ID)
	if err != nil {
		return xerrors.Errorf("delete channel message: %w", err)
	}

	return nil
}

func (c *Client) MessageReactionAdd(ctx context.Context, d []byte) error {
	rc, err := c.enc.DecodeMessageReaction(d)
	if err != nil {
		return err
	}

	err = c.db.SetChannelMessageReaction(ctx, rc.ChannelID, rc.MessageID, rc.UserID, rc.Name, rc.Raw)
	if err != nil {
		return xerrors.Errorf("set channel message reaction: %w", err)
	}

	return nil
}

func (c *Client) MessageReactionRemove(ctx context.Context, d []byte) error {
	rc, err := c.enc.DecodeMessageReaction(d)
	if err != nil {
		return err
	}

	err = c.db.DeleteChannelMessageReaction(ctx, rc.ChannelID, rc.MessageID, rc.UserID, rc.Name)
	if err != nil {
		return xerrors.Errorf("delete channel message reaction: %w", err)
	}

	return nil
}

func (c *Client) MessageReactionRemoveAll(ctx context.Context, d []byte) error {
	rc, err := c.enc.DecodeMessageReactionRemoveAll(d)
	if err != nil {
		return err
	}

	err = c.db.DeleteChannelMessageReactions(ctx, rc.ChannelID, rc.MessageID, rc.UserID)
	if err != nil {
		return xerrors.Errorf("remove channel message reactions: %w", err)
	}

	return nil
}
