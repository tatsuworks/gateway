package handler

import (
	"cdr.dev/slog"
	"golang.org/x/xerrors"
)

func (c *Client) MemberChunk(d []byte) error {
	mc, err := c.enc.DecodeMemberChunk(d)
	if err != nil {
		return err
	}

	if mc.GuildID == 173184118492889089 || mc.GuildID == 390426490103136256 {
		c.log.Info(defaultCtx,
			"member chunk",
			slog.F("guild", mc.GuildID),
			slog.F("members", len(mc.Members)),
		)
	}

	err = c.db.SetGuildMembers(defaultCtx, mc.GuildID, mc.Members)
	if err != nil {
		return xerrors.Errorf("set guild members: %w", err)
	}

	return nil
}

func (c *Client) MemberAdd(d []byte) error {
	mc, err := c.enc.DecodeMember(d)
	if err != nil {
		return err
	}

	err = c.db.SetGuildMember(defaultCtx, mc.GuildID, mc.ID, mc.Raw)
	if err != nil {
		return xerrors.Errorf("set guild member: %w", err)
	}

	return nil
}

func (c *Client) MemberRemove(d []byte) error {
	mc, err := c.enc.DecodeMember(d)
	if err != nil {
		return err
	}

	err = c.db.DeleteGuildMember(defaultCtx, mc.GuildID, mc.ID)
	if err != nil {
		return xerrors.Errorf("delete guild member: %w", err)
	}

	return nil
}
