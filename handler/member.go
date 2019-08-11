package handler

import (
	"github.com/tatsuworks/gateway/discordetf"
	"golang.org/x/xerrors"
)

func (c *Client) MemberChunk(d []byte) error {
	mc, err := discordetf.DecodeMemberChunk(d)
	if err != nil {
		return err
	}

	err = c.db.SetGuildMembers(mc.Guild, mc.Members)
	if err != nil {
		return xerrors.Errorf("failed to set guild members: %w", err)
	}

	return nil
}

func (c *Client) MemberAdd(d []byte) error {
	mc, err := discordetf.DecodeMember(d)
	if err != nil {
		return err
	}

	err = c.db.SetGuildMember(mc.Guild, mc.Id, mc.Raw)
	if err != nil {
		return xerrors.Errorf("failed to set guild member: %w", err)
	}

	return nil
}

func (c *Client) MemberRemove(d []byte) error {
	mc, err := discordetf.DecodeMember(d)
	if err != nil {
		return err
	}

	err = c.db.DeleteGuildMember(mc.Guild, mc.Id)
	if err != nil {
		return xerrors.Errorf("failed to delete guild member: %w", err)
	}

	return nil
}
