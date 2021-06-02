package handler

import (
	"context"

	"golang.org/x/xerrors"
)

func (c *Client) ThreadCreate(ctx context.Context, d []byte) error {
	th, err := c.enc.DecodeThread(d)
	if err != nil {
		return err
	}

	err = c.db.SetThread(ctx, th.GuildID, th.ParentID, th.OwnerID, th.ID, th.Raw)
	if err != nil {
		return xerrors.Errorf("set thread: %w", err)
	}

	return nil
}

func (c *Client) ThreadDelete(ctx context.Context, d []byte) error {
	th, err := c.enc.DecodeThread(d)
	if err != nil {
		return err
	}

	err = c.db.DeleteThread(ctx, th.ID)
	if err != nil {
		return xerrors.Errorf("delete thread: %w", err)
	}

	return nil
}
