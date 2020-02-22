package handler

import (
	"golang.org/x/xerrors"
)

func (c *Client) RoleCreate(d []byte) error {
	rc, err := c.enc.DecodeRole(d)
	if err != nil {
		return xerrors.Errorf("decode role create: %w", err)
	}

	err = c.db.SetGuildRole(defaultCtx, rc.GuildID, rc.ID, rc.Raw)
	if err != nil {
		return xerrors.Errorf("set guild role: %w", err)
	}

	return nil
}

func (c *Client) RoleDelete(d []byte) error {
	rc, err := c.enc.DecodeRoleDelete(d)
	if err != nil {
		return xerrors.Errorf("decode role delete: %w", err)
	}

	err = c.db.DeleteGuildRole(defaultCtx, rc.GuildID, rc.ID)
	if err != nil {
		return xerrors.Errorf("delete role: %w", err)
	}

	return nil
}
