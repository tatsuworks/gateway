package handler

import (
	"context"

	"cdr.dev/slog"
	"golang.org/x/xerrors"
)

func (c *Client) MemberChunk(ctx context.Context, d []byte) error {
	mc, err := c.enc.DecodeMemberChunk(d)
	if err != nil {
		return err
	}

	err = c.db.SetGuildMembers(ctx, mc.GuildID, mc.Members)
	if err != nil {
		c.log.Error(ctx, "failed to set members", slog.Error(err))
	}

	if mc.ChunkIndex == 0 {
		if err := c.db.BeginGuildBackfill(ctx, mc.GuildID); err != nil {
			c.log.Error(ctx, "begin guild backfill", slog.Error(err), slog.F("guild", mc.GuildID))
		}
	}
	// ChunkCount > 0 guard: a payload missing the field decodes to 0 and can never stamp a marker.
	if mc.ChunkCount > 0 && mc.ChunkIndex == mc.ChunkCount-1 {
		if err := c.db.CompleteGuildBackfill(ctx, mc.GuildID); err != nil {
			c.log.Error(ctx, "complete guild backfill", slog.Error(err), slog.F("guild", mc.GuildID))
		}
	}

	return nil
}

func (c *Client) MemberAdd(ctx context.Context, d []byte, isNew bool) error {
	mc, err := c.enc.DecodeMember(d)
	if err != nil {
		return err
	}

	err = c.db.SetGuildMember(ctx, mc.GuildID, mc.ID, mc.Raw, isNew)
	if err != nil {
		return xerrors.Errorf("set guild member: %w", err)
	}

	return nil
}

func (c *Client) MemberRemove(ctx context.Context, d []byte) error {
	mc, err := c.enc.DecodeMember(d)
	if err != nil {
		return err
	}

	err = c.db.DeleteGuildMember(ctx, mc.GuildID, mc.ID)
	if err != nil {
		return xerrors.Errorf("delete guild member: %w", err)
	}

	return nil
}

func (c *Client) PresenceCreate(ctx context.Context, d []byte) error {
	th, err := c.enc.DecodePresence(d)
	if err != nil {
		return err
	}

	err = c.db.SetPresence(ctx, th.GuildID, th.ID, th.Raw)
	if err != nil {
		return xerrors.Errorf("set user presence: %w", err)
	}

	return nil
}
