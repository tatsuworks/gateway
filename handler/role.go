package handler

import (
	"github.com/tatsuworks/gateway/discordetf"
	"golang.org/x/xerrors"
)

func (c *Client) RoleCreate(d []byte) error {
	rc, err := discordetf.DecodeRole(d)
	if err != nil {
		return xerrors.Errorf("failed to decode role create: %w", err)
	}

	err = c.db.SetGuildRole(rc.Guild, rc.Id, rc.Raw)
	if err != nil {
		return xerrors.Errorf("failed to set guild role: %w", err)
	}

	return nil
}

func (c *Client) RoleDelete(d []byte) error {
	rc, err := discordetf.DecodeRoleDelete(d)
	if err != nil {
		return xerrors.Errorf("failed to decode role delete: %w", err)
	}

	err = c.db.DeleteGuildRole(rc.Guild, rc.Id)
	if err != nil {
		return xerrors.Errorf("failed to delete role: %w", err)
	}

	return nil
}