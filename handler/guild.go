package handler

import (
	"github.com/tatsuworks/gateway/discordetf"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
)

func (c *Client) GuildCreate(d []byte) error {
	gc, err := discordetf.DecodeGuildCreate(d)
	if err != nil {
		return xerrors.Errorf("failed to parse guild create: %w", err)
	}

	eg := new(errgroup.Group)

	eg.Go(func() error {
		err := c.db.SetGuild(gc.Id, gc.Guild)
		if err != nil {
			return xerrors.Errorf("failed to set guild: %w", err)
		}
		return nil
	})

	eg.Go(func() error {
		if len(gc.Roles) > 0 {
			err := c.db.SetGuildRoles(gc.Id, gc.Roles)
			if err != nil {
				return xerrors.Errorf("failed to set guild roles: %w", err)
			}
		}
		return nil
	})
	eg.Go(func() error {
		if len(gc.Members) > 0 {
			err := c.db.SetGuildMembers(gc.Id, gc.Members)
			if err != nil {
				return xerrors.Errorf("failed to set guild member: %w", err)
			}
		}
		return nil
	})
	eg.Go(func() error {
		if len(gc.Channels) > 0 {
			err := c.db.SetChannels(gc.Id, gc.Channels)
			if err != nil {
				return xerrors.Errorf("failed to set guild channels: %w", err)
			}

		}
		return nil
	})

	return eg.Wait()
}

func (c *Client) GuildDelete(d []byte) error {
	gc, err := discordetf.DecodeGuildCreate(d)
	if err != nil {
		return err
	}

	eg := new(errgroup.Group)

	eg.Go(func() error {
		err := c.db.DeleteGuild(gc.Id)
		if err != nil {
			return xerrors.Errorf("failed to delete guild: %w", err)
		}

		return nil
	})

	eg.Go(func() error {
		err := c.db.DeleteGuildRoles(gc.Id)
		if err != nil {
			return xerrors.Errorf("failed to delete guild roles: %w", err)
		}

		return nil
	})
	eg.Go(func() error {
		err := c.db.DeleteGuildMembers(gc.Id)
		if err != nil {
			return xerrors.Errorf("failed to delete guild members: %w", err)
		}

		return nil
	})
	eg.Go(func() error {
		err := c.db.DeleteChannels(gc.Id)
		if err != nil {
			return xerrors.Errorf("failed to delete channels: %w", err)
		}

		return nil
	})

	return eg.Wait()
}

func (c *Client) GuildBanAdd(d []byte) error {
	gb, err := discordetf.DecodeGuildBan(d)
	if err != nil {
		return err
	}

	err = c.db.SetGuildBan(gb.Guild, gb.User, gb.Raw)
	if err != nil {
		return xerrors.Errorf("failed to set guild ban: %w", err)
	}
	return nil
}

func (c *Client) GuildBanRemove(d []byte) error {
	gb, err := discordetf.DecodeGuildBan(d)
	if err != nil {
		return err
	}

	err = c.db.DeleteGuildBan(gb.Guild, gb.User)
	if err != nil {
		return xerrors.Errorf("failed to delete guild ban: %w", err)
	}
	return nil
}
